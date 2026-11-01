package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDashboardSaveAndConflict(t *testing.T) {
	original := fetchActiveClientKeys
	fetchActiveClientKeys = func(_ ...managementCaller) ([]string, error) { return []string{"client-secret"}, nil }
	t.Cleanup(func() { fetchActiveClientKeys = original })
	t.Chdir(t.TempDir())
	p := policy{Owned: []string{"owner@example.com"}, Routes: map[string]target{digest("client-secret"): {Provider: "codex", Model: "gpt-6-luna"}}, Cutoff: .75, ExpiringUsed: .5, ExpiringSeconds: 3600, Poll: 60, MaxAge: 300}
	shared := defaultSharedPolicy()
	shared.WeeklyReleaseMode = "reset_schedule"
	shared.WeeklyReleaseHours = 5
	p.SharedClaude = &shared
	raw, _ := json.Marshal(p)
	os.WriteFile("team-routing.json", raw, 0600)
	os.WriteFile("secrets.json", []byte(`{"managementKey":"management-secret","clients":[{"name":"claude-code","key":"client-secret"}]}`), 0600)
	secrets, e := secretsForDashboard()
	if e != nil {
		t.Fatal(e)
	}
	public := publicPolicy(p, secrets)
	data, _ := json.Marshal(public)
	if strings.Contains(string(data), "client-secret") || strings.Contains(string(data), "management-secret") {
		t.Fatal("secret exposed")
	}
	if public.Routes[digest("client-secret")].Model != "gpt-6-luna" {
		t.Fatal("named route missing")
	}
	public.Routes = map[string]target{}
	public.ClientLabels = map[string]string{digest("client-secret"): "Claude Code workstation"}
	public.WeeklyRatio = 6
	saved, e := saveDashboard(public)
	if e != nil {
		t.Fatal(e)
	}
	got, e := readPolicy()
	if e != nil || got.WeeklyRatio != 6 || got.Cutoff != 0 || len(got.Routes) != 0 {
		t.Fatal("policy not persisted")
	}
	if got.ClientLabels[digest("client-secret")] != "Claude Code workstation" {
		t.Fatal("client label not persisted")
	}
	if got.SharedClaude == nil || got.SharedClaude.WeeklyReleaseMode != "reset_schedule" || got.SharedClaude.WeeklyReleaseHours != 5 {
		t.Fatal("scheduled release settings lost on dashboard save")
	}
	if saved.Revision == public.Revision {
		t.Fatal("revision unchanged")
	}
	if _, e = saveDashboard(public); e == nil || !strings.HasPrefix(e.Error(), "conflict:") {
		t.Fatal("stale write allowed")
	}
	saved.WeeklyRatio = .5
	if _, e = saveDashboard(saved); e == nil {
		t.Fatal("invalid weekly conversion allowed")
	}
}

func TestDynamicClientDiscovery(t *testing.T) {
	var local localSecrets
	json.Unmarshal([]byte(`{"managementKey":"admin-secret","clients":[{"name":"claude-code","key":"known-active-key"},{"name":"revoked-client","key":"revoked-key"}]}`), &local)
	p := policy{Routes: map[string]target{digest("new-active-key"): {Provider: "codex", Model: "gpt-6-luna"}}, ClientLabels: map[string]string{digest("new-active-key"): "New developer"}}
	inventory, details := discoverClients([]string{"known-active-key", "new-active-key", "new-active-key"}, local, p)
	if len(inventory.Clients) != 2 || len(details) != 2 {
		t.Fatal("active keys not deduplicated")
	}
	if details[0].Name != "claude-code" || details[1].Name != "New developer" {
		t.Fatal("labels incorrect")
	}
	if details[1].ID != digest("new-active-key") {
		t.Fatal("key identity not stable")
	}
	raw, _ := json.Marshal(details)
	if strings.Contains(string(raw), "known-active-key") || strings.Contains(string(raw), "new-active-key") || strings.Contains(string(raw), "revoked-client") {
		t.Fatal("raw or revoked key exposed")
	}
	public := publicPolicy(p, inventory)
	if public.Routes[digest("new-active-key")].Model != "gpt-6-luna" {
		t.Fatal("UI-created key cannot be routed")
	}
	_, remaining := discoverClients([]string{"new-active-key"}, local, p)
	if len(remaining) != 1 || remaining[0].ID != details[1].ID {
		t.Fatal("key removal changed identity")
	}
	if publicPolicy(p, inventory).Revision == publicPolicy(p, localSecrets{Clients: inventory.Clients[:1]}).Revision {
		t.Fatal("key-set change did not invalidate stale saves")
	}
}
func TestDashboardPublicShell(t *testing.T) {
	res := handleDashboard(dashboardRequest{Method: "GET", Path: "/v0/resource/plugins/team-routing/dashboard"})
	if res.StatusCode != 200 || !strings.Contains(string(res.Body), "Save changes") || !strings.Contains(res.Headers.Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatal("dashboard shell invalid")
	}
}
