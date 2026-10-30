package main

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestPerKeyModelOverride(t *testing.T) {
	t.Chdir(t.TempDir())
	p := policy{Cutoff: .85, MaxAge: 300, Poll: 60, Routes: map[string]target{digest("client-a"): {Provider: "codex", Model: "gpt-6-luna"}}}
	data, _ := json.Marshal(p)
	if err := os.WriteFile("team-routing.json", data, 0600); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(map[string]any{"Headers": http.Header{"X-Api-Key": []string{"client-a"}}, "AvailableProviders": []string{"codex"}})
	result, err := dispatch("model.route", request)
	if err != nil {
		t.Fatal(err)
	}
	route := result.(map[string]any)
	if route["TargetModel"] != "gpt-6-luna" || route["Target"] != "codex" {
		t.Fatal("override not applied")
	}
	request, _ = json.Marshal(map[string]any{"Headers": http.Header{"X-Api-Key": []string{"client-b"}}, "AvailableProviders": []string{"codex"}})
	result, err = dispatch("model.route", request)
	if err != nil || result.(map[string]bool)["Handled"] {
		t.Fatal("override leaked to another key")
	}
}

func TestPolicySelection(t *testing.T) {
	now := time.Now()
	p := policy{Owned: []string{"mine@example.com"}, Cutoff: .85, ExpiringUsed: .5, ExpiringSeconds: 3600, MaxAge: 300}
	candidates := []candidate{{ID: "mine", Provider: "claude", Metadata: map[string]any{"email": "mine@example.com"}}, {ID: "spare", Provider: "claude"}}
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{}
	set := func(id string, used float64, reset time.Time) {
		state.Quota[id] = snapshot{At: now, Groups: []group{{Buckets: []bucket{{Window: "five_hour", Remaining: 1 - used, Reset: reset}}}}}
	}
	pick := func() string {
		r := choose(p, "claude", "opus", http.Header{"Authorization": []string{"Bearer test"}}, nil, candidates, now)
		s, _ := r["AuthID"].(string)
		return s
	}
	set("mine", .2, now.Add(3*time.Hour))
	set("spare", .3, now.Add(3*time.Hour))
	if pick() != "mine" {
		t.Fatal("owner preference")
	}
	set("spare", .3, now.Add(30*time.Minute))
	if pick() != "mine" {
		t.Fatal("active cache binding lost")
	}
	state.Pins = map[string]binding{}
	if pick() != "spare" {
		t.Fatal("expiring spare not selected")
	}
	set("spare", 1, now.Add(31*time.Minute))
	if pick() != "mine" {
		t.Fatal("exhaustion did not override affinity")
	}
	set("mine", 1, now.Add(time.Hour))
	if pick() != "" {
		t.Fatal("exhausted candidates selected")
	}
}
func TestStaleQuotaRejected(t *testing.T) {
	now := time.Now()
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{"a": {At: now.Add(-time.Hour)}}
	r := choose(policy{MaxAge: 300, Cutoff: .85}, "codex", "gpt", nil, nil, []candidate{{ID: "a", Provider: "codex"}}, now)
	if r["Reject"] != true {
		t.Fatal("stale usage allowed")
	}
}

func TestClaudeWeeklyPreference(t *testing.T) {
	now := time.Now()
	p := policy{Owned: []string{"mine@example.com"}, Cutoff: .85, MaxAge: 300}
	for _, window := range []string{"seven_day", "seven_day_opus", "seven_day_sonnet", "weekly"} {
		for _, used := range []float64{.69, .70, .71} {
			state.Pins = map[string]binding{}
			state.Quota = map[string]snapshot{
				"mine":  {At: now, Groups: []group{{Buckets: []bucket{{Window: "five_hour", Remaining: .6}, {Window: window, Remaining: 1 - used}}}}},
				"spare": {At: now, Groups: []group{{Buckets: []bucket{{Window: "five_hour", Remaining: .65}, {Window: window, Remaining: 1 - used}}}}},
			}
			candidates := []candidate{{ID: "mine", Provider: "claude", Metadata: map[string]any{"email": "mine@example.com"}}, {ID: "spare", Provider: "claude"}}
			r := choose(p, "claude", "opus", nil, nil, candidates, now)
			want := "mine"
			if used >= .70 {
				want = "spare"
			}
			if r["AuthID"] != want {
				t.Fatalf("%s usage %.2f: got %v, want %s", window, used, r, want)
			}
			// Removing priority must not disable the account when it is the only option.
			state.Pins = map[string]binding{}
			if r := choose(p, "claude", "opus", nil, nil, candidates[:1], now); r["AuthID"] != "mine" {
				t.Fatal("weekly preference threshold disabled an eligible account")
			}
		}
	}
}

func TestOwnedPreferenceOnlyClaude(t *testing.T) {
	now := time.Now()
	p := policy{Owned: []string{"mine@example.com"}, Cutoff: .85, MaxAge: 300}
	for _, provider := range []string{"codex", "antigravity", "freehub"} {
		state.Pins = map[string]binding{}
		state.Quota = map[string]snapshot{
			"z-mine":  {At: now, Groups: []group{{Name: "gemini", Buckets: []bucket{{Window: "five_hour", Remaining: .6}}}}},
			"a-spare": {At: now, Groups: []group{{Name: "gemini", Buckets: []bucket{{Window: "five_hour", Remaining: .8}}}}},
		}
		candidates := []candidate{{ID: "z-mine", Provider: provider, Metadata: map[string]any{"email": "mine@example.com"}}, {ID: "a-spare", Provider: provider}}
		if r := choose(p, provider, "gemini", nil, nil, candidates, now); r["AuthID"] != "a-spare" {
			t.Fatalf("owner preference leaked to %s: %v", provider, r)
		}
	}
}

