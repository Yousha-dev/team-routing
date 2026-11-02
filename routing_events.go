package main

import (
	"time"
)

// Sanitized, bounded attribution evidence; no prompts, credentials or headers.
// Called under traffic's mutex, which also serializes append/rotation.
func writeRoutingEvent(auth, model, outcome string, status int, started, completed time.Time) {
	p, err := readPolicy()
	if err != nil || !p.RecordRouting {
		return
	}
	if completed.IsZero() {
		completed = time.Now()
	}
	event := map[string]any{"at": completed, "auth_id": auth, "model": model, "outcome": outcome, "status_code": status}
	if !started.IsZero() {
		event["duration_ms"] = completed.Sub(started).Milliseconds()
	}
	appendEvidence("team-routing-events.jsonl", event)
}
