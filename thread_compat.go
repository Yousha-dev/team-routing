package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Native Claude Code retries an unsupported beta statelessly and latches that
// beta off for the session. Negotiate this before inference, instead of letting
// account-local server threads break when the scheduler changes subscriptions.
// Never strip thread continuation fields: they may be the only copy of context.
func interceptBefore(raw []byte) (any, error) {
	var req availabilityRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	p, err := readClientPolicy()
	if err != nil {
		return nil, err
	}
	if paidRequest(req.Body) {
		return genericCreditRejection(), nil
	}
	if p.StatelessThreads {
		for _, value := range req.Headers.Values("Anthropic-Beta") {
			for _, beta := range strings.Split(value, ",") {
				if strings.HasPrefix(strings.TrimSpace(beta), "message-threads-") {
					body, _ := json.Marshal(map[string]any{"type": "error", "error": map[string]string{"type": "invalid_request_error", "message": "Unsupported beta: " + strings.TrimSpace(beta)}})
					return map[string]any{"Terminate": true, "StatusCode": 400, "ResponseHeaders": http.Header{"Content-Type": {"application/json"}}, "ResponseBody": body}, nil
				}
			}
		}
	}
	response := availabilityPreflight(p, req, time.Now())
	if response["Terminate"] != true {
		response["Headers"] = http.Header{affinityHeader: {preparedSession(req)}}
	}
	return response, nil
}
