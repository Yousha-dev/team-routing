package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestExactModelRulesPrecedeDefaultsWithoutChaining(t *testing.T) {
	p := defaultPolicy()
	id := digest("client")
	p.Routes[id] = target{"codex", "default"}
	p.ModelRoutes = map[string]map[string]modelRule{id: {"opus": {Action: "requested"}, "haiku": {Action: "override", Provider: "antigravity", Model: "gemini"}, "gemini": {Action: "override", Provider: "codex", Model: "another"}}}
	for _, tc := range []struct {
		source          string
		override        bool
		provider, model string
	}{{"opus", false, "", ""}, {"haiku", true, "antigravity", "gemini"}, {"other", true, "codex", "default"}} {
		got, handled, _ := resolveClientRoute(p, id, tc.source)
		if handled != tc.override || got.Provider != tc.provider || got.Model != tc.model {
			t.Fatal("incorrect exact/default resolution", tc, got)
		}
	}
	if _, handled, _ := resolveClientRoute(p, digest("other-client"), "haiku"); handled {
		t.Fatal("model rule escaped client key")
	}
	for _, rule := range []modelRule{{Action: "unknown"}, {Action: "requested", Provider: "codex", Model: "bad"}, {Action: "override", Provider: "codex"}} {
		if validateModelRules(map[string]map[string]modelRule{id: {"source": rule}}) == nil {
			t.Fatal("invalid rule accepted")
		}
	}
}

func TestModelPreflightUsesOriginalModelAndSameResolver(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	now := time.Now()
	p := defaultPolicy()
	id := digest("client")
	p.Routes[id] = target{"antigravity", "gemini"}
	p.ModelRoutes = map[string]map[string]modelRule{id: {"haiku": {Action: "override", Provider: "antigravity", Model: "gemini"}, "sonnet": {Action: "requested"}}}
	inventoryFixture(t, []hostAuthFile{{ID: "native", Provider: "claude"}, {ID: "google", Provider: "antigravity"}}, []hostCredentialModels{{ID: "native", Provider: "claude", Models: []string{"haiku", "sonnet"}}, {ID: "google", Provider: "antigravity", Models: []string{"gemini"}}})
	state.Quota = map[string]snapshot{"native": sample(now, 0, .8, time.Hour), "google": {At: now, Groups: []group{{Name: "Gemini models", Buckets: []bucket{{Window: "weekly", Remaining: .8, Reset: now.Add(24 * time.Hour)}}}}}}
	h := http.Header{"Authorization": {"Bearer client"}}
	if got := availabilityPreflight(p, availabilityRequest{Model: "gemini", RequestedModel: "haiku", Headers: h}, now); len(got) != 0 {
		t.Fatal("mapped model did not use target quota", got)
	}
	if got := availabilityPreflight(p, availabilityRequest{Model: "sonnet", Headers: h}, now); got["StatusCode"] != 429 {
		t.Fatal("pass-through exception incorrectly used default override", got)
	}
}

func TestOwnershipSupportsEveryProviderWithoutBroadeningLegacyEmails(t *testing.T) {
	p := defaultPolicy()
	p.Owned = []string{"owner@example.com"}
	claude := candidate{ID: "a", Provider: "claude", Metadata: map[string]any{"email": "owner@example.com"}}
	codex := candidate{ID: "b", Provider: "codex", Metadata: claude.Metadata}
	if !ownedAccount(p, claude) || ownedAccount(p, codex) {
		t.Fatal("legacy email scope changed")
	}
	p.AccountOwnership = map[string]bool{"a": false, "b": true}
	if ownedAccount(p, claude) || !ownedAccount(p, codex) || ownedClaude(p, codex) {
		t.Fatal("explicit provider-neutral ownership precedence failed")
	}
}

