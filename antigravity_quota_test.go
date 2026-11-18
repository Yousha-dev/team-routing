package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

const argonQuotaFixture = `{"groups":[{"displayName":"Gemini Models","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.99999243,"resetTime":"2026-11-25T08:16:42Z"},{"bucketId":"gemini-5h","window":"5h","remainingFraction":0.9999736,"resetTime":"2026-11-18T13:16:42Z"}]},{"displayName":"Gemini 4 Argon","buckets":[{"bucketId":"gemini-argon-wtu-weekly","window":"weekly","remainingFraction":0.8760508,"resetTime":"2026-11-25T07:59:03Z"}]}]}`

const argonCatalogFixture = `{"models":{"gemini-4-argon-high":{"quotaInfo":{"remainingFraction":0.876176,"resetTime":"2026-11-25T07:59:03Z"}},"gemini-4-argon-medium":{"quotaInfo":{"remainingFraction":0.876176,"resetTime":"2026-11-25T07:59:03Z"}},"gemini-4-argon-low":{"quotaInfo":{"remainingFraction":0.876176,"resetTime":"2026-11-25T07:59:03Z"}},"gemini-3.8-flash-high":{"quotaInfo":{"remainingFraction":0.9999736,"resetTime":"2026-11-18T13:16:42Z"}},"gemini-4-hydrogen-xhigh":{"quotaInfo":{"remainingFraction":0.9999736,"resetTime":"2026-11-18T13:16:42Z"}}}}`

func TestAntigravityManifestAgentAndQuotaCohort(t *testing.T) {
	oldHost := hostCall
	antigravityAgent.Lock()
	oldValue, oldNext := antigravityAgent.value, antigravityAgent.next
	antigravityAgent.value, antigravityAgent.next = "", time.Time{}
	antigravityAgent.Unlock()
	t.Cleanup(func() {
		hostCall = oldHost
		antigravityAgent.Lock()
		antigravityAgent.value, antigravityAgent.next = oldValue, oldNext
		antigravityAgent.Unlock()
	})
	manifestCalls, quotaCalls := 0, 0
	hostCall = func(method string, input any) (json.RawMessage, error) {
		if method != "host.http.do" {
			return nil, fmt.Errorf("unexpected host method %s", method)
		}
		raw, _ := json.Marshal(input)
		var req struct {
			URL     string
			Headers map[string][]string
		}
		json.Unmarshal(raw, &req)
		body := argonQuotaFixture
		if req.URL == antigravityManifest {
			manifestCalls++
			if len(req.Headers["Authorization"]) != 0 {
				t.Fatal("credential sent to public manifest")
			}
			body = "version: 2.32.2\n"
		} else {
			quotaCalls++
			if strings.HasSuffix(req.URL, ":fetchAvailableModels") {
				body = argonCatalogFixture
			}
			if got := req.Headers["User-Agent"]; len(got) != 1 || got[0] != "antigravity/hub/2.32.2 (aidev_client; os_type=darwin; arch=arm64)" {
				t.Fatalf("quota UA %v", got)
			}
		}
		return json.Marshal(map[string]any{"StatusCode": 200, "Body": []byte(body)})
	}
	a := quotaAccount{Provider: "antigravity", Metadata: map[string]any{"project_id": "test-project"}, Storage: []byte(`{"access_token":"test-token"}`)}
	for range 2 {
		q, err := fetchProviderQuota(a, providerQuotaHTTP)
		if err != nil || len(q.Groups) != 2 || q.Groups[1].Buckets[0].Remaining != .8760508 || len(q.Groups[1].Models) != 3 {
			t.Fatal("Argon quota was lost", q, err)
		}
	}
	if manifestCalls != 1 || quotaCalls != 4 {
		t.Fatal("manifest version must be cached", manifestCalls, quotaCalls)
	}
	// The prior cohort is valid but incomplete; never synthesize an Argon bucket.
	q, err := parseProviderQuota("antigravity", []byte(`{"groups":[{"displayName":"Gemini Models","buckets":[{"window":"5h","remainingFraction":1}]}]}`), time.Now())
	if err != nil || len(q.Groups) != 1 {
		t.Fatal(q, err)
	}
}

