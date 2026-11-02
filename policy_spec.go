package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"strings"
)

// This registry is the single source of field bounds, labels and UI metadata.
// The JSON Schema and runtime validation both consume it.
type policyField struct {
	Path        string            `json:"path"`
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Group       string            `json:"group"`
	Minimum     *float64          `json:"minimum,omitempty"`
	Maximum     *float64          `json:"maximum,omitempty"`
	Enum        []string          `json:"enum,omitempty"`
	EnumLabels  map[string]string `json:"enum_labels,omitempty"`
	Format      string            `json:"format,omitempty"`
	Advanced    bool              `json:"advanced,omitempty"`
}

func bound(n float64) *float64 { return &n }
func policyGroups() []map[string]any {
	return []map[string]any{
		{"id": "accounts", "title": "Ownership"}, {"id": "reserves", "title": "Shared account protection"}, {"id": "schedule", "title": "Release schedule"}, {"id": "monitoring", "title": "Quota monitoring"},
		{"id": "ranking", "title": "Ranking estimates", "advanced": true}, {"id": "compatibility", "title": "Client compatibility", "advanced": true}, {"id": "billing", "title": "Billing safeguards", "advanced": true}, {"id": "diagnostics", "title": "Diagnostics", "advanced": true},
	}
}
func policyFields() []policyField {
	return []policyField{
		{Path: "owned_emails", Type: "array", Title: "Owned Claude accounts", Description: "Exempt from default reserves unless you set account-specific reserves. Ownership only breaks equal ranking scores.", Group: "accounts", Format: "email-list"},
		{Path: "shared_claude.enabled", Type: "boolean", Title: "Enable reserve protection", Description: "Enable default and account-specific reserves. Owned accounts are exempt from defaults.", Group: "reserves"},
		{Path: "shared_claude.weekly_remaining_floor", Type: "number", Title: "Default weekly reserve", Description: "Percentage preserved until release when an account has no custom reserve.", Group: "reserves", Minimum: bound(0), Maximum: bound(maximumReserveFraction), Format: "percent"},
		{Path: "shared_claude.five_hour_remaining_floor", Type: "number", Title: "Default five-hour reserve", Description: "Percentage preserved for a whole overlapping session when an account has no custom reserve.", Group: "reserves", Minimum: bound(0), Maximum: bound(maximumReserveFraction), Format: "percent"},
		{Path: "shared_claude.weekly_release_mode", Type: "string", Title: "Weekly reserve release", Description: "Use a fixed lead time, or the final daytime window / preceding evening schedule.", Group: "schedule", Enum: []string{"hours", "reset_schedule"}, EnumLabels: map[string]string{"hours": "Fixed lead time", "reset_schedule": "Final daytime window / preceding evening"}},
		{Path: "shared_claude.weekly_release_within_hours", Type: "integer", Title: "Release lead time (hours)", Description: "Fixed-hours mode: time before reset. Scheduled mode: final window for daytime resets on protected days.", Group: "schedule", Minimum: bound(1), Maximum: bound(72)},
		{Path: "shared_claude.protected_weekdays", Type: "array", Title: "Protected weekdays", Description: "Weekly daytime releases and five-hour session protection use these days.", Group: "schedule", Format: "weekdays"},
		{Path: "shared_claude.protected_start_hour", Type: "integer", Title: "Protected start hour", Description: "Local hour when protected time starts.", Group: "schedule", Minimum: bound(0), Maximum: bound(23)},
		{Path: "shared_claude.protected_end_hour", Type: "integer", Title: "Protected end hour", Description: "The end minute is included. An end hour of 20 covers through 20:00; night starts at 20:01.", Group: "schedule", Minimum: bound(1), Maximum: bound(24)},
		{Path: "shared_claude.timezone", Type: "string", Title: "Schedule time zone", Description: "IANA time zone, such as Asia/Karachi. Leave empty to use proxy system time.", Group: "schedule", Format: "time-zone"},
		{Path: "poll_seconds", Type: "integer", Title: "Quota polling interval (seconds)", Description: "Refresh due readings; provider backoff is always respected.", Group: "monitoring", Minimum: bound(15), Maximum: bound(600)},
		{Path: "max_quota_age_seconds", Type: "integer", Title: "Quota age upper bound (seconds)", Description: "Claude also uses the stricter polling interval plus grace. The effective limit is shown in evaluated rules.", Group: "monitoring", Minimum: bound(30), Maximum: bound(3600), Advanced: true},
		{Path: "weekly_five_hour_ratio", Type: "number", Title: "Initial weekly conversion", Description: "Estimated five-hour allowances per weekly allowance. Valid paired samples learn a per-account conversion.", Group: "ranking", Minimum: bound(1), Maximum: bound(30), Advanced: true},
		{Path: "claude_stateless_threads", Type: "boolean", Title: "Full-history Claude requests", Description: "Negotiate account-local Message Threads off. Clients must handle the unsupported-beta retry; normal prompt caching remains.", Group: "compatibility", Advanced: true},
		{Path: "require_extra_usage_disabled", Type: "boolean", Title: "Require provider credits disabled", Description: "Strict mode excludes Claude accounts whose credits are enabled or unconfirmed. Off allows best-effort included-quota routing; zero paid spillover cannot be guaranteed.", Group: "billing", Advanced: true},
		{Path: "record_routing_events", Type: "boolean", Title: "Record sanitized diagnostics", Description: "Record private routing, quota and cache evidence without prompts or credentials.", Group: "diagnostics", Advanced: true},
	}
}
func jsonObject(v any) map[string]any {
	raw, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}
