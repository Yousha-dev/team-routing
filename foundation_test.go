package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func writePolicyFixture(t *testing.T, p policy) {
	t.Helper()
	raw, _ := json.Marshal(p)
	if err := os.WriteFile("team-routing.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func setObjectPath(m map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	node := m
	for _, part := range parts[:len(parts)-1] {
		node = node[part].(map[string]any)
	}
	node[parts[len(parts)-1]] = value
}
func policyFromObject(t *testing.T, m map[string]any) policy {
	t.Helper()
	raw, _ := json.Marshal(m)
	var p policy
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPolicySchemaMatchesRuntimeBounds(t *testing.T) {
	p := scheduledPolicy()
	if err := validatePolicy(p); err != nil {
		t.Fatal(err)
	}
	schema := policySchema()
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatal("missing standard schema")
	}
	for _, field := range policyFields() {
		if field.Minimum == nil {
			continue
		}
		for _, bad := range []float64{*field.Minimum - 1, *field.Maximum + 1} {
			m := jsonObject(p)
			setObjectPath(m, field.Path, bad)
			if validatePolicy(policyFromObject(t, m)) == nil {
				t.Fatalf("%s accepted out-of-schema value %v", field.Path, bad)
			}
		}
	}
}

func TestEveryEditableFieldPersistsThroughAPI(t *testing.T) {
	original := fetchActiveClientKeys
	fetchActiveClientKeys = func(...managementCaller) ([]string, error) { return []string{"synthetic-client"}, nil }
	t.Cleanup(func() { fetchActiveClientKeys = original })
	values := map[string]any{"owned_emails": []string{"other@example.com"}, "shared_claude.enabled": false, "shared_claude.weekly_remaining_floor": .18, "shared_claude.five_hour_remaining_floor": .25, "shared_claude.weekly_release_mode": "hours", "shared_claude.weekly_release_within_hours": 12, "shared_claude.protected_weekdays": []int{5, 6}, "shared_claude.protected_start_hour": 10, "shared_claude.protected_end_hour": 21, "shared_claude.timezone": "UTC", "poll_seconds": 120, "max_quota_age_seconds": 900, "weekly_five_hour_ratio": 6.0, "claude_stateless_threads": false, "require_extra_usage_disabled": false, "record_routing_events": true}
	for _, field := range policyFields() {
		t.Run(field.Path, func(t *testing.T) {
			t.Chdir(t.TempDir())
			applyConfig(nil)
			t.Cleanup(func() { stopWorker(); applyConfig(nil) })
			p := scheduledPolicy()
			p.StatelessThreads = true
			p.RecordRouting = false
			p.RequireCreditsOff = true
			writePolicyFixture(t, p)
			public := publicPolicy(p, localSecrets{Clients: []struct{ Name, Key string }{{"test", "synthetic-client"}}})
			data := jsonObject(public)
			setObjectPath(data, field.Path, values[field.Path])
			raw, _ := json.Marshal(data)
			response := handleDashboard(dashboardRequest{Method: "PUT", Path: dashboardAPI + "/policy", Body: raw})
			if response.StatusCode != 200 {
				t.Fatalf("field save failed: %d %s", response.StatusCode, response.Body)
			}
			got, err := readPolicy()
			if err != nil {
				t.Fatal(err)
			}
			actual, _ := atPath(jsonObject(got), field.Path)
			wantRaw, _ := json.Marshal(values[field.Path])
			actualRaw, _ := json.Marshal(actual)
			if string(actualRaw) != string(wantRaw) {
				t.Fatalf("ignored field: got %s want %s", actualRaw, wantRaw)
			}
		})
	}
}

func TestCatalogUsesActualProvidersAndValidatesPairs(t *testing.T) {
	old := fetchHostModelInventory
	fetchHostModelInventory = func() ([]hostCredentialModels, error) {
		return []hostCredentialModels{{ID: "a", Provider: "new-provider", Models: []string{"model-a", "shared"}}, {ID: "b", Provider: "codex", Models: []string{"model-b", "shared"}}}, nil
	}
	t.Cleanup(func() { fetchHostModelInventory = old })
	c := loadRoutingCatalog()
	if len(c.Models) != 3 || len(c.Providers) != 2 || !c.supports(target{Provider: "new-provider", Model: "model-a"}) {
		t.Fatal("dynamic registry incomplete", c)
	}
	if validateRouteTarget(c, target{Provider: "codex", Model: "model-a"}) == nil {
		t.Fatal("unsupported provider/model pair accepted")
	}
}

func TestDynamicProviderOverrideSaveAndKeyIsolation(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { stopWorker(); applyConfig(nil) })
	original := fetchActiveClientKeys
	fetchActiveClientKeys = func(...managementCaller) ([]string, error) { return []string{"key-a", "key-b"}, nil }
	t.Cleanup(func() { fetchActiveClientKeys = original })
	old := fetchHostModelInventory
	fetchHostModelInventory = func() ([]hostCredentialModels, error) {
		return []hostCredentialModels{{ID: "custom", Provider: "new-provider", Models: []string{"new-model"}}}, nil
	}
	t.Cleanup(func() { fetchHostModelInventory = old })
	p := defaultPolicy()
	writePolicyFixture(t, p)
	public := publicPolicy(p, localSecrets{Clients: []struct{ Name, Key string }{{"a", "key-a"}, {"b", "key-b"}}})
	public.Routes = map[string]target{digest("key-a"): {Provider: "new-provider", Model: "new-model"}}
	if _, err := saveDashboard(public); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		key     string
		handled bool
	}{{"key-a", true}, {"key-b", false}} {
		raw, _ := json.Marshal(map[string]any{"Headers": map[string][]string{"Authorization": {"Bearer " + tc.key}}, "AvailableProviders": []string{"claude", "new-provider"}})
		got, err := dispatch("model.route", raw)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(got)
		var result struct {
			Handled             bool
			Target, TargetModel string
		}
		_ = json.Unmarshal(encoded, &result)
		if result.Handled != tc.handled || tc.handled && (result.Target != "new-provider" || result.TargetModel != "new-model") {
			t.Fatal("route escaped key scope", result)
		}
	}
	stopWorker()
}

