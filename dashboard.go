package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed dashboard.html
var dashboardHTML []byte

//go:embed dashboard.css
var dashboardCSS []byte

//go:embed dashboard.js
var dashboardJS []byte

const dashboardAPI = "/v0/management/plugins/team-routing"

type dashboardRequest struct {
	Method, Path string
	Headers      http.Header
	Body         []byte
}
type dashboardResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}
type localSecrets struct {
	Clients []struct{ Name, Key string } `json:"clients"`
}
type dashboardPolicy struct {
	Revision          string                    `json:"revision"`
	Owned             []string                  `json:"owned_emails"`
	Routes            map[string]target         `json:"routes"`
	ClientLabels      map[string]string         `json:"client_labels,omitempty"`
	AccountReserves   map[string]accountReserve `json:"account_reserves,omitempty"`
	Cutoff            float64                   `json:"cutoff_used,omitempty"`
	ExpiringUsed      float64                   `json:"expiring_used_below,omitempty"`
	ExpiringSeconds   int                       `json:"expiring_within_seconds,omitempty"`
	Poll              int                       `json:"poll_seconds"`
	MaxAge            int                       `json:"max_quota_age_seconds"`
	SharedClaude      *sharedClaudePolicy       `json:"shared_claude,omitempty"`
	StatelessThreads  bool                      `json:"claude_stateless_threads"`
	RecordRouting     bool                      `json:"record_routing_events"`
	WeeklyRatio       float64                   `json:"weekly_five_hour_ratio"`
	RequireCreditsOff bool                      `json:"require_extra_usage_disabled"`
	EditedFields      map[string]bool           `json:"-"`
}

func (p *dashboardPolicy) UnmarshalJSON(raw []byte) error {
	type plain dashboardPolicy
	var value plain
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	*p = dashboardPolicy(value)
	p.EditedFields = map[string]bool{}
	for field := range fields {
		p.EditedFields[field] = true
	}
	if raw, ok := fields["account_reserves"]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("account_reserves must be an object; use {} to remove all custom rules")
	}
	return nil
}

type dashboardAccount struct {
	ID, Name, Email, Provider string
	Disabled                  bool
	Snapshot                  snapshot
	Owned                     bool
	Cache                     map[string]any
	ActiveRequests            int
	Preferred                 bool
	Eligibility               string
	Deadline                  *adaptiveRank          `json:"Deadline,omitempty"`
	WeeklyReleases            []weeklyReserveRelease `json:"weekly_reserve_releases,omitempty"`
	Unavailable               bool                   `json:"unavailable"`
	NextRetry                 time.Time              `json:"next_retry_after"`
	Decision                  map[string]any         `json:"decision"`
	Models                    []string               `json:"models"`
	Reserves                  map[string]any         `json:"reserves,omitempty"`
}

type weeklyReserveRelease struct {
	Window  string    `json:"window"`
	Reset   time.Time `json:"reset"`
	Release time.Time `json:"release"`
	Locked  bool      `json:"locked"`
}

var policyWrite sync.Mutex
var refreshWrite sync.Mutex

// secretsForDashboard reads optional client labels from a secrets.json in
// state_dir (the layout of the scripted Windows setup). It is not required:
// without it, clients are discovered from the proxy and labelled in the
// dashboard.
func secretsForDashboard() (localSecrets, error) {
	var s localSecrets
	raw, e := os.ReadFile(statePath("secrets.json"))
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(bytes.TrimPrefix(raw, []byte{239, 187, 191}), &s)
	return s, e
}

