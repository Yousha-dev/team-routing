package main

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func resetCacheEvidence(t *testing.T) {
	t.Helper()
	evidence.Lock()
	evidence.Selections = map[string]selectionEvidence{}
	evidence.Warm = map[string]warmCache{}
	evidence.Seen = map[string]time.Time{}
	evidence.Counters = map[string]tokenCounters{}
	evidence.Recent = nil
	evidence.Pruned = time.Time{}
	evidence.Decisions = 0
	evidence.Retained = 0
	evidence.Switched = 0
	evidence.Unlock()
}
func TestBodyOnlyClaudeSessionsAndSubagentsStayDistinct(t *testing.T) {
	req := availabilityRequest{Headers: http.Header{affinityHeader: {"spoof"}}, Body: []byte(`{"metadata":{"user_id":"{\"session_id\":\"root\",\"agent_id\":\"worker-a\"}"},"messages":[{"role":"user","content":"private prompt"}]}`)}
	a := preparedSession(req)
	req.Body = []byte(strings.ReplaceAll(string(req.Body), "worker-a", "worker-b"))
	b := preparedSession(req)
	if a == b || a == digest("key-fallback") || strings.Contains(a, "root") {
		t.Fatal("body session/agent identities lost or exposed", a, b)
	}
	req.Body = []byte(strings.ReplaceAll(string(req.Body), "private prompt", "different turn"))
	if preparedSession(req) != b {
		t.Fatal("turn content changed affinity")
	}
	req.Headers.Del(affinityHeader)
	if preparedSession(req) != b {
		t.Fatal("caller spoof was trusted")
	}
	h := http.Header{"Session_id": {"thread-1"}}
	if session(h, nil) == "key-fallback" {
		t.Fatal("Codex underscore session ignored")
	}
	if affinityKey(nil, map[string]any{"caller_scope": "a"}, "claude", "opus") == affinityKey(nil, map[string]any{"caller_scope": "b"}, "claude", "opus") {
		t.Fatal("callers collided")
	}
	raw, _ := json.Marshal(map[string]any{"RequestID": "hint-cleanup", "Headers": http.Header{affinityHeader: {a}}, "Metadata": map[string]any{}})
	result, err := trackRequest("request.intercept_after", raw)
	if err != nil || len(result.(map[string]any)["ClearHeaders"].([]string)) != 1 {
		t.Fatal("private hint not removed", result, err)
	}
}
func TestCurrentOpportunityCannotOutliveCurrentWindow(t *testing.T) {
	now := time.Now()
	got := quotaOpportunities(defaultPolicy(), candidate{}, now, now.Add(10*time.Minute), 1, now.Add(time.Hour), 1)
	if math.Abs(got-1) > 1e-8 {
		t.Fatal("current-window capacity was counted beyond reset", got)
	}
}
func TestMeasuredWarmCacheRetainedButUrgentDeadlineStillMoves(t *testing.T) {
	resetCacheEvidence(t)
	now := time.Now()
	p := defaultPolicy()
	p.RecordRouting = false
	state.Pins = map[string]binding{}
	a := candidate{ID: "cache-a", Provider: "claude"}
	b := candidate{ID: "cache-b", Provider: "claude"}
	state.Quota = map[string]snapshot{a.ID: sample(now, .8, .5, 3*time.Hour), b.ID: sample(now, .8, .67, 3*time.Hour)}
	h := http.Header{"X-Api-Key": {"cache-client"}, "Session-Id": {"conversation"}}
	choose(p, "claude", "opus", h, nil, []candidate{a}, now)
	pk := affinityKey(h, nil, "claude", "opus")
	evidence.Lock()
	evidence.Warm[pk] = warmCache{now, a.ID, .95}
	evidence.Unlock()
	if r := choose(p, "claude", "opus", h, nil, []candidate{a, b}, now); r["AuthID"] != a.ID {
		t.Fatal("modest urgency discarded warm cache", r)
	}
	evidence.Lock()
	evidence.Warm = map[string]warmCache{}
	evidence.Unlock()
	if r := choose(p, "claude", "opus", h, nil, []candidate{a, b}, now); r["AuthID"] != b.ID {
		t.Fatal("cold account ignored meaningful urgency", r)
	}
	// A materially urgent reset overrides even measured warm affinity immediately.
	state.Pins[pk] = binding{ID: a.ID, At: now, BoundAt: now}
	evidence.Lock()
	evidence.Warm[pk] = warmCache{now, a.ID, 1}
	evidence.Unlock()
	state.Quota[b.ID] = sample(now, .8, .67, 10*time.Minute)
	if r := choose(p, "claude", "opus", h, nil, []candidate{a, b}, now); r["AuthID"] != b.ID {
		t.Fatal("warm affinity blocked imminent waste", r)
	}
	state.Quota[a.ID] = sample(now, 0, .5, time.Hour)
	if r := choose(p, "claude", "opus", h, nil, []candidate{a, b}, now); r["AuthID"] != b.ID {
		t.Fatal("warm cache bypassed exhausted quota", r)
	}
}
func TestUsageCountersCorrelationPrivacyAndWarmExpiry(t *testing.T) {
	resetCacheEvidence(t)
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	p := defaultPolicy()
	p.RecordRouting = true
	raw, _ := json.Marshal(p)
	_ = os.WriteFile("team-routing.json", raw, 0600)
	now := time.Now()
	meta := map[string]any{"selection_id": "fixture-selection", "caller_scope": "opaque-caller"}
	recordDecision(p, nil, meta, "claude", "opus", "auth-a", "", "best_deadline_score", 0, nil, now)
	usage := map[string]any{"RequestID": "execution-a", "SelectionID": "fixture-selection", "Provider": "claude", "Model": "opus", "AuthID": "auth-a", "RequestedAt": now, "APIKey": "do-not-log-key", "SessionID": "do-not-log-session", "ResponseHeaders": http.Header{"Secret": {"do-not-log-header"}}, "Detail": map[string]int64{"InputTokens": 100, "CacheReadTokens": 800, "CacheCreationTokens": 100, "OutputTokens": 20}}
	raw, _ = json.Marshal(usage)
	if _, e := observeUsage(raw); e != nil {
		t.Fatal(e)
	}
	_, _ = observeUsage(raw)
	pk := affinityKey(nil, meta, "claude", "opus")
	if cacheWarmth(pk, "auth-a", now.Add(time.Minute)) < .89 || cacheWarmth(pk, "auth-a", now.Add(5*time.Minute)) != 0 || cacheWarmth(pk, "auth-b", now.Add(time.Minute)) != 0 {
		t.Fatal("warm lifetime or account scope wrong")
	}
	totals := cacheDiagnostics()["totals"].(tokenCounters)
	if totals.Requests != 1 || totals.Input != 1000 || totals.Read != 800 || totals.Write != 100 || math.Abs(totals.Share-.8) > 1e-8 {
		t.Fatal("Claude counters or dedup wrong", totals)
	}
	usage["RequestID"] = "execution-b"
	usage["Provider"] = "codex"
	usage["Detail"] = map[string]int64{"InputTokens": 1000, "CachedTokens": 800, "OutputTokens": 20}
	raw, _ = json.Marshal(usage)
	_, _ = observeUsage(raw)
	totals = cacheDiagnostics()["totals"].(tokenCounters)
	if totals.Input != 2000 || totals.Read != 1600 {
		t.Fatal("OpenAI cached input was double-counted", totals)
	}
	data, e := os.ReadFile("team-routing-usage.jsonl")
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []string{"do-not-log-key", "do-not-log-session", "do-not-log-header", "APIKey", "ResponseHeaders"} {
		if strings.Contains(string(data), v) {
			t.Fatal("secret field persisted", v)
		}
	}
}
func TestFailedOrMissingUsageCannotCreateWarmCache(t *testing.T) {
	resetCacheEvidence(t)
	now := time.Now()
	p := defaultPolicy()
	p.RecordRouting = false
	meta := map[string]any{"selection_id": "usage-failed"}
	recordDecision(p, nil, meta, "claude", "opus", "a", "", "best", 0, nil, now)
	raw, _ := json.Marshal(map[string]any{"RequestID": "r", "SelectionID": "usage-failed", "AuthID": "a", "Provider": "claude", "RequestedAt": now, "Failed": true, "Detail": map[string]int{"CacheReadTokens": 1000}})
	_, _ = observeUsage(raw)
	if cacheWarmth(affinityKey(nil, meta, "claude", "opus"), "a", now) != 0 {
		t.Fatal("failed request established cache warmth")
	}
}