func TestAntigravityBucketDefaultsAndDisabled(t *testing.T) {
	raw := `{"groups":[{"displayName":"Gemini Models","buckets":[{"bucketId":"gemini-weekly","window":"weekly","resetTime":"2026-11-25T08:16:42Z"},{"bucketId":"gemini-5h","window":"5h","remainingFraction":1,"disabled":true},{"bucketId":"explicit-zero","window":"weekly","remainingFraction":0},{"bucketId":"invalid","window":"weekly","remainingFraction":null},{}]}]}`
	q, err := parseProviderQuota("antigravity", []byte(raw), time.Now())
	if err != nil || len(q.Groups[0].Buckets) != 3 {
		t.Fatal(q, err)
	}
	b := q.Groups[0].Buckets
	if b[0].ID != "gemini-weekly" || b[0].Remaining != 0 || !b[1].Disabled || b[2].Remaining != 0 {
		t.Fatal("protobuf zero or disabled state lost", b)
	}
}

func TestAntigravityArgonAdmissionIsolation(t *testing.T) {
	t.Chdir(t.TempDir())
	now, _ := time.Parse(time.RFC3339, "2026-11-18T10:54:12Z")
	q, err := parseProviderQuota("antigravity", []byte(argonQuotaFixture), now)
	if err != nil {
		t.Fatal(err)
	}
	q.At = now
	if err := bindAntigravityModelGroups(&q, []byte(argonCatalogFixture)); err != nil {
		t.Fatal(err)
	}
	p := defaultPolicy()
	c := candidate{ID: "argon-test", Provider: "antigravity"}
	for _, model := range []string{"gemini-4-argon-high", "gemini-4-argon-medium", "gemini-4-argon-low", "gemini-3.8-flash-high", "gemini-4-hydrogen-xhigh"} {
		argon := strings.Contains(model, "argon")
		if relevantGroup(c.Provider, model, "Gemini 4 Argon", q.Groups) != argon || relevantGroup(c.Provider, model, "Gemini Models", q.Groups) == argon {
			t.Fatal("pool matching", model)
		}
		if _, ok := rankAccount(p, c, model, q, now, 0); !ok {
			t.Fatal("healthy model rejected", model)
		}
	}
	q.Groups[1].Buckets[0].Remaining = 0
	if _, ok := rankAccount(p, c, "gemini-4-argon-high", q, now, 0); ok {
		t.Fatal("exhausted Argon admitted")
	}
	if _, ok := rankAccount(p, c, "gemini-3.8-flash-high", q, now, 0); !ok {
		t.Fatal("Argon exhaustion blocked Flash")
	}
	q.Groups[1].Buckets[0].Disabled = true
	if _, ok := rankAccount(p, c, "gemini-4-argon-high", q, now, 0); !ok {
		t.Fatal("disabled quota bucket used as an exhausted limit")
	}
	if err := bindAntigravityModelGroups(&q, []byte(`{"models":{"gemini-4-argon-high":{"quotaInfo":{"remainingFraction":0.9999736,"resetTime":"2026-11-18T13:16:42Z"}}}}`)); err != nil {
		t.Fatal(err)
	}
	if !relevantGroup(c.Provider, "gemini-4-argon-high", "Gemini Models", q.Groups) {
		t.Fatal("catalog shared Gemini pool lost")
	}
}

func TestAntigravityDelayedAccountingDoesNotInferCapacity(t *testing.T) {
	now := time.Now()
	before, _ := parseProviderQuota("antigravity", []byte(argonQuotaFixture), now)
	before.At = now
	next := before
	next.Groups = []group{{Name: "Gemini 4 Argon", Buckets: []bucket{{Window: "weekly", Remaining: .8761841, Reset: before.Groups[1].Buckets[0].Reset}}}}
	if rates := estimateRates(before, next, now.Add(time.Minute)); len(rates) != 0 {
		t.Fatal("allowance recovery became consumption", rates)
	}
	if math.Abs(before.Groups[1].Buckets[0].Remaining*100-87.60508) > 1e-10 {
		t.Fatal("fraction normalized twice")
	}
}

