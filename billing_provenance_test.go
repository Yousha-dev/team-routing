package main

import (
	"testing"
	"time"
)

func TestUnusableStartRetainsWarningButNotQuota(t *testing.T) {
	for _, kind := range []string{"old", "future"} {
		t.Run(kind, func(t *testing.T) {
			p, c, q, _, now := observationFixture(t)
			at := now.Add(-(ledgerRetentionSeconds + 1) * time.Second)
			if kind == "future" {
				at = now.Add((quotaResetJitterSeconds + 1) * time.Second)
			}
			headers := observationHeaders(q)
			headers.Set("Anthropic-Ratelimit-Unified-Overage-Status", "allowed")
			if signals := recordAdmissionSignals(c.ID, headers, at, now); len(signals) != 0 {
				t.Fatalf("invalid start established quota: %+v", signals)
			}
			p.RequireCreditsOff = true
			q.At = now.Add(-time.Minute)
			q.RequestedAt = q.At
			if rank, ok := rankAccount(p, c, "opus", q, now, 0); ok {
				t.Fatalf("warning ignored: %+v", rank)
			}
			admission.Lock()
			admission.Path = ""
			admission.Unlock()
			if rank, ok := rankAccount(p, c, "opus", q, now, 0); ok {
				t.Fatalf("warning lost after reload: %+v", rank)
			}
			p.RequireCreditsOff = false
			q.At = now.Add(-270 * time.Second)
			q.RequestedAt = q.At
			if rank, ok := rankAccount(p, c, "opus", q, now, 0); ok {
				t.Fatalf("invalid start renewed quota: %+v", rank)
			}
		})
	}
}