func atPath(m map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var v any = m
	for _, p := range parts {
		node, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok = node[p]
		if !ok {
			return nil, false
		}
	}
	return v, true
}
func policySchema() map[string]any {
	root := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "title": "Team Routing policy", "type": "object", "additionalProperties": false, "properties": map[string]any{}}
	props := root["properties"].(map[string]any)
	for _, f := range policyFields() {
		parent := props
		parts := strings.Split(f.Path, ".")
		for _, part := range parts[:len(parts)-1] {
			if parent[part] == nil {
				parent[part] = map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
			}
			parent = parent[part].(map[string]any)["properties"].(map[string]any)
		}
		node := map[string]any{"type": f.Type, "title": f.Title, "description": f.Description}
		if f.Minimum != nil {
			node["minimum"] = *f.Minimum
		}
		if f.Maximum != nil {
			node["maximum"] = *f.Maximum
		}
		if len(f.Enum) > 0 {
			node["enum"] = f.Enum
		}
		if f.Type == "array" {
			node["items"] = map[string]any{"type": "string"}
			if f.Format == "weekdays" {
				node["items"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 6}
				node["minItems"] = 1
				node["uniqueItems"] = true
			} else {
				node["maxItems"] = 100
			}
		}
		parent[parts[len(parts)-1]] = node
	}
	props["routes"] = map[string]any{"type": "object", "maxProperties": 200, "additionalProperties": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"provider", "model"}, "properties": map[string]any{"provider": map[string]any{"type": "string", "minLength": 1}, "model": map[string]any{"type": "string", "minLength": 1}}}}
	props["client_labels"] = map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string", "maxLength": 80}}
	props["account_reserves"] = accountReserveSchema()
	return root
}
func validatePolicy(p policy) error {
	m := jsonObject(p)
	if p.SharedClaude != nil && p.SharedClaude.WeeklyReleaseMode == "" {
		m["shared_claude"].(map[string]any)["weekly_release_mode"] = "hours"
	}
	for _, f := range policyFields() {
		v, exists := atPath(m, f.Path)
		if !exists || v == nil {
			continue
		}
		switch f.Type {
		case "integer", "number":
			n, ok := v.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) || (f.Type == "integer" && math.Trunc(n) != n) {
				return fmt.Errorf("%s must be %s", f.Title, f.Type)
			}
			if f.Minimum != nil && n < *f.Minimum || f.Maximum != nil && n > *f.Maximum {
				return fmt.Errorf("%s is outside its allowed range", f.Title)
			}
		case "string":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be text", f.Title)
			}
			if len(f.Enum) > 0 {
				found := false
				for _, e := range f.Enum {
					found = found || s == e
				}
				if !found {
					return fmt.Errorf("%s has an unsupported value", f.Title)
				}
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s must be true or false", f.Title)
			}
		case "array":
			if _, ok := v.([]any); !ok {
				return fmt.Errorf("%s must be a list", f.Title)
			}
		}
	}
	if p.MaxAge < p.Poll {
		return fmt.Errorf("quota age upper bound must be at least the polling interval")
	}
	if len(p.Owned) > 100 || len(p.Routes) > 200 {
		return fmt.Errorf("too many accounts or routes")
	}
	for _, email := range p.Owned {
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email || len(email) > 254 {
			return fmt.Errorf("enter plain owned-account email addresses")
		}
	}
	if err := validateAccountReserves(p.AccountReserves); err != nil {
		return err
	}
	return validateSharedPolicy(p.SharedClaude)
}