func TestModelScopedLimitsDoNotBlockOtherFamilies(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { stopWorker(); applyConfig(nil) })
	p := scheduledPolicy()
	now := localTime("2026-11-02 14:00")
	c := candidate{ID: "scoped", Provider: "claude"}
	q := sample(now, .8, .8, 4*time.Hour)
	q.Groups[0].Buckets = append(q.Groups[0].Buckets, bucket{Window: "seven_day_sonnet", Remaining: 0, Reset: now.Add(48 * time.Hour)}, bucket{Window: "seven_day_cowork", Remaining: 0, Reset: now.Add(48 * time.Hour)})
	if _, ok := rankAccount(p, c, "claude-opus-5-5", q, now, 0); !ok {
		t.Fatal("Sonnet or Cowork limit blocked Opus")
	}
	if _, ok := rankAccount(p, c, "claude-sonnet-5-5", q, now, 0); ok {
		t.Fatal("Sonnet exhaustion ignored")
	}
	if !strings.HasPrefix(accountEligibility(p, c, q, now, 0), "Eligible") {
		t.Fatal("overview treated a model-specific limit as global")
	}
	q.Groups[0].Buckets = append(q.Groups[0].Buckets, bucket{Window: "seven_day_oauth_apps", Remaining: 0})
	if _, ok := rankAccount(p, c, "claude-opus-5-5", q, now, 0); ok {
		t.Fatal("global OAuth-app exhaustion ignored")
	}
}

