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

func protectedPolicy() policy {
	p := defaultPolicy()
	s := defaultSharedPolicy()
	s.Enabled = true
	s.Timezone = "Asia/Karachi"
	p.SharedClaude = &s
	p.Owned = []string{"mine@example.com"}
	return p
}
func localTime(s string) time.Time {
	loc, _ := time.LoadLocation("Asia/Karachi")
	t, _ := time.ParseInLocation("2006-01-02 15:04", s, loc)
	return t
}
func TestProtectedFiveHourWindows(t *testing.T) {
	s := defaultSharedPolicy()
	s.Timezone = "Asia/Karachi"
	cases := []struct {
		at, end string
		want    bool
	}{
		{"2026-11-02 07:00", "2026-11-02 12:00", true},
		{"2026-11-02 10:00", "2026-11-02 15:00", true},
		{"2026-11-02 19:59", "2026-11-03 00:59", true},
		{"2026-11-02 21:00", "2026-11-03 02:00", false},
		{"2026-11-06 02:00", "2026-11-06 07:00", false},
		{"2026-11-07 12:00", "2026-11-07 17:00", false},
		{"2026-11-08 07:00", "2026-11-08 12:00", true},
		{"2026-11-02 06:00", "2026-11-02 11:00", true},
		{"2026-11-02 20:00", "2026-11-03 01:00", true},
	}
	for _, c := range cases {
		if got := protectedFiveHourWindow(&s, localTime(c.end), localTime(c.at)); got != c.want {
			t.Errorf("%s -> %s: %v", c.at, c.end, got)
		}
	}
	if !protectedFiveHourWindow(&s, time.Time{}, localTime("2026-11-02 07:00")) {
		t.Fatal("unstarted session bypassed upcoming protected hours")
	}
}
func TestSharedWeeklyReserveAndOwnerExemption(t *testing.T) {
	p := protectedPolicy()
	now := localTime("2026-11-07 02:00")
	c := candidate{ID: "shared", Provider: "claude", Metadata: map[string]any{"email": "shared@example.com"}}
	for _, hours := range []int{25, 48, 72, 100} {
		q := sample(now, .8, .149, 4*time.Hour)
		q.Groups[0].Buckets[1].Reset = now.Add(time.Duration(hours) * time.Hour)
		if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
			t.Errorf("shared low weekly allowed with %dh until reset", hours)
		}
		own := c
		own.Metadata = map[string]any{"email": "mine@example.com"}
		if _, ok := rankAccount(p, own, "opus", q, now, 0); !ok {
			t.Fatal("own Claude account incorrectly reserved")
		}
		other := c
		other.Provider = "codex"
		if _, ok := rankAccount(p, other, "opus", q, now, 0); !ok {
			t.Fatal("non-Claude provider incorrectly reserved")
		}
	}
	for _, hours := range []int{1, 23, 24} {
		q := sample(now, .8, .05, 4*time.Hour)
		q.Groups[0].Buckets[1].Reset = now.Add(time.Duration(hours) * time.Hour)
		if _, ok := rankAccount(p, c, "opus", q, now, 0); !ok {
			t.Errorf("near-reset weekly tail stranded at%dh", hours)
		}
	}
	q := sample(now, .8, .14, 4*time.Hour)
	q.Groups[0].Buckets[1].Reset = time.Time{}
	if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatal("unknown weekly reset reopened reserve")
	}
}
func TestSharedFiveHourReserveOverridesAffinityAndGrace(t *testing.T) {
	p := protectedPolicy()
	now := localTime("2026-11-02 12:00")
	c := candidate{ID: "shared", Provider: "claude"}
	for _, reset := range []time.Duration{20 * time.Minute, 4 * time.Hour} {
		q := sample(now, .20, .8, reset)
		if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
			t.Fatal("hard20% reserve bypassed", reset)
		}
	}
	q := sample(now, .23, .8, 4*time.Hour)
	if _, ok := rankAccount(p, c, "opus", q, now, 0); !ok {
		t.Fatal("usable headroom unexpectedly blocked")
	}
	if _, ok := rankAccount(p, c, "opus", q, now, 6); ok {
		t.Fatal("concurrency reserve ignored")
	}
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{"shared": sample(now, .19, .8, 20*time.Minute), "mine": sample(now, .8, .8, 3*time.Hour)}
	cs := []candidate{c, {ID: "mine", Provider: "claude", Metadata: map[string]any{"email": "mine@example.com"}}}
	pk := digest("|key-fallback|claude|opus")
	state.Pins[pk] = binding{ID: "shared", At: now, BoundAt: now}
	if r := choose(p, "claude", "opus", nil, nil, cs, now); r["AuthID"] != "mine" {
		t.Fatal("affinity bypassed hard reserve", r)
	}
}
func TestRateLearningToleratesObservedResetJitter(t *testing.T) {
	now := time.Now()
	old := sample(now.Add(-5*time.Minute), .8, .6, 4*time.Hour)
	next := sample(now, .7, .59, 4*time.Hour)
	for i := range next.Groups[0].Buckets {
		next.Groups[0].Buckets[i].Reset = old.Groups[0].Buckets[i].Reset.Add(750 * time.Millisecond)
	}
	rates := estimateRates(old, next, now)
	if math.Abs(rates["Claude|five_hour"]-1.2) > .00001 || rates["Claude|seven_day"] <= 0 {
		t.Fatal("observed provider jitter erased rates", rates)
	}
	next.Groups[0].Buckets[0].Reset = old.Groups[0].Buckets[0].Reset.Add(5 * time.Hour)
	if _, ok := estimateRates(old, next, now)["Claude|five_hour"]; ok {
		t.Fatal("real reset retained old rate")
	}
}

