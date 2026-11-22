package main

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const claudeQuotaRecoveryPath = dashboardAPI + "/recover-claude-quota"

func recoverClaudeQuota(body []byte, caller managementCaller) dashboardResponse {
	var input struct {
		AuthID string `json:"auth_id"`
	}
	if len(body) > 4096 || json.Unmarshal(body, &input) != nil || strings.TrimSpace(input.AuthID) == "" {
		return dashboardJSON(400, map[string]string{"error": "Specify one Claude auth_id."})
	}
	// Only trust the configured management server, never request-supplied origins.
	settings.Lock()
	caller.BaseURL = settings.ManagementURL
	settings.Unlock()
	if caller.BaseURL == "" {
		caller.BaseURL = "http://127.0.0.1:8317"
	}
	var inventory struct {
		Files []struct {
			hostAuthFile
			AdoptedAt      time.Time `json:"adopted_at"`
			RequiresSignIn *bool     `json:"requires_sign_in"`
		} `json:"files"`
	}
	if err := localJSON("GET", "/v8/management/credentials", &inventory, caller); err != nil {
		return dashboardJSON(503, map[string]string{"error": "Unable to verify credential adoption with the proxy."})
	}
	for _, f := range inventory.Files {
		if f.ID != input.AuthID || f.provider() != "claude" || f.Disabled || f.Index == "" {
			continue
		}
		if f.RequiresSignIn == nil || *f.RequiresSignIn || f.AdoptedAt.IsZero() {
			return dashboardJSON(409, map[string]string{"error": "No confirmed Claude grant adoption."})
		}
		raw, err := hostAuthJSON(f.Index)
		if err != nil {
			return dashboardJSON(503, map[string]string{"error": "Unable to read the renewed credential."})
		}
		a := quotaAccount{ID: f.ID, Index: f.Index, Provider: "claude", Storage: raw}
		result, err := monitoredQuotaAfterAdoption(a, f.AdoptedAt)
		cache := cacheDetails(a)
		if errors.Is(err, errQuotaRecoveryNotDue) {
			return dashboardJSON(409, map[string]any{"error": err.Error(), "cache": cache})
		}
		if err != nil {
			return dashboardJSON(502, map[string]any{"error": "No fresh quota was obtained.", "cache": cache})
		}
		return dashboardJSON(200, map[string]any{"quota_at": result.At, "cache": cache})
	}
	return dashboardJSON(404, map[string]string{"error": "Enabled Claude account not found."})
}
