package main

import (
	"fmt"
	"strings"
	"time"
)

type routeEvaluationInput struct {
	Model    string `json:"model"`
	Provider string `json:"provider,omitempty"`
	ClientID string `json:"client_id,omitempty"`
}

func evaluateRoute(input routeEvaluationInput, caller managementCaller) (any, error) {
	p, err := readPolicy()
	if err != nil {
		return nil, err
	}
	routingReason := "requested model"
	if input.ClientID != "" {
		clients, _, err := activeClientInventory(p, caller)
		if err != nil {
			return nil, err
		}
		found := false
		for _, c := range clients.Clients {
			found = found || digest(c.Key) == input.ClientID
		}
		if !found {
			return nil, fmt.Errorf("client key is no longer active")
		}
		route, ok, reason := resolveClientRoute(p, input.ClientID, input.Model)
		routingReason = reason
		if ok {
			input.Provider, input.Model = route.Provider, route.Model
		}
	}
	if strings.TrimSpace(input.Model) == "" {
		return nil, fmt.Errorf("choose a requested model to evaluate")
	}
	entries, err := fetchHostModelInventory()
	if err != nil {
		return nil, fmt.Errorf("runtime model registry is unavailable")
	}
	files, err := hostAuthList()
	if err != nil {
		return nil, err
	}
	byID := map[string]hostAuthFile{}
	for _, f := range files {
		byID[f.ID] = f
	}
	now := time.Now()
	counts := activeCounts(now)
	rows := []map[string]any{}
	eligible := 0
	state.Lock()
	defer state.Unlock()
	for _, entry := range entries {
		if input.Provider != "" && entry.Provider != input.Provider {
			continue
		}
		matches := false
		for _, id := range entry.Models {
			matches = matches || id == input.Model
		}
		if !matches {
			continue
		}
		f := byID[entry.ID]
		label := f.Email
		if label == "" {
			label = f.Name
		}
		if label == "" {
			label = entry.ID
		}
		c := candidate{ID: entry.ID, Provider: entry.Provider, Metadata: map[string]any{"email": f.Email, "native_unavailable": f.Unavailable, "native_retry_after": f.NextRetry}}
		rank, ok := rankAccount(p, c, input.Model, state.Quota[entry.ID], now, counts[entry.ID])
		if f.Disabled {
			ok = false
			rank.Reason = "Disabled in proxy"
		}
		if ok {
			eligible++
		}
		rows = append(rows, map[string]any{"account_id": entry.ID, "label": label, "provider": entry.Provider, "eligible": ok, "reason": rank.Reason, "assessment": rank, "reserves": effectiveAccountReserve(p, c)})
	}
	note := "Provider translation does not guarantee Anthropic server-side safeguards or no-charge classifier eligibility."
	if input.Provider == "claude" {
		note = "Native Claude traffic preserves safeguards fields; eligibility still requires genuine upstream safeguard results."
	}
	return map[string]any{"model": input.Model, "provider": input.Provider, "routing_reason": routingReason, "protocol_note": note, "eligible_count": eligible, "matching_count": len(rows), "candidates": rows, "at": now, "note": "Uses cached quota and runtime availability; no inference or forced quota refresh is performed."}, nil
}
