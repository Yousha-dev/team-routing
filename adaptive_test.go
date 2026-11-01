package main

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func sample(now time.Time, five, weekly float64, reset time.Duration) snapshot {
	off := false
	return snapshot{At: now, ExtraUsage: &extraUsageStatus{Enabled: &off}, Groups: []group{{Name: "Claude", Buckets: []bucket{
		{Window: "five_hour", Remaining: five, Reset: now.Add(reset)},
		{Window: "seven_day", Remaining: weekly, Reset: now.Add(72 * time.Hour)},
	}}}}
}
func TestAdaptiveRateLearningAndReset(t *testing.T) {
	now := time.Now()
	old := sample(now.Add(-time.Hour), .8, .9, time.Hour)
	next := old
	next.Groups = []group{{Name: "Claude", Buckets: append([]bucket(nil), old.Groups[0].Buckets...)}}
	next.Groups[0].Buckets[0].Remaining = .6
	rates := estimateRates(old, next, now)
	if math.Abs(rates["Claude|five_hour"]-.2) > 1e-9 {
		t.Fatal(rates)
	}
	next.Groups[0].Buckets[0].Remaining = 1
	if r := estimateRates(old, next, now); r["Claude|five_hour"] != 0 {
		t.Fatal("reset retained burn", r)
	}
	next.Groups[0].Buckets[0].Remaining = .6
	next.Groups[0].Buckets[0].Reset = now.Add(5 * time.Hour)
	if r := estimateRates(old, next, now); r["Claude|five_hour"] != 0 {
		t.Fatal("changed window retained burn", r)
	}
}
func TestAdaptiveSoftCutoffAndExhaustion(t *testing.T) {
	now := time.Now()
	p := policy{Cutoff: .85, Poll: 300, MaxAge: 1200, ExpiringSeconds: 3600, ExpiringUsed: .5}
	c := candidate{ID: "a", Provider: "claude"}
	a, _ := rankAccount(p, c, "opus", sample(now, .151, .8, 2*time.Hour), now, 0)
	b, ok := rankAccount(p, c, "opus", sample(now, .149, .8, 2*time.Hour), now, 0)
	if !ok || math.Abs(a.Score-b.Score) > .03 {
		t.Fatal("hard cutoff cliff", a, b, ok)
	}
	if _, ok := rankAccount(p, c, "opus", sample(now, .05, .8, 2*time.Hour), now, 0); !ok {
		t.Fatal("non-exhausted quota stranded")
	}
	if _, ok := rankAccount(p, c, "opus", sample(now, 0, .8, time.Minute), now, 0); ok {
		t.Fatal("exhausted quota selected")
	}
}
func TestAdaptiveWeeklyRunwayAndInflight(t *testing.T) {
	now := time.Now()
	p := policy{Cutoff: .85, Poll: 300, MaxAge: 1200, ExpiringSeconds: 3600, ExpiringUsed: .5}
	c := candidate{ID: "a", Provider: "claude"}
	healthy := sample(now, .7, .8, 2*time.Hour)
	pressured := sample(now, .7, .1, 2*time.Hour)
	pressured.Rates = map[string]float64{"Claude|seven_day": .02}
	a, _ := rankAccount(p, c, "opus", healthy, now, 0)
	b, _ := rankAccount(p, c, "opus", pressured, now, 0)
	if a.Score <= b.Score {
		t.Fatal("weekly runway ignored")
	}
	busy, _ := rankAccount(p, c, "opus", healthy, now, 5)
	if busy.Score >= a.Score || busy.Remaining >= a.Remaining {
		t.Fatal("active work ignored")
	}
	old := sample(now.Add(-10*time.Minute), .05, .8, time.Hour)
	old.Rates = map[string]float64{"Claude|five_hour": .4}
	if _, ok := rankAccount(p, c, "opus", old, now, 0); ok {
		t.Fatal("projected exhaustion allowed")
	}
}
func TestAdaptiveAffinityHysteresis(t *testing.T) {
	now := time.Now()
	p := policy{Cutoff: .85, Poll: 300, MaxAge: 1200, ExpiringSeconds: 3600, ExpiringUsed: .5}
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{"a": sample(now, .8, .8, 3*time.Hour)}
	cs := []candidate{{ID: "a", Provider: "claude"}, {ID: "b", Provider: "claude"}}
	choose(p, "claude", "opus", nil, nil, cs[:1], now)
	state.Quota["a"] = sample(now, .05, .1, 4*time.Hour)
	state.Quota["b"] = sample(now, 1, 1, 10*time.Minute)
	if r := choose(p, "claude", "opus", nil, nil, cs, now.Add(5*time.Minute)); r["AuthID"] != "b" {
		t.Fatal("low-runway rebalance missed", r)
	}
	// Refresh samples without changing the binding's original age.
	state.Quota["a"] = sample(now.Add(11*time.Minute), .05, .1, 4*time.Hour)
	state.Quota["b"] = sample(now.Add(11*time.Minute), 1, 1, 10*time.Minute)
	if r := choose(p, "claude", "opus", nil, nil, cs, now.Add(11*time.Minute)); r["AuthID"] != "b" {
		t.Fatal("material rebalance missed", r)
	}
}
func TestAdaptiveLifecycleTracking(t *testing.T) {
	traffic.Lock()
	traffic.Active = map[string]activeRequest{}
	traffic.Completed = 0
	traffic.Unlock()
	defer func() {
		traffic.Lock()
		traffic.Active = map[string]activeRequest{}
		traffic.Completed = 0
		traffic.Unlock()
	}()
	raw, _ := json.Marshal(map[string]any{"RequestID": "test", "Metadata": map[string]any{"selected_auth_id": "a"}})
	r, e := trackRequest("request.intercept_after", raw)
	if e != nil || len(r.(map[string]any)) != 0 || activeCounts(time.Now())["a"] != 1 {
		t.Fatal("tracking failed or mutated request")
	}
	trackRequest("request.intercept_after", raw)
	if activeCounts(time.Now())["a"] != 1 {
		t.Fatal("retry counted twice")
	}
	trackRequest("request.complete", raw)
	trackRequest("request.complete", raw)
	if activeCounts(time.Now())["a"] != 0 || traffic.Completed != 1 {
		t.Fatal("completion accounting")
	}
}
