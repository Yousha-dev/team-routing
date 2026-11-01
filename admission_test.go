package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

func admissionFixture(t *testing.T) (policy, candidate, snapshot, time.Time) {
	t.Helper()
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	p := scheduledPolicy()
	p.RequireCreditsOff = false
	raw, _ := json.Marshal(p)
	if err := os.WriteFile("team-routing.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
	now := localTime("2026-11-01 19:00")
	c := candidate{ID: "claude-burst-shared", Provider: "claude"}
	q := sample(now, 1, .7, 4*time.Hour)
	on := true
	q.ExtraUsage = &extraUsageStatus{Enabled: &on}
	return p, c, q, now
}
func quotaHeaders(five, weekly float64, reset, weeklyReset time.Time) http.Header {
	h := http.Header{}
	h.Set("Anthropic-Ratelimit-Unified-5h-Utilization", strconv.FormatFloat(five, 'f', 3, 64))
	h.Set("Anthropic-Ratelimit-Unified-5h-Reset", strconv.FormatInt(reset.Unix(), 10))
	h.Set("Anthropic-Ratelimit-Unified-7d-Utilization", strconv.FormatFloat(weekly, 'f', 3, 64))
	h.Set("Anthropic-Ratelimit-Unified-7d-Reset", strconv.FormatInt(weeklyReset.Unix(), 10))
	return h
}
func TestCompletedBurstCannotReuseCachedAllowance(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	admitted := 0
	for i := 0; i < 441; i++ {
		at := now.Add(time.Duration(i) * 50 * time.Millisecond)
		if _, ok := rankAccount(p, c, "opus", q, at, 0); !ok {
			break
		}
		admitted++
		recordAdmissionCompletion(c.ID, fmt.Sprint("burst-", i), at)
	}
	if admitted == 0 || admitted > 156 {
		t.Fatal("completed work reused unchanged cached allowance", admitted)
	}
	rank, _ := rankAccount(p, c, "opus", q, now.Add(time.Minute), 0)
	if rank.UnreconciledCompletions != admitted || rank.FiveHourProjected < .2-1e-9 {
		t.Fatal("completed debit missing or reserve overdrawn", rank)
	}
	admission.Lock()
	admission.Path = ""
	admission.Unlock()
	if _, ok := rankAccount(p, c, "opus", q, now.Add(time.Minute), 0); ok {
		t.Fatal("restart forgot unpolled consumption")
	}
}
func TestFreshQuotaReconcilesCompletedReservations(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	for i := 0; i < 80; i++ {
		recordAdmissionCompletion(c.ID, fmt.Sprint(i), now)
	}
	next := now.Add(5 * time.Minute)
	q.At = next
	q.Groups[0].Buckets[0].Remaining = .3
	rank, ok := rankAccount(p, c, "opus", q, next, 0)
	if !ok || rank.UnreconciledCompletions != 0 || rank.FiveHourProjected < .299 {
		t.Fatal("fresh quota double charged old completions", rank)
	}
	recordAdmissionCompletion(c.ID, "late", next.Add(-5*time.Second))
	rank, _ = rankAccount(p, c, "opus", q, next, 0)
	if rank.UnreconciledCompletions != 1 {
		t.Fatal("settlement grace missed near-sample request", rank)
	}
}
func TestResponseQuotaStopsBeforeCachedQuota(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	h := quotaHeaders(.79, .3, q.Groups[0].Buckets[0].Reset, q.Groups[0].Buckets[1].Reset)
	if len(recordAdmissionSignals(c.ID, h, now, now)) != 2 {
		t.Fatal("valid native quota headers ignored")
	}
	rank, ok := rankAccount(p, c, "opus", q, now, 0)
	if ok || rank.QuotaSignal == nil || rank.FiveHourProjected > .21 {
		t.Fatal("old healthy cache overrode live reserve threshold", rank)
	}
	// Later/out-of-order higher remaining cannot reopen this window.
	h = quotaHeaders(.2, .1, q.Groups[0].Buckets[0].Reset, q.Groups[0].Buckets[1].Reset)
	recordAdmissionSignals(c.ID, h, now.Add(time.Second), now.Add(time.Second))
	if _, ok = rankAccount(p, c, "opus", q, now.Add(time.Second), 0); ok {
		t.Fatal("higher late signal reopened protected quota")
	}
}
func TestLiveResetKeepsWholeSessionProtectedAfterEight(t *testing.T) {
	p, c, q, _ := admissionFixture(t)
	now := localTime("2026-11-01 20:30")
	q.At = now
	q.Groups[0].Buckets[0].Reset = time.Time{} // Poll still lacks the active reset.
	reset := localTime("2026-11-01 23:00")
	recordAdmissionSignals(c.ID, quotaHeaders(.81, .3, reset, q.Groups[0].Buckets[1].Reset), now, now)
	rank, ok := rankAccount(p, c, "opus", q, now, 0)
	if ok || rank.FiveHourFloor != .2 || !rank.FiveHourReset.Equal(reset) {
		t.Fatal("actual 18-23 session lost protection after 20:00", rank)
	}
}
func TestNewWindowDoesNotInheritOldSignalOrDebt(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	recordAdmissionSignals(c.ID, quotaHeaders(1, .3, q.Groups[0].Buckets[0].Reset, q.Groups[0].Buckets[1].Reset), now, now)
	recordAdmissionCompletion(c.ID, "old-window", now)
	next := localTime("2026-11-01 23:01")
	q.At = next
	q.Groups[0].Buckets[0].Reset = next.Add(5 * time.Hour)
	if _, ok := rankAccount(p, c, "opus", q, next, 0); !ok {
		t.Fatal("new confirmed included window stayed blocked by old signals")
	}
}
func TestAdmissionCompletionDedupeAndConcurrentWork(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	recordAdmissionCompletion(c.ID, "same-request", now)
	recordAdmissionCompletion(c.ID, "same-request", now.Add(time.Millisecond))
	rank, _ := rankAccount(p, c, "opus", q, now.Add(time.Second), 0)
	if rank.UnreconciledCompletions != 1 {
		t.Fatal("usage and terminal callbacks double charged", rank)
	}
	q.Groups[0].Buckets[0].Remaining = .4
	if _, ok := rankAccount(p, c, "opus", q, now, 40); ok {
		t.Fatal("in-flight reservations bypassed completed debt")
	}
}
func TestAdmissionRejectsMalformedSignalsAndCorruptState(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	h := quotaHeaders(.8, .3, q.Groups[0].Buckets[0].Reset, q.Groups[0].Buckets[1].Reset)
	h.Set("Anthropic-Ratelimit-Unified-5h-Utilization", "NaN")
	h.Set("Anthropic-Ratelimit-Unified-7d-Reset", "invalid")
	if len(recordAdmissionSignals(c.ID, h, now, now)) != 0 {
		t.Fatal("malformed signals invented quota")
	}
	if err := os.WriteFile("team-routing-admission.json", []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	admission.Lock()
	admission.Path = ""
	admission.Unlock()
	if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatal("corrupt admission state allowed work")
	}
}

func TestUsageAndLifecycleIDsShareOneSelectionDebit(t *testing.T) {
	_, c, _, _ := admissionFixture(t)
	metadata := map[string]any{"selection_id": "same-selection", "selected_auth_id": c.ID}
	after, _ := json.Marshal(map[string]any{"RequestID": "http-request-id", "Metadata": metadata})
	if _, err := trackRequest("request.intercept_after", after); err != nil {
		t.Fatal(err)
	}
	usage, _ := json.Marshal(map[string]any{"RequestID": "provider-execution-id", "SelectionID": "same-selection", "AuthID": c.ID, "Provider": "claude", "Detail": map[string]int{"InputTokens": 10, "OutputTokens": 1}})
	if _, err := observeUsage(usage); err != nil {
		t.Fatal(err)
	}
	if _, err := trackRequest("request.complete", after); err != nil {
		t.Fatal(err)
	}
	admission.Lock()
	n := len(admission.Data.Completed)
	admission.Unlock()
	if n != 1 {
		t.Fatal("different host request IDs duplicated one execution debit", n)
	}
}

func TestFreshResponseReadingsReconcileEstimatedCompletionDebt(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	for i := 0; i < 250; i++ {
		at := now.Add(time.Duration(i) * time.Second)
		if rank, ok := rankAccount(p, c, "opus", q, at, 0); !ok {
			t.Fatal("fresh provider headroom lost to accumulated estimates", i, rank)
		}
		recordAdmissionCompletion(c.ID, fmt.Sprint(i), at)
		recordAdmissionSignals(c.ID, quotaHeaders(.10, .30, q.Groups[0].Buckets[0].Reset, q.Groups[0].Buckets[1].Reset), at, at)
	}
	rank, ok := rankAccount(p, c, "opus", q, now.Add(250*time.Second), 0)
	if !ok || rank.CompletedReserve > .08 || rank.FiveHourProjected < .8 {
		t.Fatal("response observation did not settle older estimates", rank)
	}
}
