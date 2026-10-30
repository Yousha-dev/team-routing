package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type target struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}
type policy struct {
	Owned           []string          `json:"owned_emails"`
	Routes          map[string]target `json:"routes"`
	ClientLabels    map[string]string `json:"client_labels,omitempty"`
	Cutoff          float64           `json:"cutoff_used"`
	ExpiringUsed    float64           `json:"expiring_used_below"`
	ExpiringSeconds int               `json:"expiring_within_seconds"`
	Poll            int               `json:"poll_seconds"`
	MaxAge          int               `json:"max_quota_age_seconds"`
}
type bucket struct {
	Window      string
	Description string    `json:"description,omitempty"`
	Remaining   float64   `json:"remainingFraction"`
	Reset       time.Time `json:"resetTime"`
}
type group struct {
	Name    string `json:"displayName"`
	Buckets []bucket
}
type snapshot struct {
	At     time.Time
	Groups []group
	Rates  map[string]float64 `json:"rates,omitempty"`
}
type candidate struct {
	ID, Provider string
	Metadata     map[string]any
	Attributes   map[string]string
}
type binding struct {
	ID      string
	At      time.Time
	BoundAt time.Time
}

var state = struct {
	sync.Mutex
	Quota   map[string]snapshot
	Pins    map[string]binding
	Running bool
	Stop    chan struct{}
}{Quota: map[string]snapshot{}, Pins: map[string]binding{}, Stop: make(chan struct{})}

// pluginVersion is the release version reported to the host.
const pluginVersion = "1.0.2"

func main() {}
func readPolicy() (policy, error) {
	p := policy{}
	raw, e := os.ReadFile(statePath("team-routing.json"))
	if e != nil {
		return p, e
	}
	raw = bytes.TrimPrefix(raw, []byte{239, 187, 191})
	e = json.Unmarshal(raw, &p)
	if e != nil {
		return p, e
	}
	if p.Cutoff <= 0 || p.Cutoff > 1 || p.ExpiringUsed < 0 || p.ExpiringUsed > p.Cutoff || p.MaxAge < 30 || p.Poll < 15 {
		return p, fmt.Errorf("invalid routing policy")
	}
	return p, nil
}
func digest(s string) string { x := sha256.Sum256([]byte(s)); return hex.EncodeToString(x[:]) }
func key(h http.Header) string {
	v := h.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return h.Get("X-Api-Key")
}
func session(h http.Header, meta map[string]any) string {
	for _, n := range []string{"X-Claude-Code-Session-Id", "Session-Id", "X-Session-Id", "X-Openai-Session-Id", "X-Codex-Thread-Id"} {
		if v := h.Get(n); v != "" {
			return v
		}
	}
	for _, n := range []string{"session_id", "prompt_cache_key", "conversation_id"} {
		if v, ok := meta[n].(string); ok && v != "" {
			return v
		}
	}
	return "key-fallback"
}
func stopWorker() {
	state.Lock()
	defer state.Unlock()
	select {
	case <-state.Stop:
	default:
		close(state.Stop)
	}
	state.Running = false
}
func startWorker() {
	state.Lock()
	defer state.Unlock()
	if state.Running {
		return
	}
	select {
	case <-state.Stop:
		state.Stop = make(chan struct{})
	default:
	}
	state.Running = true
	go poll(state.Stop)
}
func dispatch(method string, raw []byte) (any, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		var lifecycle struct {
			ConfigYAML []byte `json:"config_yaml"`
		}
		_ = json.Unmarshal(raw, &lifecycle)
		applyConfig(lifecycle.ConfigYAML)
		if err := ensurePolicyFile(); err != nil {
			return nil, err
		}
		if _, err := readPolicy(); err != nil {
			return nil, err
		}
		startWorker()
		return map[string]any{"schema_version": 7, "metadata": map[string]any{"Name": "Team Routing", "Version": pluginVersion, "Author": "Yousha-dev", "GitHubRepository": "https://github.com/Yousha-dev/team-routing", "ConfigFields": []map[string]string{
			{"Name": "state_dir", "Type": "string", "Description": "Folder for team-routing.json, the quota cache and backups (default: the proxy's working directory)."},
			{"Name": "management_url", "Type": "string", "Description": "Optional management API base URL override. The browser dashboard sends its own origin, including port and HTTPS."},
		}}, "capabilities": map[string]bool{"scheduler": true, "scheduler_across_priorities": true, "model_router": true, "management_api": true, "quota_provider": true, "request_interceptor": true, "request_interceptor_omit_body_before": true, "request_interceptor_omit_body_after": true, "request_lifecycle_plugin": true}}, nil
	case "request.intercept_before", "request.intercept_after", "request.complete":
		return trackRequest(method, raw)
	case "quota.identifier":
		return map[string]string{"identifier": "team-routing"}, nil
	case "quota.describe":
		return map[string]any{"supported_providers": []string{"claude", "codex", "antigravity"}, "display_name": "Team Routing Quota", "supports_reset": false}, nil
	case "quota.fetch":
		return quotaRPC(raw)
	case "management.register":
		return map[string]any{"routes": []map[string]string{{"Method": "GET", "Path": dashboardAPI + "/state"}, {"Method": "PUT", "Path": dashboardAPI + "/policy"}, {"Method": "POST", "Path": dashboardAPI + "/refresh"}}, "resources": []map[string]string{{"Path": "/dashboard", "Menu": "Team Routing", "Description": "Live client model overrides, account preferences and quota routing."}}}, nil
	case "management.handle":
		var req dashboardRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return handleDashboard(req), nil
	case "model.route":
		startWorker()
		var req struct {
			Headers            http.Header
			AvailableProviders []string
		}
		if e := json.Unmarshal(raw, &req); e != nil {
			return nil, e
		}
		p, e := readPolicy()
		if e != nil {
			return nil, e
		}
		t, ok := p.Routes[digest(key(req.Headers))]
		if !ok {
			return map[string]bool{"Handled": false}, nil
		}
		found := false
		for _, provider := range req.AvailableProviders {
			if provider == t.Provider {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("configured target provider unavailable")
		}
		return map[string]any{"Handled": true, "TargetKind": "provider", "Target": t.Provider, "TargetModel": t.Model, "Reason": "client key model override"}, nil
	case "scheduler.pick":
		startWorker()
		var req struct {
			Provider, Model string
			Options         struct {
				Headers  http.Header
				Metadata map[string]any
			}
			Candidates []candidate
		}
		if e := json.Unmarshal(raw, &req); e != nil {
			return nil, e
		}
		p, e := readPolicy()
		if e != nil {
			return nil, e
		}
		state.Lock()
		defer state.Unlock()
		return choose(p, req.Provider, req.Model, req.Options.Headers, req.Options.Metadata, req.Candidates, time.Now()), nil
	default:
		return nil, fmt.Errorf("unsupported method")
	}
}

