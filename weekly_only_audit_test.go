package main

import (
	"testing"
	"time"
)

func TestWeeklyOnlyCustomReserveAndStrictBilling(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	p.RequireCreditsOff = true
	p.AccountReserves = map[string]accountReserve{c.ID: {FiveHourFloor: .3, WeeklyFloor: .2}}
	off := false
	q.ExtraUsage = &extraUsageStatus{Enabled: &off}
	q.Groups[0].Buckets = []bucket{{Window: "seven_day", Remaining: .19, Reset: now.Add(72 * time.Hour)}}
	if r, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatalf("custom weekly floor bypassed: %+v", r)
	}
	q.Groups[0].Buckets[0].Remaining = .6
	if r, ok := rankAccount(p, c, "opus", q, now, 0); !ok {
		t.Fatalf("usable weekly-only account blocked: %+v", r)
	}
	q.Groups[0].Buckets = append(q.Groups[0].Buckets, bucket{Window: "seven_day_opus", Remaining: 0, Reset: now.Add(72 * time.Hour)})
	if r, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatalf("model-specific exhausted weekly bucket bypassed: %+v", r)
	}
	q.Groups[0].Buckets = []bucket{{Window: "seven_day_opus", Remaining: .8, Reset: now.Add(72 * time.Hour)}}
	if r, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatalf("family-only quota admitted without global weekly reading: %+v", r)
	}
	q.Groups[0].Buckets = []bucket{{Window: "seven_day", Remaining: .6, Reset: now.Add(72 * time.Hour)}}
	on := true
	q.ExtraUsage.Enabled = &on
	if r, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatalf("enabled credits admitted: %+v", r)
	}
	q.ExtraUsage.Enabled = &off
	if r, ok := rankAccount(p, c, "opus", q, now.Add(time.Duration(p.Poll+61)*time.Second), 0); ok {
		t.Fatalf("stale quota and billing admitted: %+v", r)
	}
}