func TestSaturatedWeeklyPressureOverridesWarmAffinity(t *testing.T) {
	resetCacheEvidence(t)
	now := time.Now()
	p := defaultPolicy()
	p.RecordRouting = false
	p.WeeklyRatio = 30
	a := candidate{ID: "later", Provider: "claude"}
	b := candidate{ID: "sooner", Provider: "claude"}
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{a.ID: deadlineSample(now, .8, .9, now.Add(3*time.Hour), now.Add(12*time.Hour)), b.ID: deadlineSample(now, .8, .9, now.Add(3*time.Hour), now.Add(6*time.Hour))}
	h := http.Header{"Session-Id": {"saturated"}}
	choose(p, "claude", "opus", h, nil, []candidate{a}, now)
	pk := affinityKey(h, nil, "claude", "opus")
	evidence.Lock()
	evidence.Warm[pk] = warmCache{now, a.ID, 1}
	evidence.Unlock()
	if r := choose(p, "claude", "opus", h, nil, []candidate{a, b}, now); r["AuthID"] != b.ID {
		t.Fatal("score saturation stranded nearer weekly deadline", r)
	}
	diagnostics := cacheDiagnostics()["recent_decisions"].([]selectionEvidence)
	if diagnostics[len(diagnostics)-1].Reason != "weekly_deadline_at_risk" {
		t.Fatal("deadline rescue not explained", diagnostics)
	}
}
