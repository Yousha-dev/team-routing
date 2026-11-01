package main

import (
	"encoding/json"
	"os"
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
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	path := statePath("team-routing-events.jsonl")
	if info, err := os.Stat(path); err == nil && info.Size() > 8<<20 {
		_ = os.Remove(path + ".previous")
		_ = os.Rename(path, path+".previous")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(raw, '\n'))
}
