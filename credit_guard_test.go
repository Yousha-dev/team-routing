package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCreditFlagsParsedSeparatelyFromIncludedQuota(t *testing.T) {
	now := time.Now()
	for _, raw := range []string{
		`{"five_hour":{"utilization":0},"seven_day":{"utilization":0},"extra_usage":{"is_enabled":true,"used_credits":337,"currency":"USD","decimal_places":2}}`,
		`{"five_hour":{"utilization":0},"seven_day":{"utilization":0},"extra_usage":{"is_enabled":false,"user_disabled":true}}`,
		`{"five_hour":{"utilization":0},"seven_day":{"utilization":0}}`,
	} {
		q, e := parseProviderQuota("claude", []byte(raw), now)
		if e != nil {
			t.Fatal(e)
		}
		if len(q.Groups[0].Buckets) != 2 {
			t.Fatal("paid credit became quota", q)
		}
		if strings.Contains(raw, `"is_enabled":true`) && (q.ExtraUsage == nil || q.ExtraUsage.Enabled == nil || !*q.ExtraUsage.Enabled || *q.ExtraUsage.UsedCredits != 337) {
			t.Fatal("credit status lost", q)
		}
		if !strings.Contains(raw, "extra_usage") && q.ExtraUsage != nil {
			t.Fatal("unknown credit setting invented")
		}
	}
}
func TestCreditGuardFailsClosedEvenForOwnedHealthyQuota(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	now := time.Now()
	p := defaultPolicy()
	p.Owned = []string{"mine@example.com"}
	c := candidate{ID: "strict-owned", Provider: "claude", Metadata: map[string]any{"email": "mine@example.com"}}
	healthy := sample(now, 1, 1, 5*time.Hour)
	if _, ok := rankAccount(p, c, "opus", healthy, now, 0); !ok {
		t.Fatal("fresh explicit disabled flag not accepted")
	}
	on := true
	unknown := healthy
	unknown.ExtraUsage = nil
	enabled := healthy
	enabled.ExtraUsage = &extraUsageStatus{Enabled: &on}
	invalid := healthy
	invalid.ExtraUsage = &extraUsageStatus{}
	stale := healthy
	stale.At = now.Add(-time.Duration(p.Poll+61) * time.Second)
	stale.RequestedAt = stale.At
	for _, q := range []snapshot{unknown, enabled, invalid, stale} {
		if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
			t.Fatal("credits/unknown/stale bypassed guard", q)
		}
	}
}
func TestPaidRequestsRejectedBeforeInferenceWithGenericMessage(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	p := defaultPolicy()
	// Disabling the provider-toggle requirement must not enable paid requests.
	p.RequireCreditsOff = false
	policyJSON, _ := json.Marshal(p)
	_ = os.WriteFile("team-routing.json", policyJSON, 0600)
	for _, body := range []string{`{"speed":"fast"}`, `{"service_tier":"priority"}`, `{"fallback_credit_token":"private-token"}`, `{"fallbacks":[{"model":"paid"}]}`} {
		raw, _ := json.Marshal(availabilityRequest{Body: []byte(body)})
		response, err := interceptBefore(raw)
		if err != nil {
			t.Fatal(err)
		}
		r := response.(map[string]any)
		if r["Terminate"] != true || r["StatusCode"] != 429 || strings.Contains(string(r["ResponseBody"].([]byte)), "private-token") || strings.Contains(string(r["ResponseBody"].([]byte)), "credit") {
			t.Fatal("paid request was sent or internal detail leaked", r)
		}
	}
	if paidRequest([]byte(`{"messages":[],"speed":"standard","fallbacks":[]}`)) {
		t.Fatal("ordinary included request rejected")
	}
}
func TestObservedOverageCreatesDurableAccountFence(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	now := time.Now()
	p := defaultPolicy()
	p.RequireCreditsOff = false
	c := candidate{ID: "overage-observed", Provider: "claude"}
	q := sample(now, 1, 1, 5*time.Hour)
	if overageInUse(http.Header{"Anthropic-Ratelimit-Unified-Overage-Status": {"allowed"}}) {
		t.Fatal("available credits confused with actual spending")
	}
	raw, _ := json.Marshal(map[string]any{"RequestID": "paid-proof", "Provider": "claude", "AuthID": c.ID, "ResponseHeaders": http.Header{"Anthropic-Ratelimit-Unified-Overage-In-Use": {"true"}}})
	if _, e := observeUsage(raw); e != nil {
		t.Fatal(e)
	}
	if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatal("paid response did not fence future requests")
	}
	creditFences.Lock()
	creditFences.Path = ""
	creditFences.Unlock()
	if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatal("restart cleared paid-use fence")
	}
	if !strings.Contains(accountEligibility(p, c, q, now, 0), "newer window") {
		t.Fatal("fence not visible privately")
	}
}

func TestBestEffortSubscriptionModeKeepsQuotaGuards(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	now := time.Now()
	p := defaultPolicy()
	p.RequireCreditsOff = false
	p.Owned = []string{"mine@example.com"}
	c := candidate{ID: "best-effort-owned", Provider: "claude", Metadata: map[string]any{"email": "mine@example.com"}}
	on := true
	healthy := sample(now, .8, .5, 5*time.Hour)
	healthy.ExtraUsage = &extraUsageStatus{Enabled: &on}
	if _, ok := rankAccount(p, c, "opus", healthy, now, 0); !ok {
		t.Fatal("provider toggle blocked usable subscription quota")
	}
	for _, fractions := range [][2]float64{{0, .5}, {.8, 0}, {.004, .5}, {.8, .0005}} {
		q := sample(now, fractions[0], fractions[1], 5*time.Hour)
		q.ExtraUsage = healthy.ExtraUsage
		if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
			t.Fatal("exhausted quota or next-request buffer admitted", fractions)
		}
	}
	stale := healthy
	stale.At = now.Add(-time.Duration(p.Poll+61) * time.Second)
	stale.RequestedAt = stale.At
	if _, ok := rankAccount(p, c, "opus", stale, now, 0); ok {
		t.Fatal("stale quota admitted in best-effort mode")
	}
	incomplete := healthy
	incomplete.Groups = nil
	if _, ok := rankAccount(p, c, "opus", incomplete, now, 0); ok {
		t.Fatal("missing included quota admitted")
	}
	shared := candidate{ID: "best-effort-shared", Provider: "claude"}
	s := defaultSharedPolicy()
	s.Enabled = true
	s.Timezone = "Asia/Karachi"
	p.SharedClaude = &s
	sunday := time.Date(2026, 11, 1, 7, 0, 0, 0, time.UTC) // Noon PKT.
	for _, fractions := range [][2]float64{{.8, .15}, {.20, .5}} {
		q := sample(sunday, fractions[0], fractions[1], 4*time.Hour)
		q.ExtraUsage = healthy.ExtraUsage
		if _, ok := rankAccount(p, shared, "opus", q, sunday, 0); ok {
			t.Fatal("best-effort mode bypassed shared reserve", fractions)
		}
	}
}
