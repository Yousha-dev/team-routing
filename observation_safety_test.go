package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestRejectedOverageDoesNotRenewDisabledBilling(t *testing.T) {
	p, c, q, at, now := observationFixture(t)
	p.RequireCreditsOff = true
	h := observationHeaders(q)
	h.Set("Anthropic-Ratelimit-Unified-Overage-Disabled-Reason", "org_spend_cap_reached")
	recordAdmissionSignals(c.ID, h, at, now)
	if rank, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatalf("stale billing setting was renewed by a rejected response: %+v", rank)
	}
}

func TestOverlappingEndpointCannotClearOverageWarning(t *testing.T) {
	for _, strict := range []bool{true, false} {
		t.Run(map[bool]string{true: "strict", false: "best_effort"}[strict], func(t *testing.T) {
			p, c, q, _, now := observationFixture(t)
			p.RequireCreditsOff = strict
			p.AccountOwnership = map[string]bool{c.ID: true}
			q.At = now.Add(-10 * time.Second)
			q.RequestedAt = now.Add(-30 * time.Second)
			h := observationHeaders(q)
			h.Set("Anthropic-Ratelimit-Unified-5h-Utilization", "0.986")
			h.Set("Anthropic-Ratelimit-Unified-Overage-Status", "allowed")
			recordAdmissionSignals(c.ID, h, now.Add(-20*time.Second), now)
			if rank, ok := rankAccount(p, c, "opus", q, now, 0); ok {
				t.Fatalf("overlapping endpoint erased an overage warning or its buffer: %+v", rank)
			}
		})
	}
}

func TestHeaderObservationsCannotRenewExpiredQuotaSchema(t *testing.T) {
	for _, age := range []time.Duration{299 * time.Second, 300 * time.Second, 24 * time.Hour} {
		t.Run(age.String(), func(t *testing.T) {
			p, c, q, at, now := observationFixture(t)
			p.RequireCreditsOff = false
			q.At = now.Add(-age)
			q.RequestedAt = q.At
			recordAdmissionSignals(c.ID, observationHeaders(q), at, now)
			if rank, ok := rankAccount(p, c, "opus", q, now, 0); ok != (age < time.Duration(p.MaxAge)*time.Second) {
				t.Fatalf("quota schema age %s: eligible=%v, rank=%+v", age, ok, rank)
			}
		})
	}
}

func TestClaudeEndpointRequestStartIsRetained(t *testing.T) {
	observationFixture(t)
	var dispatchedAt time.Time
	q, err := fetchProviderQuota(quotaAccount{Provider: "claude"}, func(quotaAccount, string, string, map[string]string, string) (quotaHTTP, error) {
		dispatchedAt = time.Now()
		return quotaHTTP{Code: 200, Body: `{"seven_day":{"utilization":10},"extra_usage":{"is_enabled":false}}`}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if q.RequestedAt.IsZero() || q.RequestedAt.After(dispatchedAt) {
		t.Fatal("endpoint observation was dated after request dispatch", q.RequestedAt, dispatchedAt)
	}
}

func TestLegacyClaudeSnapshotNeedsRequestProvenance(t *testing.T) {
	p, c, q, at, now := observationFixture(t)
	q.At = now.Add(-time.Second)
	q.RequestedAt = time.Time{}
	recordAdmissionSignals(c.ID, observationHeaders(q), at, now)
	if rank, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatalf("legacy completion timestamp established fresh eligibility: %+v", rank)
	}
}

func TestMissingRequestStartCannotRenewQuota(t *testing.T) {
	for _, usage := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct_signal", true: "usage_callback"}[usage], func(t *testing.T) {
			p, c, q, _, now := observationFixture(t)
			p.RequireCreditsOff = false
			if usage {
				now = time.Now()
				q = sample(now.Add(-270*time.Second), .9, .88, 4*time.Hour)
				u := usageEvidence{RequestID: "missing-start", AuthID: c.ID, Provider: c.Provider, Model: "opus", ResponseHeaders: observationHeaders(q)}
				raw, err := json.Marshal(u)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := observeUsage(raw); err != nil {
					t.Fatal(err)
				}
			} else {
				recordAdmissionSignals(c.ID, observationHeaders(q), time.Time{}, now)
			}
			if rank, ok := rankAccount(p, c, "opus", q, now, 0); ok {
				t.Fatalf("missing request provenance established fresh eligibility: %+v", rank)
			}
		})
	}
}

func TestSlowQuotaReadCannotSupersedeOverlappingWarning(t *testing.T) {
	p, c, _, _, now := observationFixture(t)
	p.RequireCreditsOff = true
	clock := now.Add(-30 * time.Second)
	cache := &quotaCache{Entries: map[string]*quotaEntry{}, Now: func() time.Time { return clock }}
	q, err := cache.get(c.ID, time.Minute, func() (snapshot, error) {
		sampled := sample(clock, .9, .88, 4*time.Hour)
		clock = now.Add(-20 * time.Second)
		recordAdmissionSignals(c.ID, http.Header{"Anthropic-Ratelimit-Unified-Overage-Status": {"allowed"}}, clock, clock)
		clock = now.Add(-10 * time.Second)
		return sampled, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if reason := creditBillingReason(p, c, "opus", q, now); reason == "" {
		t.Fatal("quota response completion cleared a warning newer than its request start")
	}
}