func TestAntigravityStableQuotaIdentity(t *testing.T) {
	now := time.Date(2026, 11, 18, 11, 0, 0, 0, time.UTC)
	reset := now.Add(time.Hour)
	for _, tc := range []struct {
		name  string
		quota string
		other *bucket
		want  int
	}{
		{"fraction drift", `"remainingFraction":0.876176`, nil, 0},
		{"omitted protobuf zero", ``, nil, 0},
		{"explicit zero", `"remainingFraction":0`, nil, 0},
		{"window compatible", `"window":"5h"`, nil, 0},
		{"window incompatible", `"window":"weekly"`, nil, -1},
		{"duplicate reset different fractions", `"remainingFraction":0.8760508`, &bucket{ID: "other", Window: "five_hour", Remaining: .2, Reset: reset}, -1},
		{"duplicate reset different windows", `"window":"5h"`, &bucket{ID: "other", Window: "weekly", Remaining: .2, Reset: reset}, -1},
		{"duplicate reset disabled competitor", ``, &bucket{ID: "other", Window: "five_hour", Disabled: true, Reset: reset}, -1},
		{"explicit ID resolves collision", `"bucketId":"stable"`, &bucket{ID: "other", Window: "five_hour", Reset: reset}, 0},
		{"duplicate ID ambiguous", `"bucketId":"stable"`, &bucket{ID: "stable", Window: "five_hour", Reset: reset}, -1},
		{"unknown explicit ID", `"bucketId":"unknown"`, nil, -1},
		{"explicit ID incompatible window", `"bucketId":"stable","window":"weekly"`, nil, -1},
		{"disabled model quota", `"disabled":true`, nil, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := snapshot{At: now, Groups: []group{{Name: "first", Models: []string{"old-model"}, Buckets: []bucket{
				{ID: "stable", Window: "five_hour", Remaining: .8760508, Reset: reset},
				{ID: "weekly", Window: "weekly", Remaining: .9, Reset: now.Add(7 * 24 * time.Hour)},
			}}}}
			if tc.other != nil {
				q.Groups = append(q.Groups, group{Name: "second", Buckets: []bucket{*tc.other}})
			}
			fields := tc.quota
			if fields != "" {
				fields += ","
			}
			raw := fmt.Sprintf(`{"models":{"new-model":{"quotaInfo":{%s"resetTime":%q}}}}`, fields, reset.Format(time.RFC3339Nano))
			if err := bindAntigravityModelGroups(&q, []byte(raw)); err != nil {
				t.Fatal(err)
			}
			for i, g := range q.Groups {
				if (len(g.Models) == 1 && g.Models[0] == "new-model") != (i == tc.want) || len(g.Models) > 1 {
					t.Fatalf("unexpected assignment: %+v", q.Groups)
				}
			}
			if q.Groups[0].Buckets[0].Remaining != .8760508 {
				t.Fatal("catalog fraction overwrote summary capacity")
			}
		})
	}
	for _, tc := range []struct {
		name     string
		reset    time.Time
		disabled bool
	}{
		{"expired", now.Add(-time.Second), false},
		{"reset boundary", now, false},
		{"missing reset", time.Time{}, false},
		{"reset drift is not identity", reset.Add(time.Nanosecond), false},
		{"disabled bucket", reset, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := snapshot{At: now, Groups: []group{{Name: "pool", Buckets: []bucket{{ID: "stable", Window: "weekly", Reset: tc.reset, Disabled: tc.disabled}}}}}
			raw := fmt.Sprintf(`{"models":{"new-model":{"quotaInfo":{"bucketId":"stable","resetTime":%q}},"absent":{},"empty":{"quotaInfo":{}},"disabled":{"disabled":true,"quotaInfo":{"resetTime":%q}}}}`, reset.Format(time.RFC3339Nano), reset.Format(time.RFC3339Nano))
			if err := bindAntigravityModelGroups(&q, []byte(raw)); err != nil || len(q.Groups[0].Models) != 0 {
				t.Fatal(q, err)
			}
		})
	}
}

