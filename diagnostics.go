package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The journal is an allowlist, never a serialization of callback arguments.
// Failure bodies can contain prompts, credentials, URLs and account identifiers.
const diagnosticJournal = "team-routing-diagnostics.jsonl"

var diagnosticAtom = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/() -]{0,191}$`)
var diagnosticDelay = regexp.MustCompile(`^[0-9]{1,7}(\.[0-9]{1,9})?s$`)

func diagnosticLabel(s string) string {
	// Free text is deliberately not accepted, even if it looks innocuous.
	if strings.ContainsAny(s, " @?=\\\r\n\t") || !diagnosticAtom.MatchString(s) {
		return ""
	}
	lower := strings.ToLower(s)
	for _, prefix := range []string{"bearer", "cpa_", "sk-", "sk_", "eyj"} {
		if strings.HasPrefix(lower, prefix) {
			return ""
		}
	}
	return s
}

func optionalDigest(s string) string {
	if s == "" {
		return ""
	}
	return digest(s)
}

type diagnosticRequest struct {
	RequestID, TraceID, SourceFormat, RequestedModel, Model, ToFormat string
	Stream                                                            bool
	Headers                                                           http.Header
	Body                                                              []byte
	Metadata                                                          map[string]any
	Outcome, Error                                                    string
	StatusCode                                                        int
	StartedAt, CompletedAt                                            time.Time
}

func recordRequestDiagnostic(method string, raw []byte) {
	p, err := readPolicy()
	if err != nil || !p.RecordRouting {
		return
	}
	var r diagnosticRequest
	if json.Unmarshal(raw, &r) != nil || r.RequestID == "" {
		return
	}
	kind := map[string]string{"request.intercept_before": "request_received", "request.intercept_after": "credential_selected", "request.complete": "request_complete"}[method]
	if kind == "" {
		return
	}
	// The host also completes management/health HTTP requests. They have no
	// model execution and would drown out useful inference evidence.
	if r.Model == "" && r.RequestedModel == "" {
		return
	}
	event := map[string]any{"schema_version": 1, "plugin_version": pluginVersion, "kind": kind, "at": time.Now(), "request_hash": digest(r.RequestID), "trace_hash": optionalDigest(r.TraceID), "requested_model": diagnosticLabel(r.RequestedModel), "model": diagnosticLabel(r.Model), "source_format": diagnosticLabel(r.SourceFormat), "stream": r.Stream}
	for _, name := range []string{"selection_id", "selected_auth_id"} {
		if value, ok := r.Metadata[name].(string); ok {
			// Auth IDs already identify accounts in the existing private journals.
			if name == "selected_auth_id" {
				event["auth_id"] = value
			} else {
				event[name] = value
			}
		}
	}
	if round, ok := r.Metadata["retry_round"].(float64); ok && round >= 0 && round < 100 {
		event["retry_round"] = round
	}
	if method == "request.intercept_before" {
		event["client_hash"] = optionalDigest(key(r.Headers))
		event["shape"] = diagnosticShape(r.Body)
		policyJSON, _ := json.Marshal(p)
		event["policy_hash"] = digest(string(policyJSON))
	}
	if method == "request.complete" {
		event["outcome"] = diagnosticLabel(r.Outcome)
		event["status_code"] = r.StatusCode
		if !r.StartedAt.IsZero() {
			event["started_at"] = r.StartedAt
			if !r.CompletedAt.Before(r.StartedAt) {
				event["duration_ms"] = r.CompletedAt.Sub(r.StartedAt).Milliseconds()
			}
		}
		if !r.CompletedAt.IsZero() {
			event["at"] = r.CompletedAt
		}
		if r.Error != "" || r.Outcome == "failed" {
			event["failure"] = failureDiagnostic(r.Error, r.StatusCode)
		}
	}
	appendEvidence(diagnosticJournal, event)
}

func diagnosticShape(body []byte) map[string]any {
	out := map[string]any{"body_bytes": len(body), "contents_recorded": false}
	if len(body) == 0 || len(body) > 8<<20 {
		out["shape_inspected"] = false
		return out
	}
	var b struct {
		Messages, Tools                       []json.RawMessage
		System, Contents, Input, Instructions json.RawMessage
		MaxTokens, MaxCompletionTokens        int64
		MaxOutputTokens                       int64 `json:"max_output_tokens"`
		MaxTokensSnake                        int64 `json:"max_tokens"`
		MaxCompletionTokensSnake              int64 `json:"max_completion_tokens"`
		Thinking                              struct {
			Type   string
			Budget int64 `json:"budget_tokens"`
		}
		Output struct{ Effort string } `json:"output_config"`
	}
	if json.Unmarshal(body, &b) != nil {
		out["shape_inspected"] = false
		return out
	}
	out["shape_inspected"] = true
	out["message_count"], out["tool_count"] = len(b.Messages), len(b.Tools)
	messageBytes, toolBytes := 0, 0
	for _, m := range b.Messages {
		messageBytes += len(m)
	}
	for _, tool := range b.Tools {
		toolBytes += len(tool)
	}
	out["message_bytes"], out["tool_schema_bytes"], out["system_bytes"] = messageBytes, toolBytes, len(b.System)
	out["input_bytes"], out["contents_bytes"], out["instructions_bytes"] = len(b.Input), len(b.Contents), len(b.Instructions)
	out["max_output_tokens"] = max(b.MaxTokens, b.MaxTokensSnake, b.MaxCompletionTokens, b.MaxCompletionTokensSnake, b.MaxOutputTokens)
	out["thinking_type"] = diagnosticLabel(b.Thinking.Type)
	out["thinking_budget_tokens"] = b.Thinking.Budget
	out["effort"] = diagnosticLabel(b.Output.Effort)
	return out
}

func failureDiagnostic(body string, status int) map[string]any {
	out := map[string]any{"status_code": status, "body_bytes": len(body), "body_hash": optionalDigest(body), "raw_body_recorded": false, "class": "unknown"}
	lower := strings.ToLower(body)
	switch {
	case strings.Contains(lower, "cooling down") || strings.Contains(lower, "cooldown"):
		out["class"] = "credential_cooldown"
	case strings.Contains(lower, "no eligible"):
		out["class"] = "no_eligible_account"
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "econnrefused"):
		out["class"] = "connection_refused"
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "name resolution"):
		out["class"] = "dns_error"
	case strings.Contains(lower, "connection reset"):
		out["class"] = "connection_reset"
	case strings.Contains(lower, "tls handshake") || strings.Contains(lower, "x509:"):
		out["class"] = "tls_error"
	case strings.Contains(lower, "unexpected eof") || strings.TrimSpace(lower) == "eof":
		out["class"] = "connection_closed"
	case strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timed out"):
		out["class"] = "timeout"
	case strings.Contains(lower, "context canceled"):
		out["class"] = "canceled"
	case status == 429:
		out["class"] = "rate_limited"
	case status == 401:
		out["class"] = "authentication"
	case status == 403:
		out["class"] = "permission_denied"
	case status >= 500:
		out["class"] = "upstream_server_error"
	case status == 400 || status == 422:
		out["class"] = "invalid_request"
	}
	// Parse structured provider diagnostics, including JSON wrapped in native errors.
	if len(body) > 64<<10 {
		body = body[:64<<10]
		out["inspection_truncated"] = true
	}
	var envelope map[string]json.RawMessage
	start := strings.IndexByte(body, '{')
	if start < 0 || json.NewDecoder(strings.NewReader(body[start:])).Decode(&envelope) != nil {
		out["structured_details_available"] = false
		return out
	}
	var e struct {
		Code         json.RawMessage
		Status, Type string
		Details      []map[string]json.RawMessage
	}
	if json.Unmarshal(envelope["error"], &e) != nil {
		out["structured_details_available"] = false
		return out
	}
	out["provider_status"], out["provider_type"] = diagnosticLabel(e.Status), diagnosticLabel(e.Type)
	out["provider_detail_count"] = len(e.Details)
	var code int
	if json.Unmarshal(e.Code, &code) == nil {
		out["provider_code"] = code
	}
	allowedDetails := []map[string]any{}
	for _, d := range e.Details {
		var typ string
		_ = json.Unmarshal(d["@type"], &typ)
		x := map[string]any{}
		switch typ {
		case "type.googleapis.com/google.rpc.RetryInfo":
			var delay string
			_ = json.Unmarshal(d["retryDelay"], &delay)
			if diagnosticDelay.MatchString(delay) {
				x["retry_delay"] = delay
			}
		case "type.googleapis.com/google.rpc.ErrorInfo":
			for _, field := range []string{"reason", "domain"} {
				var value string
				_ = json.Unmarshal(d[field], &value)
				if safe := diagnosticLabel(value); safe != "" {
					x[field] = safe
				}
			}
			var meta map[string]string
			_ = json.Unmarshal(d["metadata"], &meta)
			for _, field := range []string{"quota_metric", "quota_limit", "quota_limit_value", "quota_location", "model", "service"} {
				if safe := diagnosticLabel(meta[field]); safe != "" {
					x[field] = safe
				}
			}
		case "type.googleapis.com/google.rpc.QuotaFailure":
			var violations []map[string]json.RawMessage
			_ = json.Unmarshal(d["violations"], &violations)
			items := []map[string]string{}
			for _, v := range violations {
				item := map[string]string{}
				for _, field := range []string{"quotaMetric", "quotaId", "quotaValue"} {
					var value string
					_ = json.Unmarshal(v[field], &value)
					if safe := diagnosticLabel(value); safe != "" {
						item[field] = safe
					}
				}
				if len(item) > 0 {
					items = append(items, item)
				}
				if len(items) == 8 {
					break
				}
			}
			x["violations"] = items
		default:
			continue
		}
		x["type"] = strings.TrimPrefix(typ, "type.googleapis.com/google.rpc.")
		allowedDetails = append(allowedDetails, x)
		if len(allowedDetails) == 8 {
			break
		}
	}
	out["structured_details_available"] = len(allowedDetails) > 0
	if len(allowedDetails) > 0 {
		out["provider_details"] = allowedDetails
	}
	return out
}

func diagnosticHeaders(h http.Header) map[string]any {
	out := map[string]any{}
	if s := h.Get("Retry-After"); s != "" {
		if n, err := strconv.ParseFloat(s, 64); err == nil && n >= 0 && n <= 604800 {
			out["retry_after_seconds"] = n
		} else if at, err := http.ParseTime(s); err == nil {
			out["retry_after_at"] = at
		}
	}
	for _, name := range []string{"X-Request-Id", "Request-Id", "X-Goog-Request-Id"} {
		if s := h.Get(name); s != "" {
			out[strings.ToLower(name)+"_hash"] = digest(s)
		}
	}
	for _, name := range []string{"Anthropic-Ratelimit-Requests-Reset", "Anthropic-Ratelimit-Tokens-Reset", "X-Ratelimit-Reset-Requests", "X-Ratelimit-Reset-Tokens"} {
		s := h.Get(name)
		if at, err := time.Parse(time.RFC3339Nano, s); err == nil {
			out[strings.ToLower(name)] = at
		} else if duration, err := time.ParseDuration(s); err == nil && duration >= 0 && duration <= 7*24*time.Hour {
			out[strings.ToLower(name)+"_seconds"] = duration.Seconds()
		}
	}
	for _, name := range []string{"X-Ratelimit-Limit-Requests", "X-Ratelimit-Remaining-Requests", "X-Ratelimit-Limit-Tokens", "X-Ratelimit-Remaining-Tokens", "Anthropic-Ratelimit-Requests-Remaining", "Anthropic-Ratelimit-Tokens-Remaining"} {
		if s := h.Get(name); s != "" {
			if n, err := strconv.ParseFloat(s, 64); err == nil && n >= 0 && n <= 1e15 {
				out[strings.ToLower(name)] = n
			}
		}
	}
	return out
}

func recordUsageDiagnostic(u usageEvidence) {
	p, err := readPolicy()
	if err != nil || !p.RecordRouting || u.RequestID == "" {
		return
	}
	origin := ""
	if parsed, err := url.Parse(u.BaseURL); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
		origin = (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
	}
	event := map[string]any{"schema_version": 1, "plugin_version": pluginVersion, "kind": "upstream_usage", "at": time.Now(), "requested_at": u.RequestedAt, "request_hash": digest(u.RequestID), "trace_hash": optionalDigest(u.TraceID), "selection_id": u.SelectionID, "provider": diagnosticLabel(u.Provider), "model": diagnosticLabel(u.Model), "response_model": diagnosticLabel(u.ResponseModel), "auth_id": u.AuthID, "stream": u.Stream, "failed": u.Failed, "latency_ms": u.Latency.Milliseconds(), "ttft_ms": u.TTFT.Milliseconds(), "input_tokens": u.Detail.InputTokens, "output_tokens": u.Detail.OutputTokens, "reasoning_tokens": u.Detail.ReasoningTokens, "cache_read_tokens": max(u.Detail.CacheReadTokens, u.Detail.CachedTokens), "cache_creation_tokens": u.Detail.CacheCreationTokens, "response_hints": diagnosticHeaders(u.ResponseHeaders), "upstream_origin": origin, "executor": diagnosticLabel(u.ExecutorType)}
	if u.Failed {
		event["failure"] = failureDiagnostic(u.Failure.Body, u.Failure.StatusCode)
		state.Lock()
		q := state.Quota[u.AuthID]
		state.Unlock()
		windows := []map[string]any{}
		for _, g := range q.Groups {
			if !relevantGroup(u.Provider, u.Model, g.Name) {
				continue
			}
			for _, b := range g.Buckets {
				if len(windows) == 16 {
					break
				}
				windows = append(windows, map[string]any{"window": diagnosticLabel(b.Window), "remaining": b.Remaining, "reset": b.Reset})
			}
		}
		event["cached_quota_at"], event["cached_quota_windows"] = q.At, windows
	}
	// Every delivered callback is captured, before accounting deduplication.
	// Callback count is not a count of independent downstream requests.
	appendEvidence(diagnosticJournal, event)
}

func recordQuotaDiagnostic(a quotaAccount, response quotaHTTP, err error, started time.Time) {
	p, policyErr := readPolicy()
	if policyErr != nil || !p.RecordRouting {
		return
	}
	failed := err != nil || response.Code < 200 || response.Code >= 300
	event := map[string]any{"schema_version": 1, "plugin_version": pluginVersion, "kind": "quota_read", "at": time.Now(), "provider": diagnosticLabel(a.Provider), "auth_id": a.ID, "status_code": response.Code, "failed": failed, "duration_ms": time.Since(started).Milliseconds(), "response_hints": diagnosticHeaders(response.Header)}
	if failed {
		body := response.Body
		if err != nil && body == "" {
			body = err.Error()
		}
		event["failure"] = failureDiagnostic(body, response.Code)
		if err != nil && response.Code >= 200 && response.Code < 300 {
			event["failure_class"] = "quota_response_parse"
		}
	}
	appendEvidence(diagnosticJournal, event)
}

type evidenceWriteStatus struct {
	Written     uint64    `json:"written"`
	Failures    uint64    `json:"failures"`
	LastSuccess time.Time `json:"last_success,omitempty"`
	LastFailure time.Time `json:"last_failure,omitempty"`
	ErrorClass  string    `json:"error_class,omitempty"`
}

var evidenceHealth = struct {
	sync.Mutex
	Files map[string]evidenceWriteStatus
}{Files: map[string]evidenceWriteStatus{}}

func noteEvidenceWrite(name string, err error) {
	evidenceHealth.Lock()
	defer evidenceHealth.Unlock()
	s := evidenceHealth.Files[name]
	if err == nil {
		s.Written++
		s.LastSuccess = time.Now()
	} else {
		s.Failures++
		s.LastFailure = time.Now()
		s.ErrorClass = "io_error"
		if os.IsPermission(err) {
			s.ErrorClass = "permission_denied"
		}
		if os.IsNotExist(err) {
			s.ErrorClass = "path_missing"
		}
	}
	evidenceHealth.Files[name] = s
}

func loggingDiagnostics(enabled bool) map[string]any {
	evidenceHealth.Lock()
	defer evidenceHealth.Unlock()
	files := map[string]evidenceWriteStatus{}
	healthy := true
	for name, status := range evidenceHealth.Files {
		files[name] = status
		if status.LastFailure.After(status.LastSuccess) {
			healthy = false
		}
	}
	return map[string]any{"enabled": enabled, "healthy": healthy, "files": files, "journal": diagnosticJournal, "max_file_bytes": 8 << 20, "previous_generations": 1, "scope": "since process start; upstream callbacks include retries, not independent client requests", "raw_contents_recorded": false}
}