// localJSON calls this proxy's API with the identity of the dashboard
// request currently being served.
func localJSON(method, path string, out any, callers ...managementCaller) error {
	caller := managementCaller{}
	if len(callers) > 0 {
		caller = callers[0]
	}
	u, e := url.Parse(caller.BaseURL)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid management URL")
	}
	if caller.Authorization == "" {
		return fmt.Errorf("no management request in progress")
	}
	req, e := http.NewRequest(method, caller.BaseURL+path, nil)
	if e != nil {
		return e
	}
	credential := strings.TrimSpace(strings.TrimPrefix(caller.Authorization, "Bearer "))
	if strings.HasPrefix(path, "/v1/") {
		if caller.ClientKey != "" {
			credential = caller.ClientKey
		} else {
			var active []string
			if err := localJSON("GET", "/v8/management/config/access/api-keys", &active, callers...); err != nil {
				return err
			}
			if len(active) == 0 {
				return fmt.Errorf("no active client key")
			}
			credential = active[0]
		}
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("local proxy HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
}
func publicPolicy(p policy, s localSecrets) dashboardPolicy {
	raw, _ := os.ReadFile(statePath("team-routing.json"))
	ids := []string{}
	for _, c := range s.Clients {
		ids = append(ids, digest(c.Key))
	}
	sort.Strings(ids)
	out := dashboardPolicy{Revision: digest(string(raw) + strings.Join(ids, "|")), Owned: p.Owned, Routes: map[string]target{}, ClientLabels: map[string]string{}, Cutoff: p.Cutoff, ExpiringUsed: p.ExpiringUsed, ExpiringSeconds: p.ExpiringSeconds, Poll: p.Poll, MaxAge: p.MaxAge}
	if p.SharedClaude != nil {
		copy := *p.SharedClaude
		out.SharedClaude = &copy
	}
	if out.SharedClaude == nil {
		s := defaultSharedPolicy()
		out.SharedClaude = &s
	}
	if out.SharedClaude.WeeklyReleaseMode == "" {
		out.SharedClaude.WeeklyReleaseMode = "hours"
	}
	out.StatelessThreads = p.StatelessThreads
	out.RecordRouting = p.RecordRouting
	out.WeeklyRatio = p.WeeklyRatio
	out.RequireCreditsOff = p.RequireCreditsOff
	out.AccountReserves = cloneAccountReserves(p.AccountReserves)
	if out.WeeklyRatio == 0 {
		out.WeeklyRatio = 5
	}
	for _, c := range s.Clients {
		id := digest(c.Key)
		if v, ok := p.Routes[id]; ok {
			out.Routes[id] = v
		}
		if label := p.ClientLabels[id]; label != "" {
			out.ClientLabels[id] = label
		}
	}
	return out
}

type dashboardClient struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Masked      string `json:"masked"`
	Fingerprint string `json:"fingerprint"`
	Source      string `json:"source"`
}

func discoverClients(active []string, local localSecrets, p policy) (localSecrets, []dashboardClient) {
	out := localSecrets{}
	details := []dashboardClient{}
	seen := map[string]bool{}
	for _, key := range active {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		id := digest(key)
		name := "Client " + id[:12]
		source := "Discovered from proxy"
		for _, known := range local.Clients {
			if known.Key == key {
				name = known.Name
				source = "Local setup label"
				break
			}
		}
		if label := p.ClientLabels[id]; label != "" {
			name = label
			source = "Dashboard label"
		}
		masked := "••••"
		if len(key) > 8 {
			masked = key[:4] + "••••" + key[len(key)-4:]
		}
		details = append(details, dashboardClient{id, name, masked, id[:12], source})
		out.Clients = append(out.Clients, struct{ Name, Key string }{name, key})
	}
	return out, details
}

var fetchActiveClientKeys = func(callers ...managementCaller) ([]string, error) {
	var active []string
	e := localJSON("GET", "/v8/management/config/access/api-keys", &active, callers...)
	return active, e
}

