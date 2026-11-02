package main

import (
	"fmt"
	"strings"
)

type modelRule struct {
	Action   string `json:"action"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// Resolve exactly once from the original client model. A pass-through exception
// also wins over a client's catch-all override. Targets are never remapped.
func resolveClientRoute(p policy, client, requested string) (target, bool, string) {
	if rule, ok := p.ModelRoutes[client][requested]; ok {
		if rule.Action == "requested" {
			return target{}, false, "exact model pass-through"
		}
		return target{Provider: rule.Provider, Model: rule.Model}, true, "exact client model override"
	}
	if t, ok := p.Routes[client]; ok {
		return t, true, "client default model override"
	}
	return target{}, false, "requested model"
}

func validateModelRules(values map[string]map[string]modelRule) error {
	if len(values) > 200 {
		return fmt.Errorf("too many client model rule sets")
	}
	for client, rules := range values {
		if client == "" || len(client) > 128 || len(rules) > 100 {
			return fmt.Errorf("invalid client model rule set")
		}
		for source, r := range rules {
			if source == "" || strings.TrimSpace(source) != source || len(source) > 256 || strings.ContainsAny(source, "\r\n\t") {
				return fmt.Errorf("enter an exact requested model ID on one line")
			}
			if r.Action == "requested" {
				if r.Provider != "" || r.Model != "" {
					return fmt.Errorf("pass-through rules cannot also override a target")
				}
			} else if r.Action != "override" || r.Provider == "" || r.Model == "" {
				return fmt.Errorf("choose pass-through or a complete override target")
			}
		}
	}
	return nil
}

func cloneModelRules(values map[string]map[string]modelRule) map[string]map[string]modelRule {
	out := map[string]map[string]modelRule{}
	for id, rules := range values {
		out[id] = map[string]modelRule{}
		for source, r := range rules {
			out[id][source] = r
		}
	}
	return out
}

func modelRulesSchema() map[string]any {
	return map[string]any{"type": "object", "maxProperties": 200, "additionalProperties": map[string]any{"type": "object", "maxProperties": 100, "propertyNames": map[string]any{"minLength": 1, "maxLength": 256}, "additionalProperties": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"action"}, "properties": map[string]any{"action": map[string]any{"type": "string", "enum": []string{"requested", "override"}}, "provider": map[string]any{"type": "string"}, "model": map[string]any{"type": "string"}}, "allOf": []any{map[string]any{"if": map[string]any{"properties": map[string]any{"action": map[string]any{"const": "override"}}}, "then": map[string]any{"required": []string{"provider", "model"}, "properties": map[string]any{"provider": map[string]any{"minLength": 1}, "model": map[string]any{"minLength": 1}}}}, map[string]any{"if": map[string]any{"properties": map[string]any{"action": map[string]any{"const": "requested"}}}, "then": map[string]any{"properties": map[string]any{"provider": map[string]any{"const": ""}, "model": map[string]any{"const": ""}}}}}}}}
}
