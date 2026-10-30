//go:build abiprobe

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestABILongConversationKeepsClientOverride(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { stopWorker(); applyConfig(nil) })
	p := defaultPolicy()
	p.Routes[digest("test-client")] = target{Provider: "claude", Model: "claude-sonnet-4-6"}
	policyJSON, _ := json.Marshal(p)
	if err := os.WriteFile("team-routing.json", policyJSON, 0600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"Headers":            map[string][]string{"Authorization": {"Bearer test-client"}},
		"AvailableProviders": []string{"claude"},
		"Body":               []byte(strings.Repeat("long conversation ", 200000)),
	})
	if len(body) <= 2<<20 {
		t.Fatal("fixture does not exceed old ABI limit")
	}
	var output envelope
	if err := json.Unmarshal(probeABICall("model.route", body, uint64(len(body))), &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Handled     bool
		TargetModel string
	}
	if err := json.Unmarshal(output.Result, &result); err != nil {
		t.Fatalf("%s: %v", output.Result, err)
	}
	if !output.OK || !result.Handled || result.TargetModel != "claude-sonnet-4-6" {
		t.Fatalf("large conversation bypassed override: %+v %+v", output, result)
	}
}

func TestABIRejectsInvalidBufferBeforeCopy(t *testing.T) {
	for _, length := range []uint64{1, maxABIRequestBytes + 1} {
		var output envelope
		if err := json.Unmarshal(probeABICall("model.route", nil, length), &output); err != nil {
			t.Fatal(err)
		}
		if output.OK || output.Error == nil {
			t.Fatalf("invalid buffer accepted: %+v", output)
		}
	}
}