func TestPolicyBoundaryConsistencyAndMidnightEnd(t *testing.T) {
	p := scheduledPolicy()
	s := p.SharedClaude
	if !protectedFiveHourWindow(s, localTime("2026-11-02 20:00").Add(59*time.Second+5*time.Hour), localTime("2026-11-02 20:00").Add(59*time.Second)) {
		t.Fatal("end minute lost session protection")
	}
	if protectedFiveHourWindow(s, localTime("2026-11-02 20:01").Add(5*time.Hour), localTime("2026-11-02 20:01")) {
		t.Fatal("night start incorrectly protected")
	}
	s.EndHour = 24
	for _, tc := range []struct{ reset, release string }{{"2026-11-06 00:00", "2026-11-05 19:00"}, {"2026-11-06 00:01", "2026-11-06 00:00"}, {"2026-11-07 12:00", "2026-11-07 00:00"}} {
		if got := weeklyReserveReleaseAt(s, localTime(tc.reset)); !got.Equal(localTime(tc.release)) {
			t.Fatalf("midnight end: %s -> %s, want %s", tc.reset, got, tc.release)
		}
	}
	if !protectedFiveHourWindow(s, localTime("2026-11-06 00:00").Add(30*time.Second+5*time.Hour), localTime("2026-11-06 00:00").Add(30*time.Second)) {
		t.Fatal("prior protected day end spill was lost")
	}
}

func TestInvalidOptionsAreRejectedWithoutChangingPolicy(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { stopWorker(); applyConfig(nil) })
	p := scheduledPolicy()
	writePolicyFixture(t, p)
	before, _ := os.ReadFile("team-routing.json")
	for _, raw := range []string{`{"unexpected_option":true}`, `{"poll_seconds":15.5}`, `{"shared_claude":{"unrecognized":1}}`} {
		response := handleDashboard(dashboardRequest{Method: "PUT", Path: dashboardAPI + "/policy", Body: []byte(raw)})
		if response.StatusCode != 400 {
			t.Fatal("invalid or ignored option accepted", raw, response.StatusCode)
		}
	}
	after, _ := os.ReadFile("team-routing.json")
	if string(before) != string(after) {
		t.Fatal("invalid save changed policy")
	}
	for _, value := range []string{"invalid", "Name <other@example.com>"} {
		p.Owned = []string{value}
		if validatePolicy(p) == nil {
			t.Fatal(fmt.Sprint("invalid ownership accepted: ", value))
		}
	}
}

func TestSemanticPolicyValidation(t *testing.T) {
	for _, tc := range []struct {
		path  string
		value any
	}{
		{"shared_claude.weekly_release_mode", "invalid"},
		{"shared_claude.timezone", "Invalid/Zone"},
		{"shared_claude.protected_weekdays", []int{}},
		{"shared_claude.protected_weekdays", []int{0, 0}},
		{"shared_claude.protected_weekdays", []int{7}},
		{"shared_claude.protected_start_hour", 20},
		{"max_quota_age_seconds", 30},
	} {
		t.Run(tc.path+fmt.Sprint(tc.value), func(t *testing.T) {
			m := jsonObject(scheduledPolicy())
			setObjectPath(m, tc.path, tc.value)
			if validatePolicy(policyFromObject(t, m)) == nil {
				t.Fatal("invalid semantic value accepted")
			}
		})
	}
}

func TestNativeCooldownIsAnEligibilityGate(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	c.Metadata = map[string]any{"native_unavailable": true, "native_retry_after": now.Add(time.Minute)}
	if r, ok := rankAccount(p, c, "opus", q, now, 0); ok || !strings.Contains(r.Reason, "cooldown") {
		t.Fatal("native cooldown hidden", r)
	}
	c.Metadata["native_retry_after"] = now.Add(-time.Second)
	if _, ok := rankAccount(p, c, "opus", q, now, 0); !ok {
		t.Fatal("expired cooldown blocked fresh included allowance")
	}
}

func TestOwnershipUsesExactClaudeIdentity(t *testing.T) {
	p := scheduledPolicy()
	p.Owned = []string{"owner@example.com"}
	for _, c := range []candidate{
		{ID: "claude-owner@example.com.json", Provider: "claude", Metadata: map[string]any{"email": "other@example.com"}},
		{ID: "claude-other-owner@example.com.json", Provider: "claude"},
		{Provider: "codex", Metadata: map[string]any{"email": "owner@example.com"}},
	} {
		if ownedClaude(p, c) {
			t.Fatal("unrelated identity received owner exemption", c.ID)
		}
	}
	if !ownedClaude(p, candidate{Provider: "claude", Metadata: map[string]any{"email": "OWNER@example.com"}}) || !ownedClaude(p, candidate{Provider: "claude", Attributes: map[string]string{"email": "owner@example.com"}}) {
		t.Fatal("exact identity was not recognized")
	}
}
