package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func diagnosticFixture(t *testing.T, enabled bool) {
	t.Helper()
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	p := defaultPolicy()
	p.RecordRouting = enabled
	b, _ := json.Marshal(p)
	if err := os.WriteFile("team-routing.json", b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestDiagnosticFailureKeepsStructuredQuotaDetailsWithoutSecretText(t *testing.T) {
	body := `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"PROMPT_SECRET cpa_secret bearer password","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED","domain":"googleapis.com","metadata":{"quota_metric":"generate_content_input_tokens","quota_limit_value":"25000","consumer":"PROJECT_SECRET","model":"gemini-3.8-flash-high","service":"cpa_secret"}},{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"128.5s"},{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaMetric":"generate_requests","quotaId":"RequestsPerMinute","subject":"ACCOUNT_SECRET","description":"PROMPT_SECRET"}]}]}}`
	r := failureDiagnostic(body, 429)
	b, _ := json.Marshal(r)
	s := string(b)
	for _, forbidden := range []string{"PROMPT_SECRET", "cpa_secret", "PROJECT_SECRET", "ACCOUNT_SECRET", "password"} {
		if strings.Contains(s, forbidden) {
			t.Fatal("diagnostic exposed sensitive data", forbidden)
		}
	}
	for _, wanted := range []string{"RESOURCE_EXHAUSTED", "RATE_LIMIT_EXCEEDED", "128.5s", "25000", "RequestsPerMinute"} {
		if !strings.Contains(s, wanted) {
			t.Fatal("useful diagnostic missing", wanted)
		}
	}
	if r["structured_details_available"] != true || r["body_hash"] != digest(body) {
		t.Fatal(r)
	}
	wrapped := failureDiagnostic("All credentials cooling down (last error: "+body+")", 429)
	if wrapped["class"] != "credential_cooldown" || wrapped["structured_details_available"] != true {
		t.Fatal(wrapped)
	}
}

func TestDiagnosticUnstructuredErrorsDoNotClaimMissingProviderReasons(t *testing.T) {
	for _, body := range []string{"PROMPT_SECRET", `{"error":{"code":429,"message":"Resource has been exhausted"}}`, strings.Repeat("x", 70000)} {
		r := failureDiagnostic(body, 429)
		b, _ := json.Marshal(r)
		if r["class"] != "rate_limited" || r["structured_details_available"] != false || strings.Contains(string(b), "PROMPT_SECRET") {
			t.Fatal(r)
		}
	}
}

func TestDiagnosticRequestShapeAndCorrelationNeverRecordContents(t *testing.T) {
	diagnosticFixture(t, true)
	body := []byte(`{"max_tokens":128000,"messages":[{"role":"user","content":"PROMPT_SECRET"}],"system":"SYSTEM_SECRET","tools":[{"name":"TOOL_SECRET","input_schema":{"secret":"SCHEMA_SECRET"}}],"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`)
	r := diagnosticRequest{RequestID: "request-native", TraceID: "trace-native", RequestedModel: "claude-haiku-5-5", Model: "gemini-3.8-flash-high", Headers: http.Header{"X-Api-Key": {"cpa_secret"}}, Body: body, Metadata: map[string]any{"selection_id": "selection-one", "selected_auth_id": "fixture"}}
	b, _ := json.Marshal(r)
	recordRequestDiagnostic("request.intercept_before", b)
	recordRequestDiagnostic("request.intercept_after", b)
	r.Outcome, r.StatusCode, r.Error = "failed", 429, "All credentials cooling down PROMPT_SECRET"
	r.StartedAt, r.CompletedAt = time.Now().Add(-time.Second), time.Now()
	b, _ = json.Marshal(r)
	recordRequestDiagnostic("request.complete", b)
	journal, err := os.ReadFile(diagnosticJournal)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"PROMPT_SECRET", "SYSTEM_SECRET", "TOOL_SECRET", "SCHEMA_SECRET", "cpa_secret", "request-native", "trace-native"} {
		if strings.Contains(string(journal), forbidden) {
			t.Fatal("content or raw identifier leaked", forbidden)
		}
	}
	lines := strings.Split(strings.TrimSpace(string(journal)), "\n")
	if len(lines) != 3 {
		t.Fatal(len(lines))
	}
	for _, line := range lines {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil || event["request_hash"] != digest(r.RequestID) || event["trace_hash"] != digest(r.TraceID) {
			t.Fatal(line)
		}
	}
	shape := diagnosticShape(body)
	if shape["message_count"] != 1 || shape["tool_count"] != 1 || shape["max_output_tokens"] != int64(128000) || shape["thinking_type"] != "adaptive" {
		t.Fatal(shape)
	}
	if diagnosticShape(make([]byte, (8<<20)+1))["shape_inspected"] != false {
		t.Fatal("unbounded shape inspection")
	}
}

func TestDiagnosticHeadersDropCredentialsAndMalformedRetryValues(t *testing.T) {
	h := http.Header{"Authorization": {"Bearer SECRET"}, "Set-Cookie": {"COOKIE_SECRET"}, "Retry-After": {"128"}, "X-Request-Id": {"REQUEST_SECRET"}, "X-Ratelimit-Remaining-Tokens": {"15"}, "X-Ratelimit-Limit-Tokens": {"TOKEN_SECRET"}}
	r := diagnosticHeaders(h)
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "SECRET") || r["retry_after_seconds"] != float64(128) || r["x-ratelimit-remaining-tokens"] != float64(15) {
		t.Fatal(r)
	}
	h.Set("Retry-After", "NaN")
	if _, ok := diagnosticHeaders(h)["retry_after_seconds"]; ok {
		t.Fatal("invalid retry accepted")
	}
}

