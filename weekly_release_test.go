package main

import (
	"testing"
	"time"
)

func scheduledPolicy() policy {
	p := protectedPolicy()
	p.SharedClaude.WeeklyReleaseMode = "reset_schedule"
	p.SharedClaude.WeeklyReleaseHours = 5
	return p
}

func TestWeeklyReserveResetSchedule(t *testing.T) {
	p := scheduledPolicy()
	for _, tc := range []struct{ reset, release string }{
		{"2026-11-02 11:00", "2026-11-02 06:00"},
		{"2026-11-02 16:00", "2026-11-02 11:00"},
		{"2026-11-02 20:00", "2026-11-02 15:00"},
		{"2026-11-02 20:01", "2026-11-02 20:00"},
		{"2026-11-02 23:59", "2026-11-02 20:00"},
		{"2026-11-03 00:00", "2026-11-02 20:00"},
		{"2026-11-03 10:59", "2026-11-02 20:00"},
		{"2026-11-05 18:00", "2026-11-05 13:00"},
		{"2026-11-06 16:00", "2026-11-05 20:00"},
		{"2026-11-06 20:00", "2026-11-05 20:00"},
		{"2026-11-07 12:00", "2026-11-06 20:00"},
		{"2026-11-07 21:00", "2026-11-07 20:00"},
		{"2026-11-08 11:00", "2026-11-08 06:00"},
		{"2026-11-08 18:00", "2026-11-08 13:00"},
	} {
		t.Run(tc.reset, func(t *testing.T) {
			reset, want := localTime(tc.reset), localTime(tc.release)
			if got := weeklyReserveReleaseAt(p.SharedClaude, reset.UTC()); !got.Equal(want) {
				t.Fatalf("PKT reset %s: got %s, want %s", tc.reset, got, want)
			}
			c := candidate{ID: "scheduled-shared", Provider: "claude"}
			before := want.Add(-time.Second)
			q := sample(before, .8, .14, 4*time.Hour)
			q.Groups[0].Buckets[1].Reset = reset
			if _, ok := rankAccount(p, c, "opus", q, before, 0); ok {
				t.Fatal("weekly reserve unlocked early")
			}
			q.At = want
			if _, ok := rankAccount(p, c, "opus", q, want, 0); !ok {
				t.Fatal("weekly reserve did not unlock at its release instant")
			}
		})
	}
	for _, reset := range []time.Time{localTime("2026-11-02 20:00").Add(59 * time.Second), localTime("2026-11-06 20:00").Add(59 * time.Second)} {
		want := reset.Add(-5 * time.Hour)
		if reset.Weekday() == time.Friday {
			want = localTime("2026-11-05 20:00")
		}
		if got := weeklyReserveReleaseAt(p.SharedClaude, reset); !got.Equal(want) {
			t.Fatalf("end-minute boundary: %s != %s", got, want)
		}
	}
}

func TestScheduledReleasePreservesOtherEligibilityGuards(t *testing.T) {
	p := scheduledPolicy()
	now := localTime("2026-11-02 12:00")
	c := candidate{ID: "scheduled-guards", Provider: "claude"}
	q := sample(now, .2, .14, 3*time.Hour)
	q.Groups[0].Buckets[1].Reset = localTime("2026-11-02 16:00")
	if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatal("weekly unlock bypassed five-hour reserve")
	}
	q.Groups[0].Buckets[0].Remaining = .8
	for _, reset := range []time.Time{time.Time{}, now.Add(-time.Second), now} {
		q.Groups[0].Buckets[1].Reset = reset
		if reserveFloor(p, c, q.Groups[0].Buckets[1], now) != .15 {
			t.Fatal("unknown or passed reset reopened reserve")
		}
	}
	q.Groups[0].Buckets[1].Reset = localTime("2026-11-02 20:00")
	c.Metadata = map[string]any{"email": "mine@example.com"}
	if _, ok := rankAccount(p, c, "opus", q, now, 0); !ok {
		t.Fatal("scheduled reserve applied to owned account")
	}
	p.SharedClaude.WeeklyReleaseMode = "invalid"
	if validateSharedPolicy(p.SharedClaude) == nil {
		t.Fatal("invalid release mode accepted")
	}
}

func TestWeeklyPacingUsesScheduledRelease(t *testing.T) {
	p := scheduledPolicy()
	now := localTime("2026-11-02 08:00")
	c := candidate{ID: "scheduled-pacing", Provider: "claude"}
	q := sample(now, .8, .9, 4*time.Hour)
	q.DemandRate = .01
	q.Groups[0].Buckets[1].Reset = localTime("2026-11-02 16:00")
	rank, ok := rankAccount(p, c, "opus", q, now, 0)
	if !ok || !rank.PacingDeadline.Equal(localTime("2026-11-02 11:00")) {
		t.Fatal("ranking used a different release deadline", rank)
	}
	q.At = localTime("2026-11-02 11:00")
	rank, ok = rankAccount(p, c, "opus", q, q.At, 0)
	if !ok || !rank.PacingDeadline.Equal(q.Groups[0].Buckets[1].Reset) {
		t.Fatal("released reserve still paced toward old unlock", rank)
	}
	legacy := defaultSharedPolicy()
	if got := weeklyReserveReleaseAt(&legacy, q.Groups[0].Buckets[1].Reset); !got.Equal(q.Groups[0].Buckets[1].Reset.Add(-24 * time.Hour)) {
		t.Fatal("legacy portable release changed", got)
	}
}
