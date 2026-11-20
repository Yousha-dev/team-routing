package main

import (
	"fmt"
	"testing"
	"time"
)

func bestEffortFixture(t *testing.T) (policy, candidate, snapshot, time.Time) {
	t.Helper()
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	p := protectedPolicy()
	p.RequireCreditsOff = true
	// Saturday afternoon: the five-hour reserve is inactive. The weekly
	// reserve still applies, including to fallback.
	now := localTime("2026-11-07 12:00")
	q := sample(now.Add(-2*time.Minute), .25, .6, 4*time.Hour)
	q.Rates = map[string]float64{"Claude|five_hour": 12}
	return p, candidate{ID: "claude-forecast", Provider: "claude"}, q, now
}

func TestForecastFallbackOnlyWhenNoOrdinaryCandidate(t *testing.T) {
	p, c, q, now := bestEffortFixture(t)
	other := candidate{ID: "normal", Provider: "claude"}
	if r, ok := rankAccount(p, c, "opus", q, now, 0); ok || r.Reason != "Projected five_hour quota exhausted" {
		t.Fatalf("fixture must be blocked solely by the forecast: %+v", r)
	}
	p.AccountPriority = map[string]int{c.ID: 10}
	state.Quota = map[string]snapshot{c.ID: q, other.ID: sample(now, .5, .5, 4*time.Hour)}
	rows := assessPool(p, []candidate{c, other}, "opus", now, nil, 0)
	if rows[0].Eligible || !rows[1].Eligible || rows[1].BestEffort {
		t.Fatalf("ordinary candidate must take precedence: %+v", rows)
	}
	inventoryFixture(t, []hostAuthFile{{ID: c.ID, Provider: "claude"}}, []hostCredentialModels{{ID: c.ID, Provider: "claude", Models: []string{"opus"}}})
	if r := availabilityPreflight(p, availabilityRequest{Model: "opus"}, now); len(r) != 0 {
		t.Fatalf("preflight rejected forecast-only fallback: %v", r)
	}
	if got := adaptiveChoose(p, "claude", "opus", nil, nil, []candidate{c}, now); got["AuthID"] != c.ID {
		t.Fatalf("selection did not choose the forecast-only fallback: %v", got)
	}
	if got := accountEligibility(p, c, q, now, 0); got != bestEffortEligibility {
		t.Fatalf("dashboard detail = %q", got)
	}
}

func TestForecastFallbackChargesCompletedAndActiveWork(t *testing.T) {
	p, c, q, now := bestEffortFixture(t)
	if r, ok := bestEffortRank(p, c, "opus", q, now, 0); !ok || !r.BestEffort {
		t.Fatalf("expected initial fallback headroom: %+v", r)
	}
	if _, ok := bestEffortRank(p, c, "opus", q, now, 2); ok {
		t.Fatal("fallback ignored active reservations")
	}
	for i := range 60 {
		recordAdmissionCompletion(c.ID, fmt.Sprintf("request-%d", i), now)
	}
	if r, ok := bestEffortRank(p, c, "opus", q, now, 0); ok || r.UnreconciledCompletions != 60 {
		t.Fatalf("fallback reused allowance already reserved for completed work: %+v", r)
	}
	inventoryFixture(t, []hostAuthFile{{ID: c.ID, Provider: "claude"}}, []hostCredentialModels{{ID: c.ID, Provider: "claude", Models: []string{"opus"}}})
	state.Quota = map[string]snapshot{c.ID: q}
	if r := availabilityPreflight(p, availabilityRequest{Model: "opus"}, now); r["StatusCode"] != 429 {
		t.Fatalf("preflight failed to reject spent headroom: %v", r)
	}
	if got := adaptiveChoose(p, "claude", "opus", nil, nil, []candidate{c}, now); got["Reject"] != true {
		t.Fatalf("selection failed to reject spent headroom: %v", got)
	}
}

func TestForecastFallbackKeepsHardLimits(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*snapshot, time.Time)
	}{
		{"weekly reserve", func(q *snapshot, _ time.Time) { q.Groups[0].Buckets[1].Remaining = .14 }},
		{"exhaustion", func(q *snapshot, _ time.Time) { q.Groups[0].Buckets[0].Remaining = 0 }},
		{"missing weekly", func(q *snapshot, _ time.Time) { q.Groups[0].Buckets = q.Groups[0].Buckets[:1] }},
		{"stale quota", func(q *snapshot, now time.Time) { q.At, q.RequestedAt = now.Add(-time.Hour), now.Add(-time.Hour) }},
		{"credits enabled", func(q *snapshot, _ time.Time) { on := true; q.ExtraUsage = &extraUsageStatus{Enabled: &on} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, c, q, now := bestEffortFixture(t)
			tc.change(&q, now)
			if r, ok := bestEffortRank(p, c, "opus", q, now, 0); ok {
				t.Fatalf("fallback bypassed %s: %+v", tc.name, r)
			}
		})
	}
}

func TestForecastFallbackStillHonorsRestrictionAcrossSameWindowPoll(t *testing.T) {
	p, c, q, now := bestEffortFixture(t)
	// Completion can follow the endpoint poll even if inference started first.
	// A later poll timestamp alone must not reopen exhausted quota.
	h := quotaHeaders(1, .1, q.Groups[0].Buckets[0].Reset, q.Groups[0].Buckets[1].Reset)
	recordAdmissionSignals(c.ID, h, q.RequestedAt.Add(-time.Minute), now)
	if r, ok := bestEffortRank(p, c, "opus", q, now, 0); ok || r.QuotaSignal == nil {
		t.Fatalf("fallback ignored same-window exhaustion: %+v", r)
	}
}

func TestForecastFallbackLeavesOtherProvidersUnchanged(t *testing.T) {
	p, _, q, now := bestEffortFixture(t)
	if _, ok := bestEffortRank(p, candidate{ID: "codex", Provider: "codex"}, "gpt", q, now, 0); ok {
		t.Fatal("fallback applied outside the Claude pool")
	}
}