// Preference is specific to Claude subscriptions. Weekly pressure removes the
// preference, not eligibility, so ordinary quota and affinity rules still apply.
func preferredClaude(p policy, c candidate, q snapshot, now time.Time) bool {
	if c.Provider != "claude" || q.At.IsZero() || now.Sub(q.At) >= time.Duration(p.MaxAge)*time.Second {
		return false
	}
	email, _ := c.Metadata["email"].(string)
	owned := false
	for _, v := range p.Owned {
		if strings.EqualFold(email, v) || strings.Contains(strings.ToLower(c.ID), strings.ToLower(v)) {
			owned = true
		}
	}
	if !owned {
		return false
	}
	for _, g := range q.Groups {
		for _, b := range g.Buckets {
			if (b.Window == "weekly" || b.Window == "seven_day" || strings.HasPrefix(b.Window, "seven_day_")) && b.Remaining <= .30+1e-9 {
				return false
			}
		}
	}
	return true
}

func choose(p policy, provider, model string, h http.Header, meta map[string]any, candidates []candidate, now time.Time) map[string]any {
	return adaptiveChoose(p, provider, model, h, meta, candidates, now)
}
func poll(stop chan struct{}) {
	for {
		found := refresh()
		p, e := readPolicy()
		delay := time.Minute
		if e == nil {
			delay = time.Duration(p.Poll) * time.Second
		}
		// The first refresh can run while the host is still loading accounts;
		// retry soon instead of leaving every account without a reading.
		if found == 0 && delay > 15*time.Second {
			delay = 15 * time.Second
		}
		select {
		case <-stop:
			return
		case <-time.After(delay):
		}
	}
}
func refresh() int { return refreshQuotaMonitor() }