func activeClientInventory(p policy, callers ...managementCaller) (localSecrets, []dashboardClient, error) {
	s, e := secretsForDashboard()
	if e != nil {
		return s, nil, e
	}
	keys, e := fetchActiveClientKeys(callers...)
	if e != nil {
		return s, nil, e
	}
	s, details := discoverClients(keys, s, p)
	return s, details, nil
}
func dashboardState(caller managementCaller) (any, error) {
	p, e := readPolicy()
	if e != nil {
		return nil, e
	}
	s, details, e := activeClientInventory(p, caller)
	if e != nil {
		return nil, e
	}
	var files struct{ Files []dashboardAccount }
	inventory, inventoryErr := hostAuthList()
	if inventoryErr == nil {
		files.Files = []dashboardAccount{}
		for _, a := range inventory {
			files.Files = append(files.Files, dashboardAccount{ID: a.ID, Name: a.Name, Email: a.Email, Provider: a.provider(), Disabled: a.Disabled, Unavailable: a.Unavailable, NextRetry: a.NextRetry})
		}
	} else if e = localJSON("GET", "/v8/management/credentials", &files, caller); e != nil {
		return nil, e
	}
	if len(s.Clients) > 0 {
		caller.ClientKey = s.Clients[0].Key
	}
	catalog := loadRoutingCatalog(caller)
	registry, registryErr := fetchHostModelInventory()
	modelSets := map[string][]string{}
	if registryErr == nil {
		known := map[string]bool{}
		for _, account := range files.Files {
			known[account.ID] = true
		}
		for _, entry := range registry {
			modelSets[entry.ID] = entry.Models
			if !known[entry.ID] {
				files.Files = append(files.Files, dashboardAccount{ID: entry.ID, Name: entry.ID, Provider: entry.Provider})
				known[entry.ID] = true
			}
		}
	}
	clients := []string{}
	for _, c := range s.Clients {
		clients = append(clients, digest(c.Key))
	}
	models := []string{}
	for _, m := range catalog.Models {
		models = append(models, m.ID)
	}
	state.Lock()
	pins := len(state.Pins)
	active := activeCounts(time.Now())
	for i := range files.Files {
		a := &files.Files[i]
		a.Models = modelSets[a.ID]
		a.Snapshot = state.Quota[a.ID]
		a.ActiveRequests = active[a.ID]
		c := candidate{ID: a.ID, Provider: a.Provider, Metadata: map[string]any{"email": a.Email, "native_unavailable": a.Unavailable, "native_retry_after": a.NextRetry}}
		a.Owned = ownedClaude(p, c)
		a.Reserves = effectiveAccountReserve(p, c)
		a.Preferred = false // Owned is separate; no numerical ownership preference.
		a.Eligibility = accountEligibility(p, c, a.Snapshot, time.Now(), a.ActiveRequests)
		status := "eligible"
		if strings.HasPrefix(a.Eligibility, "Blocked:") {
			status = "blocked"
		}
		if a.Provider != "claude" && a.Provider != "codex" && status != "blocked" {
			status = "provider_managed"
		}
		a.Decision = map[string]any{"status": status, "detail": a.Eligibility}
		if settings := sharedSettings(p, c); settings != nil {
			for _, g := range a.Snapshot.Groups {
				for _, b := range g.Buckets {
					if weeklyWindow(b.Window) && !b.Reset.IsZero() {
						release := weeklyReserveReleaseAt(settings, b.Reset)
						a.WeeklyReleases = append(a.WeeklyReleases, weeklyReserveRelease{Window: b.Window, Reset: b.Reset, Release: release, Locked: !b.Reset.After(time.Now()) || time.Now().Before(release)})
					}
				}
			}
		}
		if a.Disabled {
			a.Eligibility = "Disabled in proxy"
			a.Decision = map[string]any{"status": "disabled", "detail": a.Eligibility}
			continue
		}
		if registryErr == nil && len(a.Models) == 0 {
			a.Eligibility = "No enabled models advertised by the proxy"
			a.Decision = map[string]any{"status": "blocked", "detail": a.Eligibility}
			continue
		}
		if r, ok := rankAccount(p, c, "", a.Snapshot, time.Now(), a.ActiveRequests); ok {
			a.Deadline = &r
		}
	}
	state.Unlock()
	for i := range files.Files {
		files.Files[i].Cache = cacheDetails(quotaAccount{ID: files.Files[i].ID, Provider: files.Files[i].Provider})
	}
	traffic.Lock()
	completed := traffic.Completed
	byAccount := map[string]map[string]uint64{}
	for id, counts := range traffic.Outcomes {
		byAccount[id] = map[string]uint64{}
		for outcome, count := range counts {
			byAccount[id][outcome] = count
		}
	}
	traffic.Unlock()
	return map[string]any{"version": pluginVersion, "catalog": catalog, "rules": evaluatedRules(p), "policy": publicPolicy(p, s), "clients": clients, "client_details": details, "models": models, "accounts": files.Files, "pins": pins, "monitor": monitorStats(), "cache_efficiency": cacheDiagnostics(), "routing": map[string]any{"engine": "deadline-v1", "completed_requests": completed, "active_requests": active, "outcomes_by_account": byAccount}, "now": time.Now().UTC(), "system_timezone": time.Now().Format("MST -07:00")}, nil
}
func saveDashboard(input dashboardPolicy, callers ...managementCaller) (dashboardPolicy, error) {
	policyWrite.Lock()
	defer policyWrite.Unlock()
	current, e := readPolicy()
	if e != nil {
		return dashboardPolicy{}, e
	}
	s, _, e := activeClientInventory(current, callers...)
	if e != nil {
		return dashboardPolicy{}, e
	}
	old := publicPolicy(current, s)
	if input.Revision == "" || input.Revision != old.Revision {
		return old, fmt.Errorf("conflict: settings changed elsewhere; reload before saving")
	}
	if input.WeeklyRatio == 0 {
		input.WeeklyRatio = old.WeeklyRatio
	}
	if input.SharedClaude == nil {
		input.SharedClaude = current.SharedClaude
	}
	if err := validateSharedPolicy(input.SharedClaude); err != nil {
		return old, err
	}
	if len(input.Owned) > 100 || len(input.Routes) > 200 {
		return old, fmt.Errorf("too many accounts or routes")
	}
	for _, email := range input.Owned {
		if len(email) > 254 || !strings.Contains(email, "@") {
			return old, fmt.Errorf("enter valid owned-account email addresses")
		}
	}
	var catalog routingCatalog
	if len(input.Routes) > 0 {
		catalog = loadRoutingCatalog(callers...)
	}
	routes := map[string]target{}
	for name, t := range input.Routes {
		if err := validateRouteTarget(catalog, t); err != nil && current.Routes[name] != t {
			return old, err
		}
		found := false
		for _, c := range s.Clients {
			if digest(c.Key) == name {
				routes[digest(c.Key)] = t
				found = true
			}
		}
		if !found {
			return old, fmt.Errorf("unknown client name")
		}
	}
	// Keep inactive-key preferences inert; the host still enforces key revocation.
	for hash, t := range current.Routes {
		named := false
		for _, c := range s.Clients {
			if digest(c.Key) == hash {
				named = true
			}
		}
		if !named {
			routes[hash] = t
		}
	}
	labels := map[string]string{}
	for id, label := range current.ClientLabels {
		labels[id] = label
	}
	for id, label := range input.ClientLabels {
		active := false
		for _, c := range s.Clients {
			if digest(c.Key) == id {
				active = true
			}
		}
		if !active {
			return old, fmt.Errorf("client list changed; reload settings")
		}
		label = strings.TrimSpace(label)
		if len(label) > 80 || strings.ContainsAny(label, "\r\n\t") {
			return old, fmt.Errorf("client labels must be at most 80 characters on one line")
		}
		if label == "" {
			delete(labels, id)
		} else {
			labels[id] = label
		}
	}
	next := policy{Owned: input.Owned, Routes: routes, ClientLabels: labels, Poll: input.Poll, MaxAge: input.MaxAge, WeeklyRatio: input.WeeklyRatio}
	next.SharedClaude = input.SharedClaude
	next.AccountReserves = input.AccountReserves
	if input.AccountReserves == nil && !input.EditedFields["account_reserves"] {
		next.AccountReserves = cloneAccountReserves(current.AccountReserves)
	}
	if err := validateNewAccountReserves(next.AccountReserves, current.AccountReserves); err != nil {
		return old, err
	}
	next.StatelessThreads = input.StatelessThreads
	next.RecordRouting = current.RecordRouting
	next.RequireCreditsOff = current.RequireCreditsOff
	if input.EditedFields["record_routing_events"] {
		next.RecordRouting = input.RecordRouting
	}
	if input.EditedFields["require_extra_usage_disabled"] {
		next.RequireCreditsOff = input.RequireCreditsOff
	}
	if err := validatePolicy(next); err != nil {
		return old, err
	}
	raw, e := json.MarshalIndent(next, "", "  ")
	if e != nil {
		return old, e
	}
	policyPath := statePath("team-routing.json")
	prior, e := os.ReadFile(policyPath)
	if e != nil {
		return old, e
	}
	backups := statePath("backups")
	if e = os.MkdirAll(backups, 0700); e != nil {
		return old, e
	}
	if e = os.WriteFile(filepath.Join(backups, fmt.Sprintf("team-routing-dashboard-%d.json", time.Now().UnixNano())), prior, 0600); e != nil {
		return old, e
	}
	temp, e := os.CreateTemp(filepath.Dir(policyPath), "team-routing-*.tmp")
	if e != nil {
		return old, e
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, e = temp.Write(raw); e != nil {
		temp.Close()
		return old, e
	}
	if e = temp.Close(); e != nil {
		return old, e
	}
	if e = os.Rename(name, policyPath); e != nil {
		return old, e
	}
	return publicPolicy(next, s), nil
}
func dashboardJSON(code int, value any) dashboardResponse {
	raw, _ := json.Marshal(value)
	return dashboardResponse{code, http.Header{"Content-Type": {"application/json"}, "Cache-Control": {"no-store"}, "X-Content-Type-Options": {"nosniff"}}, raw}
}
func handleDashboard(req dashboardRequest) dashboardResponse {
	if req.Method == "GET" && (strings.HasSuffix(req.Path, "/dashboard.css") || strings.HasSuffix(req.Path, "/dashboard.js")) {
		body, mime := dashboardCSS, "text/css; charset=utf-8"
		if strings.HasSuffix(req.Path, "/dashboard.js") {
			body, mime = dashboardJS, "text/javascript; charset=utf-8"
		}
		return dashboardResponse{200, http.Header{"Content-Type": {mime}, "Cache-Control": {"no-store"}, "X-Content-Type-Options": {"nosniff"}}, body}
	}
	if req.Method == "GET" && strings.HasSuffix(req.Path, "/dashboard") {
		return dashboardResponse{200, http.Header{"Content-Type": {"text/html; charset=utf-8"}, "Cache-Control": {"no-store"}, "Content-Security-Policy": {"default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'self'; base-uri 'none'"}, "Referrer-Policy": {"no-referrer"}, "X-Content-Type-Options": {"nosniff"}}, dashboardHTML}
	}
	startWorker()
	caller := callerFromRequest(req.Headers)
	switch req.Method + " " + req.Path {
	case "GET " + dashboardAPI + "/schema":
		return dashboardJSON(200, map[string]any{"version": pluginVersion, "schema": policySchema(), "fields": policyFields(), "groups": policyGroups(), "engine": engineProfile(), "account_reserves": map[string]any{"title": "Account reserves", "description": "Choose custom percentages per Claude credential, or inherit defaults. Owned accounts stay exempt unless explicitly customized. Existing protection and weekly release schedules apply. Removing and reconnecting a credential under a different ID requires a new rule.", "fields": accountReserveFields()}, "ui": map[string]any{"refresh_seconds": 15}})
	case "POST " + dashboardAPI + "/evaluate":
		if len(req.Body) > 4096 {
			return dashboardJSON(413, map[string]string{"error": "Evaluation request is too large."})
		}
		var input routeEvaluationInput
		if json.Unmarshal(req.Body, &input) != nil {
			return dashboardJSON(400, map[string]string{"error": "Invalid evaluation request."})
		}
		result, err := evaluateRoute(input, caller)
		if err != nil {
			return dashboardJSON(400, map[string]string{"error": err.Error()})
		}
		return dashboardJSON(200, result)
	case "GET " + dashboardAPI + "/state":
		result, e := dashboardState(caller)
		if e != nil {
			return dashboardJSON(503, map[string]string{"error": "Unable to load proxy state. Retry after the proxy finishes starting."})
		}
		return dashboardJSON(200, result)
	case "PUT " + dashboardAPI + "/policy":
		if len(req.Body) > 65536 {
			return dashboardJSON(413, map[string]string{"error": "Settings are too large."})
		}
		var input dashboardPolicy
		if json.Unmarshal(req.Body, &input) != nil {
			return dashboardJSON(400, map[string]string{"error": "Invalid settings JSON."})
		}
		result, e := saveDashboard(input, caller)
		if e != nil {
			code := 400
			if strings.HasPrefix(e.Error(), "conflict:") {
				code = 409
			}
			return dashboardJSON(code, map[string]string{"error": e.Error()})
		}
		return dashboardJSON(200, map[string]any{"policy": result, "message": "Saved. New requests use these settings immediately. Start a fresh conversation after changing model families."})
	case "POST " + dashboardAPI + "/refresh":
		if !refreshWrite.TryLock() {
			return dashboardJSON(202, map[string]string{"message": "A quota refresh is already running."})
		}
		go func() { defer refreshWrite.Unlock(); _ = refresh() }()
		return dashboardJSON(202, map[string]string{"message": "Checking due quota readings. Fresh cached data and provider cooldowns are respected; repeated clicks do not force upstream requests."})
	default:
		return dashboardJSON(404, map[string]string{"error": "Unknown dashboard endpoint."})
	}
}