func TestSharedReservesRequireCompleteRecentQuota(t *testing.T) {
	p := protectedPolicy()
	now := localTime("2026-11-02 12:00")
	c := candidate{ID: "shared", Provider: "claude"}
	q := sample(now.Add(-7*time.Minute), .8, .8, 4*time.Hour)
	if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatal("shared reserve used old reading")
	}
	q = sample(now, .8, .8, 4*time.Hour)
	q.Groups[0].Buckets = q.Groups[0].Buckets[:1]
	if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatal("missing weekly window bypassed reserve")
	}
}
func TestSelectionReservationsPreventConcurrentOverAdmission(t *testing.T) {
	p := protectedPolicy()
	now := localTime("2026-11-02 12:00")
	c := candidate{ID: "shared", Provider: "claude"}
	traffic.Lock()
	traffic.Active = map[string]activeRequest{}
	traffic.Pending = map[string]activeRequest{}
	traffic.Unlock()
	defer func() { traffic.Lock(); traffic.Pending = map[string]activeRequest{}; traffic.Unlock() }()
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{"shared": sample(now, .235, .8, 4*time.Hour)}
	admitted := 0
	for _, id := range []string{"pick1", "pick2", "pick3", "pick4", "pick5", "pick6"} {
		if r := choose(p, "claude", "opus", nil, map[string]any{"selection_id": id}, []candidate{c}, now); r["AuthID"] == "shared" {
			admitted++
		}
	}
	if admitted == 0 || admitted > 3 {
		t.Fatalf("unbounded reservations:%d", admitted)
	}
	if activeCounts(now)["shared"] != admitted {
		t.Fatal("pending reservations missing")
	}
	raw, _ := json.Marshal(map[string]any{"RequestID": "req1", "Metadata": map[string]any{"selection_id": "pick1", "selected_auth_id": "shared"}})
	trackRequest("request.intercept_after", raw)
	// Scheduling uses a deterministic workday clock; align the lifecycle sample.
	traffic.Lock()
	observed := traffic.Active["req1"]
	observed.At = now
	traffic.Active["req1"] = observed
	traffic.Unlock()
	if activeCounts(now)["shared"] != admitted {
		t.Fatal("handoff double-counted selection")
	}
	trackRequest("request.complete", raw)
}
func TestThreadNegotiationAndPolicyValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	p := protectedPolicy()
	p.StatelessThreads = true
	data, _ := json.Marshal(p)
	if err := os.WriteFile("team-routing.json", data, 0600); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"Headers":{"Anthropic-Beta":["oauth-2025-04-20,message-threads-2026-08-12"]}}`)
	result, err := interceptBefore(raw)
	if err != nil {
		t.Fatal(err)
	}
	response := result.(map[string]any)
	if response["Terminate"] != true || response["StatusCode"] != 400 || !strings.Contains(string(response["ResponseBody"].([]byte)), "Unsupported beta") {
		t.Fatal("missing recoverable negotiation", response)
	}
	result, err = interceptBefore([]byte(`{"Headers":{"Anthropic-Beta":["oauth-2025-04-20"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if r := result.(map[string]any); r["Body"] != nil || r["Terminate"] == true || r["Headers"].(http.Header).Get(affinityHeader) == "" {
		t.Fatal("ordinary full-history payload modified or affinity hint missing")
	}
	s := defaultSharedPolicy()
	s.Timezone = "No/Such_Zone"
	if validateSharedPolicy(&s) == nil {
		t.Fatal("invalid timezone accepted")
	}
}
