package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

type catalogModel struct {
	ID        string   `json:"id"`
	Providers []string `json:"providers"`
}
type catalogProvider struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Models int    `json:"model_count"`
}
type routingCatalog struct {
	Models    []catalogModel    `json:"models"`
	Providers []catalogProvider `json:"providers"`
	Warning   string            `json:"warning,omitempty"`
}

var fetchHostModelInventory = func() ([]hostCredentialModels, error) {
	raw, err := hostCall("host.auth.models", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct{ Credentials []hostCredentialModels }
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Credentials, nil
}

func providerLabel(id string) string {
	labels := map[string]string{"claude": "Claude", "codex": "OpenAI / Codex", "antigravity": "Antigravity", "gemini": "Gemini API", "gemini-cli": "Gemini CLI"}
	if s := labels[id]; s != "" {
		return s
	}
	return id
}
func loadRoutingCatalog(callers ...managementCaller) routingCatalog {
	entries, err := fetchHostModelInventory()
	models := map[string]map[string]bool{}
	for _, entry := range entries {
		for _, id := range entry.Models {
			if models[id] == nil {
				models[id] = map[string]bool{}
			}
			models[id][entry.Provider] = true
		}
	}
	var public struct{ Data []struct{ ID string } }
	if len(callers) > 0 {
		if e := localJSON("GET", "/v1/models", &public, callers...); e == nil {
			for _, m := range public.Data {
				if models[m.ID] == nil {
					models[m.ID] = map[string]bool{}
				}
			}
		}
	}
	out := routingCatalog{Models: []catalogModel{}, Providers: []catalogProvider{}}
	if err != nil {
		out.Warning = "The runtime provider registry is unavailable. Unmapped models cannot be selected for an override."
	}
	counts := map[string]int{}
	for id, providers := range models {
		p := []string{}
		for provider := range providers {
			if provider != "" {
				p = append(p, provider)
				counts[provider]++
			}
		}
		sort.Strings(p)
		out.Models = append(out.Models, catalogModel{id, p})
	}
	sort.Slice(out.Models, func(i, j int) bool { return out.Models[i].ID < out.Models[j].ID })
	for id, n := range counts {
		out.Providers = append(out.Providers, catalogProvider{id, providerLabel(id), n})
	}
	sort.Slice(out.Providers, func(i, j int) bool { return out.Providers[i].Label < out.Providers[j].Label })
	return out
}
func (c routingCatalog) supports(t target) bool {
	for _, m := range c.Models {
		if m.ID == t.Model {
			for _, p := range m.Providers {
				if p == t.Provider {
					return true
				}
			}
		}
	}
	return false
}
func validateRouteTarget(c routingCatalog, t target) error {
	if !c.supports(t) {
		return fmt.Errorf("model %q is not advertised by provider %q; reload the catalog or choose a supported pair", t.Model, t.Provider)
	}
	return nil
}
