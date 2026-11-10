package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestDashboardI18nResource(t *testing.T) {
	res := handleDashboard(dashboardRequest{Method: "GET", Path: "/v0/resource/plugins/team-routing/i18n.js"})
	if res.StatusCode != 200 || !bytes.Equal(res.Body, i18nJS) || res.Headers.Get("Content-Type") != "text/javascript; charset=utf-8" || res.Headers.Get("Cache-Control") != "no-store" {
		t.Fatal("localization asset must be served locally without starting the worker")
	}
	html := string(dashboardHTML)
	if strings.Index(html, `src="./i18n.js"`) > strings.Index(html, `src="./editors.js"`) || !strings.Contains(html, `id="language"`) {
		t.Fatal("localization must load before both dashboard editors")
	}
}

// Check actual runtime metadata, including derived account labels/descriptions
// and parameterized rule summaries, rather than maintaining a second UI schema.
func TestDashboardI18nMetadataCoverage(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for the optional dashboard translation coverage check")
	}
	priority := accountPriorityField()
	messages := []string{priority["title"].(string), priority["description"].(string), "higher account priority"}
	for _, fields := range [][]policyField{policyFields(), providerFields(), accountReserveFields(), windowReserveFields()} {
		for _, field := range fields {
			messages = append(messages, field.Title, field.Description)
			for _, label := range field.EnumLabels {
				messages = append(messages, label)
			}
		}
	}
	for _, group := range policyGroups() {
		messages = append(messages, group["title"].(string))
	}
	for _, mode := range []string{"hours", "reset_schedule"} {
		shared := defaultSharedPolicy()
		shared.Enabled, shared.WeeklyReleaseMode = true, mode
		p := policy{Poll: 120, MaxAge: 600, SharedClaude: &shared, ProviderReserves: map[string]*sharedClaudePolicy{"codex": &shared}, AccountReserves: map[string]accountReserve{"example": {.01, .01}}}
		for _, rule := range evaluatedRules(p) {
			messages = append(messages, rule.Title, rule.Scope, rule.Summary)
		}
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "i18n.test.cjs", "--metadata")
	cmd.Stdin = bytes.NewReader(raw)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("runtime metadata has missing translations: %v\n%s", err, output)
	}
}