func TestMatchingModelUsesBothSubscriptionProviders(t *testing.T) {
	now := time.Now()
	p := policy{Cutoff: .85, ExpiringUsed: .5, ExpiringSeconds: 3600, Poll: 300, MaxAge: 1200}
	cs := []candidate{{ID: "native", Provider: "claude"}, {ID: "google", Provider: "antigravity"}}
	// The core supplies a mixed candidate pool when both advertise this ID.
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{
		"native": {At: now, Groups: []group{{Name: "Claude", Buckets: []bucket{{Window: "five_hour", Remaining: 0}}}}},
		"google": {At: now, Groups: []group{{Name: "Claude Models", Buckets: []bucket{{Window: "weekly", Remaining: .8}}}}},
	}
	if r := choose(p, "", "claude-sonnet-4-6", nil, nil, cs, now); r["AuthID"] != "google" {
		t.Fatal("Antigravity excluded from matching model pool", r)
	}
	state.Quota["native"] = snapshot{At: now, Groups: []group{{Name: "Claude", Buckets: []bucket{{Window: "five_hour", Remaining: .8}}}}}
	state.Quota["google"] = snapshot{At: now, Groups: []group{{Name: "Claude Models", Buckets: []bucket{{Window: "weekly", Remaining: 0}, {Window: "extra_usage", Remaining: 1}}}}}
	if r := choose(p, "", "claude-sonnet-4-6", nil, nil, cs, now); r["AuthID"] != "native" {
		t.Fatal("native fallback failed or credits used", r)
	}
}

func TestResetGraceAndNoCreditFallback(t *testing.T) {
	now := time.Now()
	p := policy{Cutoff: .85, Poll: 300, MaxAge: 1200}
	for _, tc := range []struct {
		name            string
		used            float64
		five, weekly    time.Duration
		weeklyRemaining float64
		age             time.Duration
		allowed         bool
	}{
		{"below cutoff", .84, time.Hour, 48 * time.Hour, .6, 0, true},
		{"cutoff without near reset", .85, time.Hour, 48 * time.Hour, .6, 0, true},
		{"five hour reset", .90, 29 * time.Minute, 48 * time.Hour, .6, 0, true},
		{"weekly reset", .90, time.Hour, 29 * time.Minute, .1, 0, true},
		{"boundary", .85, 30 * time.Minute, 48 * time.Hour, .6, 0, true},
		{"outside boundary", .85, 30*time.Minute + time.Second, 48 * time.Hour, .6, 0, true},
		{"past reset", .9, -time.Minute, 48 * time.Hour, .6, 0, true},
		{"exhausted five hour despite credits", 1, time.Minute, time.Minute, .6, 0, false},
		{"exhausted weekly despite credits", .9, time.Minute, time.Minute, 0, 0, false},
		{"old grace reading", .9, time.Minute, time.Hour, .6, 6 * time.Minute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state.Pins = map[string]binding{}
			state.Quota = map[string]snapshot{"a": {At: now.Add(-tc.age), Groups: []group{{Buckets: []bucket{
				{Window: "five_hour", Remaining: 1 - tc.used, Reset: now.Add(tc.five)},
				{Window: "seven_day", Remaining: tc.weeklyRemaining, Reset: now.Add(tc.weekly)},
				{Window: "extra_usage", Remaining: 1, Reset: now.Add(time.Minute)},
			}}}}}
			r := choose(p, "claude", "opus", nil, nil, []candidate{{ID: "a", Provider: "claude"}}, now)
			if (r["AuthID"] == "a") != tc.allowed {
				t.Fatalf("unexpected selection: %v", r)
			}
		})
	}
}

func TestReserveSelectionAndAffinity(t *testing.T) {
	now := time.Now()
	p := policy{Owned: []string{"mine@example.com"}, Cutoff: .85, Poll: 300, MaxAge: 1200}
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{
		"mine":  {At: now, Groups: []group{{Buckets: []bucket{{Window: "five_hour", Remaining: .1, Reset: now.Add(10 * time.Minute)}}}}},
		"spare": {At: now, Groups: []group{{Buckets: []bucket{{Window: "five_hour", Remaining: .4, Reset: now.Add(time.Hour)}}}}},
	}
	cs := []candidate{{ID: "mine", Provider: "claude", Metadata: map[string]any{"email": "mine@example.com"}}, {ID: "spare", Provider: "claude"}}
	if r := choose(p, "claude", "opus", nil, nil, cs, now); r["AuthID"] != "spare" {
		t.Fatal("reserve preferred over normal capacity", r)
	}
	state.Pins = map[string]binding{}
	choose(p, "claude", "opus", nil, nil, cs[:1], now)
	if r := choose(p, "claude", "opus", nil, nil, cs, now); r["AuthID"] != "mine" {
		t.Fatal("eligible warm reserve lost affinity", r)
	}
}

func TestExpiryRankingBalancesTimeAndHeadroom(t *testing.T) {
	now := time.Now()
	p := policy{Cutoff: .85, ExpiringUsed: .5, ExpiringSeconds: 3600, Poll: 300, MaxAge: 1200}
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{
		"early": {At: now, Groups: []group{{Buckets: []bucket{{Window: "five_hour", Remaining: .51, Reset: now.Add(20 * time.Minute)}}}}},
		"spare": {At: now, Groups: []group{{Buckets: []bucket{{Window: "five_hour", Remaining: 1, Reset: now.Add(25 * time.Minute)}}}}},
	}
	if r := choose(p, "claude", "opus", nil, nil, []candidate{{ID: "early", Provider: "claude"}, {ID: "spare", Provider: "claude"}}, now); r["AuthID"] != "spare" {
		t.Fatal("earliest reset ignored useful remaining capacity", r)
	}
}