func TestDiagnosticUsageRetainsEveryCallbackAndBillingAccountingDeduplicates(t *testing.T) {
	diagnosticFixture(t, true)
	resetCacheEvidence(t)
	u := usageEvidence{RequestID: "attempt", TraceID: "trace", AuthID: "fixture", Provider: "antigravity", Model: "gemini-3.8-flash-high", BaseURL: "https://USER_SECRET:PASSWORD_SECRET@example.com/v1?key=QUERY_SECRET", Failed: true}
	u.Failure.StatusCode, u.Failure.Body = 429, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"PROMPT_SECRET"}}`
	b, _ := json.Marshal(u)
	for i := 0; i < 2; i++ {
		if _, err := observeUsage(b); err != nil {
			t.Fatal(err)
		}
	}
	journal, err := os.ReadFile(diagnosticJournal)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSpace(string(journal)), "\n")) != 2 {
		t.Fatal("retry evidence lost")
	}
	if strings.Contains(string(journal), "SECRET") {
		t.Fatal("upstream URL or message exposed")
	}
	evidence.Lock()
	counter := evidence.Counters["fixture|gemini-3.8-flash-high"]
	evidence.Unlock()
	if counter.Requests != 1 {
		t.Fatal("accounting dedup changed")
	}
}

func TestDiagnosticDisabledRotationAndWriteFailureVisibility(t *testing.T) {
	diagnosticFixture(t, false)
	b, _ := json.Marshal(diagnosticRequest{RequestID: "id", Model: "fixture"})
	recordRequestDiagnostic("request.complete", b)
	recordUsageDiagnostic(usageEvidence{RequestID: "id"})
	if _, err := os.Stat(diagnosticJournal); !os.IsNotExist(err) {
		t.Fatal("disabled logging wrote journal")
	}
	if err := os.WriteFile(diagnosticJournal, []byte(strings.Repeat("x", (8<<20)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	appendEvidence(diagnosticJournal, map[string]string{"kind": "rotation_test"})
	if _, err := os.Stat(diagnosticJournal + ".previous"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("blocked", []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join("blocked", "unwritable.jsonl")
	appendEvidence(name, map[string]string{"kind": "write_test"})
	evidenceHealth.Lock()
	status := evidenceHealth.Files[name]
	delete(evidenceHealth.Files, name)
	evidenceHealth.Unlock()
	if status.Failures == 0 || status.ErrorClass == "" {
		t.Fatal("write failure silently lost", status)
	}
}

func TestDiagnosticQuotaFailuresAndManagementTraffic(t *testing.T) {
	diagnosticFixture(t, true)
	management, _ := json.Marshal(diagnosticRequest{RequestID: "management", Outcome: "succeeded", StatusCode: 200})
	recordRequestDiagnostic("request.complete", management)
	if _, err := os.Stat(diagnosticJournal); !os.IsNotExist(err) {
		t.Fatal("management traffic polluted inference diagnostics")
	}
	_, err := fetchProviderQuota(quotaAccount{ID: "fixture", Provider: "claude"}, func(quotaAccount, string, string, map[string]string, string) (quotaHTTP, error) {
		return quotaHTTP{Code: 429, Body: `{"error":{"code":429,"message":"PROMPT_SECRET","status":"RESOURCE_EXHAUSTED"}}`, Header: http.Header{"Retry-After": {"45"}}}, nil
	})
	if err == nil {
		t.Fatal("expected quota failure")
	}
	journal, readErr := os.ReadFile(diagnosticJournal)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var event map[string]any
	if json.Unmarshal(journal, &event) != nil || event["kind"] != "quota_read" || event["failed"] != true || event["status_code"] != float64(429) || strings.Contains(string(journal), "PROMPT_SECRET") {
		t.Fatal(string(journal))
	}
}