func TestProviderAndNativeWindowProtection(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	p := scheduledPolicy()
	base := *p.SharedClaude
	p.ProviderReserves = map[string]*sharedClaudePolicy{"codex": &base, "antigravity": &base}
	now := localTime("2026-11-02 14:00")
	c := candidate{ID: "code", Provider: "codex"}
	q := sample(now, .8, .14, time.Hour)
	if _, ok := rankAccount(p, c, "gpt", q, now, 0); ok {
		t.Fatal("Codex default weekly reserve ignored")
	}
	p.AccountOwnership = map[string]bool{c.ID: true}
	if _, ok := rankAccount(p, c, "gpt", q, now, 0); !ok {
		t.Fatal("explicit Codex owner did not inherit exemption")
	}
	p.AccountReserves = map[string]accountReserve{c.ID: {FiveHourFloor: .3, WeeklyFloor: .2}}
	if _, ok := rankAccount(p, c, "gpt", q, now, 0); ok {
		t.Fatal("explicit account reserve did not win over owner exemption")
	}
	google := candidate{ID: "google", Provider: "antigravity"}
	key := quotaWindowKey("Gemini models", "daily")
	p.WindowReserves = map[string]map[string]windowReserve{google.ID: {key: {Group: "Gemini models", Window: "daily", Floor: .25, ReleaseMinutes: 60}}}
	native := snapshot{At: now, Groups: []group{{Name: "Gemini models", Buckets: []bucket{{Window: "daily", Remaining: .2, Reset: now.Add(2 * time.Hour)}}}}}
	if _, ok := rankAccount(p, google, "gemini", native, now, 0); ok {
		t.Fatal("daily native reserve ignored")
	}
	native.Groups[0].Buckets[0].Reset = now.Add(30 * time.Minute)
	rank, ok := rankAccount(p, google, "gemini", native, now, 0)
	if !ok || !rank.FiveHourReset.IsZero() || rank.Ratio != 0 {
		t.Fatal("native daily quota invented a five-hour allowance", rank)
	}
	native.Groups = nil
	if _, ok := rankAccount(p, google, "gemini", native, now, 0); ok {
		t.Fatal("missing configured native window silently bypassed reserve")
	}
}

func TestFlexibleSettingsSaveAndOlderClientPreservation(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	oldKeys, oldHost, oldModels := fetchActiveClientKeys, hostCall, fetchHostModelInventory
	fetchActiveClientKeys = func(...managementCaller) ([]string, error) { return []string{"client"}, nil }
	hostCall = func(method string, _ any) (json.RawMessage, error) {
		if method == "host.auth.list" {
			return []byte(`{"files":[{"id":"code","provider":"codex"},{"id":"google","provider":"antigravity"}]}`), nil
		}
		return nil, fmt.Errorf("unexpected callback")
	}
	fetchHostModelInventory = func() ([]hostCredentialModels, error) {
		return []hostCredentialModels{{ID: "google", Provider: "antigravity", Models: []string{"gemini"}}}, nil
	}
	t.Cleanup(func() { fetchActiveClientKeys = oldKeys; hostCall = oldHost; fetchHostModelInventory = oldModels })
	p := scheduledPolicy()
	writePolicyFixture(t, p)
	clients := localSecrets{Clients: []struct{ Name, Key string }{{"test", "client"}}}
	input := publicPolicy(p, clients)
	base := *p.SharedClaude
	input.ModelRoutes = map[string]map[string]modelRule{digest("client"): {"haiku": {Action: "override", Provider: "antigravity", Model: "gemini"}, "opus": {Action: "requested"}}}
	input.AccountOwnership = map[string]bool{"code": true}
	input.ProviderReserves = map[string]*sharedClaudePolicy{"codex": &base}
	saved, err := saveDashboard(input)
	if err != nil {
		t.Fatal(err)
	}
	m := jsonObject(saved)
	for _, name := range []string{"model_routes", "provider_reserves", "account_ownership", "window_reserves"} {
		delete(m, name)
	}
	raw, _ := json.Marshal(m)
	var old dashboardPolicy
	if json.Unmarshal(raw, &old) != nil {
		t.Fatal("old request decoding failed")
	}
	if _, err := saveDashboard(old); err != nil {
		t.Fatal(err)
	}
	got, _ := readPolicy()
	if !got.AccountOwnership["code"] || got.ModelRoutes[digest("client")]["haiku"].Model != "gemini" || got.ProviderReserves["codex"] == nil {
		t.Fatal("older client erased flexible settings")
	}
}
