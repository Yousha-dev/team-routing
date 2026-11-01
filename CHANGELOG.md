# Changelog

## 1.5.0

- Rebuild the dashboard into account overview, client routing, policy and diagnostics sections, with separate embedded HTML, CSS and JavaScript.
- Generate dashboard settings and JSON Schema from a shared backend field registry. Publish evaluated rule precedence and versioned engine parameters.
- Discover active client keys and actual provider/model pairs dynamically, including custom providers. Validate overrides, preserve unsaved edits and reject stale saves.
- Add model-specific, request-free availability diagnostics and native cooldown/model-support status.
- Schedule included quota against weekly resets, remaining five-hour opportunities and observed demand, with measured prompt-cache affinity and exact Claude ownership matching.
- Protect shared weekly/five-hour reserves using configurable time zones, weekdays and reset-based release schedules.
- Persist active/completed admission estimates and numeric response-quota constraints; reconcile fresh readings, deduplicate lifecycle/usage callbacks and preserve safety across restarts.
- Scope family-specific limits correctly, recover old paid-use fences after confirmed renewed included quota and retain strict/best-effort billing controls.
- Preserve generic public capacity errors while retaining sanitized private diagnostic evidence.

Validation: 87 Go/ABI regression tests, all editable dashboard controls, isolated official-host protocol/streaming/admission tests and a live Claude Messages smoke test. Windows runtime verified; other platforms require runtime testing after their release builds.
