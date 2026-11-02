package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestParallelRequestsSpreadBeforeDeadlineUrgency(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "antigravity", "custom-provider"} {
		t.Run(provider, func(t *testing.T) {
			t.Chdir(t.TempDir())
			applyConfig(nil)
			t.Cleanup(func() { applyConfig(nil) })
			traffic.Lock()
			traffic.Active = map[string]activeRequest{}
			traffic.Pending = map[string]activeRequest{}
			traffic.Unlock()
			p := defaultPolicy()
			now := time.Now()
			state.Pins = map[string]binding{}
			state.Quota = map[string]snapshot{}
			cs := []candidate{{ID: "urgent", Provider: provider}, {ID: "second", Provider: provider}, {ID: "third", Provider: provider}}
			for _, c := range cs {
				state.Quota[c.ID] = sample(now, .9, .8, 4*time.Hour)
			}
			state.Quota["urgent"] = sample(now, .9, .8, 30*time.Minute)
			seen := map[string]int{}
			for i := 0; i < 6; i++ {
				meta := map[string]any{"selection_id": fmt.Sprint("parallel-", provider, i), "canonical_session_id": fmt.Sprint("agent-", i)}
				got := choose(p, provider, "claude-test", http.Header{"X-Api-Key": {"client"}}, meta, cs, now)
				id, _ := got["AuthID"].(string)
				if id == "" {
					t.Fatal(got)
				}
				seen[id]++
				if i == 0 && id != "urgent" {
					t.Fatal("idle pool lost deadline priority", got)
				}
				if i == 2 && len(seen) != 3 {
					t.Fatal("parallel work concentrated on one account", seen)
				}
			}
			for _, n := range seen {
				if n != 2 {
					t.Fatal("uneven parallel distribution", seen)
				}
			}
			traffic.Lock()
			traffic.Pending = map[string]activeRequest{}
			traffic.Unlock()
		})
	}
}

func TestRejectedUnmeteredRequestsDoNotConsumeAdmissionQuota(t *testing.T) {
	for _, status := range []int{408, 409, 499, 500, 503} {
		if definiteRejection("failed", status) {
			t.Fatal("ambiguous failure was treated as free rejection", status)
		}
	}
	_, c, _, now := admissionFixture(t)
	for i, status := range []int{400, 401, 403, 404, 429} {
		selection := fmt.Sprint("rejected-", i)
		metadata := map[string]any{"selection_id": selection, "selected_auth_id": c.ID}
		raw, _ := json.Marshal(map[string]any{"RequestID": selection, "Metadata": metadata})
		trackRequest("request.intercept_after", raw)
		usage, _ := json.Marshal(map[string]any{"RequestID": selection, "SelectionID": selection, "AuthID": c.ID, "Provider": "claude", "Failed": true})
		observeUsage(usage)
		terminal, _ := json.Marshal(map[string]any{"RequestID": selection, "Metadata": metadata, "Outcome": "failed", "StatusCode": status, "CompletedAt": now})
		trackRequest("request.complete", terminal)
	}
	admission.Lock()
	n := len(admission.Data.Completed)
	admission.Unlock()
	if n != 0 {
		t.Fatal("rejections invented subscription consumption", n)
	}
	usage, _ := json.Marshal(map[string]any{"RequestID": "partial", "SelectionID": "partial", "AuthID": c.ID, "Provider": "claude", "Failed": true, "Detail": map[string]int{"OutputTokens": 1}})
	observeUsage(usage)
	admission.Lock()
	n = len(admission.Data.Completed)
	admission.Unlock()
	if n != 1 {
		t.Fatal("metered partial work was discarded", n)
	}
}

func TestQuotaPreflightDoesNotPromoteGlobalCooldownToEveryModel(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	now := time.Now()
	p := defaultPolicy()
	inventoryFixture(t, []hostAuthFile{{ID: "account", Provider: "claude", Unavailable: true, NextRetry: now.Add(time.Hour)}}, []hostCredentialModels{{ID: "account", Provider: "claude", Models: []string{"opus", "sonnet"}}})
	state.Quota = map[string]snapshot{"account": sample(now, .9, .9, 4*time.Hour)}
	if got := availabilityPreflight(p, availabilityRequest{Model: "opus"}, now); len(got) != 0 {
		t.Fatal("global flag blocked unrelated model before native selection", got)
	}
	state.Quota["account"] = sample(now, 0, .9, time.Hour)
	if got := availabilityPreflight(p, availabilityRequest{Model: "opus"}, now); got["StatusCode"] != 429 {
		t.Fatal("quota exhaustion bypassed", got)
	}
}

func TestAntigravityShortWindowNormalizationAndLegacyReserve(t *testing.T) {
	now := time.Now()
	raw := []byte(`{"groups":[{"displayName":"Gemini Models","buckets":[{"window":"5h","remainingFraction":0.8,"resetTime":"2026-11-05T10:00:00Z"}]}]}`)
	q, err := parseProviderQuota("antigravity", raw, now)
	if err != nil || q.Groups[0].Buckets[0].Window != "five_hour" {
		t.Fatal(q, err)
	}
	old := snapshot{Groups: []group{{Name: "Gemini Models", Buckets: []bucket{{Window: "5h", Remaining: .8, Reset: now.Add(time.Hour)}}}}, Rates: map[string]float64{"Gemini Models|5h": .01}}
	normalized := normalizeQuotaSnapshot("antigravity", old)
	if normalized.Groups[0].Buckets[0].Window != "five_hour" || normalized.Rates["Gemini Models|five_hour"] != .01 || old.Groups[0].Buckets[0].Window != "5h" {
		t.Fatal("cached normalization mutated shared data", normalized, old)
	}
	p := scheduledPolicy()
	base := *p.SharedClaude
	p.ProviderReserves = map[string]*sharedClaudePolicy{"antigravity": &base}
	p.WindowReserves = map[string]map[string]windowReserve{"a": {quotaWindowKey("Gemini Models", "5h"): {Group: "Gemini Models", Window: "5h", Floor: .3}}}
	c := candidate{ID: "a", Provider: "antigravity"}
	if floor := reserveForBucket(p, c, "Gemini Models", normalized.Groups[0].Buckets[0], now); floor != .3 {
		t.Fatal("legacy custom floor lost", floor)
	}
	p.WindowReserves[c.ID][quotaWindowKey("Gemini Models", "5hr")] = windowReserve{Group: "Gemini Models", Window: "5hr", Floor: .4, ReleaseMinutes: 120}
	if floor := reserveForBucket(p, c, "Gemini Models", normalized.Groups[0].Buckets[0], now); floor != .3 {
		t.Fatal("released alias bypassed another active reserve", floor)
	}
}