func TestAntigravitySameGroupResetWindows(t *testing.T) {
	now := time.Now()
	reset := now.Add(time.Hour)
	q := snapshot{At: now, Groups: []group{{Name: "pool", Buckets: []bucket{
		{Window: "weekly", Reset: reset}, {Window: "five_hour", Reset: reset},
	}}}}
	raw := fmt.Sprintf(`{"models":{"model":{"quotaInfo":{"window":"5h","resetTime":%q}}}}`, reset.Format(time.RFC3339Nano))
	if err := bindAntigravityModelGroups(&q, []byte(raw)); err != nil || len(q.Groups[0].Models) != 1 {
		t.Fatal(q, err)
	}
}

func TestAntigravityCatalogGroupsAllFamiliesWithoutPrefixGuessing(t *testing.T) {
	now := time.Now()
	q := snapshot{Groups: []group{
		{Name: "Future Gemini Pool", Buckets: []bucket{{Window: "weekly", Remaining: .7, Reset: now.Add(7 * time.Hour)}}},
		{Name: "Third-party Pool", Buckets: []bucket{{Window: "weekly", Remaining: .4, Reset: now.Add(8 * time.Hour)}}},
	}}
	models := map[string]any{}
	for _, id := range []string{"gemini-4-carbon-high", "gemini-4-hydrogen-xhigh", "gemini-3.8-flash-high"} {
		models[id] = map[string]any{"quotaInfo": map[string]any{"remainingFraction": .7, "resetTime": q.Groups[0].Buckets[0].Reset}}
	}
	for _, id := range []string{"claude-opus-5-5-high", "claude-sonnet-5-5-high", "gpt-oss-120b-medium"} {
		models[id] = map[string]any{"quotaInfo": map[string]any{"remainingFraction": .4, "resetTime": q.Groups[1].Buckets[0].Reset}}
	}
	raw, _ := json.Marshal(map[string]any{"models": models})
	if err := bindAntigravityModelGroups(&q, raw); err != nil {
		t.Fatal(err)
	}
	for model := range models {
		gemini := strings.HasPrefix(model, "gemini")
		if relevantGroup("antigravity", model, q.Groups[0].Name, q.Groups) != gemini || relevantGroup("antigravity", model, q.Groups[1].Name, q.Groups) == gemini {
			t.Fatal("wrong catalog pool", model)
		}
	}
	if relevantGroup("antigravity", "gemini-unmapped", q.Groups[0].Name, q.Groups) {
		t.Fatal("unmapped model inherited a prefix pool")
	}
	// Equal readings across different groups are not evidence of common ownership.
	q.Groups[1].Buckets = q.Groups[0].Buckets
	if err := bindAntigravityModelGroups(&q, raw); err != nil {
		t.Fatal(err)
	}
	if len(q.Groups[0].Models) != 0 {
		t.Fatal("ambiguous bucket match assigned models")
	}
	copyRaw, _ := json.Marshal(q)
	var restored snapshot
	json.Unmarshal(copyRaw, &restored)
	if restored.Groups[0].Models == nil {
		t.Fatal("empty authoritative mapping became legacy prefix inference after reload")
	}
}

func TestAntigravityManifestFailureBackoffAndLastGood(t *testing.T) {
	oldHost := hostCall
	antigravityAgent.Lock()
	oldValue, oldNext := antigravityAgent.value, antigravityAgent.next
	antigravityAgent.value, antigravityAgent.next = "", time.Time{}
	antigravityAgent.Unlock()
	t.Cleanup(func() {
		hostCall = oldHost
		antigravityAgent.Lock()
		antigravityAgent.value, antigravityAgent.next = oldValue, oldNext
		antigravityAgent.Unlock()
	})
	calls := 0
	hostCall = func(string, any) (json.RawMessage, error) {
		calls++
		return json.Marshal(map[string]any{"StatusCode": 200, "Body": []byte("version: not-a-version\n")})
	}
	for range 2 {
		if _, err := antigravityQuotaAgent(); err == nil {
			t.Fatal("invalid version accepted")
		}
	}
	if calls != 1 {
		t.Fatal("manifest failure not backed off", calls)
	}
	antigravityAgent.Lock()
	antigravityAgent.value, antigravityAgent.next = "last-good-agent", time.Time{}
	antigravityAgent.Unlock()
	if value, err := antigravityQuotaAgent(); err != nil || value != "last-good-agent" {
		t.Fatal("last successful version lost", value, err)
	}
}