type evaluatedRule struct {
	ID         string         `json:"id"`
	Title      string         `json:"title"`
	Priority   int            `json:"priority"`
	Scope      string         `json:"scope"`
	Enabled    bool           `json:"enabled"`
	Summary    string         `json:"summary"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

func evaluatedRules(p policy) []evaluatedRule {
	fresh := p.Poll + quotaAgeGraceSeconds
	if fresh > p.MaxAge {
		fresh = p.MaxAge
	}
	rules := []evaluatedRule{
		{ID: "availability", Title: "Provider availability", Priority: 10, Scope: "All requests", Enabled: true, Summary: "Enabled credentials must advertise the exact requested model and pass native cooldown checks."},
		{ID: "billing", Title: "Included subscription usage", Priority: 20, Scope: "Claude", Enabled: true, Summary: "Paid-only requests are rejected. Paid-use signals block an account until fresh usable allowance is confirmed in a newer window.", Parameters: map[string]any{"require_provider_credits_off": p.RequireCreditsOff}},
		{ID: "freshness", Title: "Quota freshness", Priority: 30, Scope: "Claude / Codex", Enabled: true, Summary: fmt.Sprintf("Claude readings must be no older than %d seconds. Provider refresh backoff remains in force.", fresh), Parameters: map[string]any{"claude_effective_seconds": fresh, "poll_seconds": p.Poll, "upper_bound_seconds": p.MaxAge}},
	}
	if s := p.SharedClaude; s != nil {
		summary := fmt.Sprintf("Default reserves: %.0f%% weekly and %.0f%% five-hour on non-owned Claude accounts without a custom rule. Five-hour protection covers the whole overlapping session.", s.WeeklyFloor*100, s.FiveHourFloor*100)
		rules = append(rules, evaluatedRule{ID: "shared_reserves", Title: "Shared account reserves", Priority: 40, Scope: "Shared Claude", Enabled: s.Enabled, Summary: summary, Parameters: jsonObject(s)})
		rules = append(rules, evaluatedRule{ID: "account_reserves", Title: "Account-specific reserves", Priority: 45, Scope: "Configured Claude accounts", Enabled: s.Enabled, Summary: fmt.Sprintf("%d custom account rules replace the default percentages, including an owned account only when explicitly configured. The same protection and release schedules apply.", len(p.AccountReserves)), Parameters: map[string]any{"accounts": p.AccountReserves}})
		lead := fmt.Sprintf("Unlock the weekly reserve %d hours before reset.", s.WeeklyReleaseHours)
		if s.WeeklyReleaseMode == "reset_schedule" {
			lead = fmt.Sprintf("Protected-day daytime resets unlock %d hours before reset. All other resets unlock at the preceding %02d:00, including unprotected-day daytime resets.", s.WeeklyReleaseHours, s.EndHour)
		}
		rules = append(rules, evaluatedRule{ID: "weekly_release", Title: "Weekly release schedule", Priority: 50, Scope: "Shared Claude", Enabled: s.Enabled, Summary: lead, Parameters: jsonObject(s)})
	}
	return append(rules,
		evaluatedRule{ID: "admission", Title: "Request admission", Priority: 60, Scope: "Claude", Enabled: true, Summary: "Keep completed work reserved until a newer reading covers it. Response headers may lower cached headroom; old responses cannot raise it.", Parameters: map[string]any{"completion_five_hour_fraction": fiveRequestFraction, "completion_weekly_fraction": weeklyRequestFraction, "settlement_seconds": settlementLagSeconds, "header_rounding_fraction": headerRoundingFraction, "protected_five_hour_buffer": fiveProtectedBuffer, "protected_weekly_buffer": weeklyProtectedBuffer}},
		evaluatedRule{ID: "ranking", Title: "Reset-aware ranking", Priority: 70, Scope: "Eligible accounts", Enabled: true, Summary: "Rank usable weekly allowance against remaining five-hour opportunities. Ownership only breaks exactly equal scores; learned conversion replaces the initial estimate.", Parameters: map[string]any{"initial_weekly_conversion": p.WeeklyRatio, "expiry_within_seconds": expiryWindowSeconds, "minimum_predicted_waste": expiryMinimumWaste}},
		evaluatedRule{ID: "affinity", Title: "Cache affinity", Priority: 80, Scope: "Similar-urgency conversations", Enabled: true, Summary: "Keep warm conversations on their account unless eligibility or materially greater expiry urgency requires a switch. Prompts and cache controls are preserved.", Parameters: map[string]any{"warm_seconds": warmCacheSeconds, "ordinary_switch_fraction": ordinarySwitchFraction, "warm_switch_fraction": warmSwitchFraction, "weekly_rescue_hours": weeklyRescueHours}},
	)
}
