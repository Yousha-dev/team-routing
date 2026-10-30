package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// settings holds the plugin's own configuration (plugins.configs.team-routing).
var settings = struct {
	sync.Mutex
	StateDir      string
	ManagementURL string
}{}

// applyConfig reads the flat keys this plugin understands from its config
// block. The block is simple key: value YAML, so no YAML dependency is needed.
func applyConfig(raw []byte) {
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || strings.HasPrefix(name, "#") {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		values[strings.TrimSpace(name)] = value
	}
	settings.Lock()
	settings.StateDir = values["state_dir"]
	settings.ManagementURL = strings.TrimRight(values["management_url"], "/")
	settings.Unlock()
}

// statePath resolves a state file inside state_dir (default: the proxy's
// working directory, where CLIProxyAPI keeps its own runtime files).
func statePath(name string) string {
	settings.Lock()
	dir := settings.StateDir
	settings.Unlock()
	if dir == "" || filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(dir, name)
}

// managementCaller is the identity of the dashboard request being served:
// the caller's own management key and the address the proxy answered on.
// Dashboard calls to the management API reuse it, so the plugin never needs
// to read the proxy's secrets from disk.
type managementCaller struct {
	Authorization string
	BaseURL       string
}

func callerFromRequest(h http.Header) managementCaller {
	settings.Lock()
	base := settings.ManagementURL
	settings.Unlock()
	if base == "" {
		base = strings.TrimRight(h.Get("X-Team-Routing-Origin"), "/")
	}
	if base == "" {
		base = strings.TrimRight(h.Get("Origin"), "/")
	}
	if base == "" {
		base = "http://127.0.0.1:8317"
	}
	return managementCaller{Authorization: h.Get("Authorization"), BaseURL: base}
}

// hostAuthFile is one entry of host.auth.list.
type hostAuthFile struct {
	ID       string `json:"id"`
	Index    string `json:"auth_index"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Provider string `json:"provider"`
	Disabled bool   `json:"disabled"`
}

func (f hostAuthFile) provider() string {
	if f.Provider != "" {
		return f.Provider
	}
	return f.Type
}

// hostCall is swapped out by tests.
var hostCall = callHost

func hostAuthList() ([]hostAuthFile, error) {
	raw, e := hostCall("host.auth.list", map[string]any{})
	if e != nil {
		return nil, e
	}
	var out struct {
		Files []hostAuthFile `json:"files"`
	}
	if e = json.Unmarshal(raw, &out); e != nil {
		return nil, e
	}
	return out.Files, nil
}

func hostAuthJSON(index string) ([]byte, error) {
	raw, e := hostCall("host.auth.get", map[string]string{"auth_index": index})
	if e != nil {
		return nil, e
	}
	var out struct {
		JSON json.RawMessage `json:"json"`
	}
	if e = json.Unmarshal(raw, &out); e != nil {
		return nil, e
	}
	return out.JSON, nil
}

// hostHTTP sends one request through the host, so the proxy's outbound
// proxy settings apply.
func hostHTTP(method, url string, headers map[string]string, body string) (quotaHTTP, error) {
	h := http.Header{}
	for name, value := range headers {
		h.Set(name, value)
	}
	raw, e := hostCall("host.http.do", map[string]any{"method": method, "url": url, "headers": h, "body": []byte(body)})
	if e != nil {
		return quotaHTTP{}, e
	}
	var out struct {
		StatusCode int
		Headers    http.Header
		Body       []byte
	}
	if e = json.Unmarshal(raw, &out); e != nil {
		return quotaHTTP{}, fmt.Errorf("decode host http response: %w", e)
	}
	return quotaHTTP{Code: out.StatusCode, Header: out.Headers, Body: string(out.Body)}, nil
}

// defaultPolicy is written on first start so the plugin works out of the box.
func defaultPolicy() policy {
	return policy{Owned: []string{}, Routes: map[string]target{}, ClientLabels: map[string]string{}, Cutoff: 0.85, ExpiringUsed: 0.5, ExpiringSeconds: 3600, Poll: 300, MaxAge: 1200}
}

func ensurePolicyFile() error {
	path := statePath("team-routing.json")
	if _, e := os.Stat(path); e == nil || !os.IsNotExist(e) {
		return e
	}
	if dir := filepath.Dir(path); dir != "." {
		if e := os.MkdirAll(dir, 0700); e != nil {
			return e
		}
	}
	raw, _ := json.MarshalIndent(defaultPolicy(), "", "  ")
	return os.WriteFile(path, raw, 0600)
}

// hostLog writes one line to the proxy's log.
func hostLog(level, message string) {
	_, _ = hostCall("host.log", map[string]any{"level": level, "message": message})
}
