# Team Routing

A standalone CLIProxyAPI plugin for reset-aware subscription routing, shared-account reserves, cache affinity and per-client model overrides. The official proxy and management UI stay unmodified.

## Installation

Install through the proxy Plugin Store, or place the library for your operating system in its plugin directory and enable it:

```yaml
plugins:
  enabled: true
  configs:
    team-routing:
      enabled: true
      # state_dir: /path/to/writable/folder
      # management_url: http://127.0.0.1:8317
```

Team Routing 1.6.0 uses ABI 1 / schema 7. State defaults to the proxy working directory. First startup creates a policy; updates preserve existing settings. Disable core `routing.session-affinity` so selections reach the plugin's eligibility checks. Plugin and official proxy updates are independent, subject to ABI compatibility.

Build with Go and a C compiler: `go test -tags abiprobe ./...`, then `go build -buildmode=c-shared -trimpath -o team-routing.so .` (use `.dll` on Windows or `.dylib` on macOS). Windows is exercised locally; other platforms need their own runtime verification.

## Dashboard

Open **Team Routing** under **PLUGINS**, or `/v0/resource/plugins/team-routing/dashboard`. Enter your management key; it stays in page memory.

- **Overview:** global account availability, included allowance, reset times, enabled model support, refresh state and reserve-release details.
- **Client routing:** discovered keys, labels, and requested-model or explicit provider/model overrides. Providers and models come from the runtime registry, including custom providers. Search filters a native selection list.
- **Policy:** fields generated from the backend schema, advanced settings and evaluated rule precedence from the saved policy.
- **Diagnostics:** request-free model/key availability evaluation, measured cache tokens, recent selections and read-only engine parameters.

Save applies to new requests immediately. Unsaved edits survive status refresh. Stale revisions are rejected; reload before retrying. Refresh due quota respects provider backoff. Model-specific eligibility can differ from the overview's global status; use Diagnostics for a particular model. Start a fresh conversation when switching model families.

## Client keys and overrides

The official `access.api-keys` configuration is authoritative. Create, copy or revoke keys in **Config Panel > Access & Authentication > Client API Keys**. Team Routing discovers them dynamically; it does not return full keys or upstream tokens. Optional `secrets.json` supplies local labels but is not required.

Routes and labels use SHA-256 key IDs. Renaming a label cannot move a route to another key. Choose **Override model**, select an advertised provider/model pair and save. **Use requested model** removes the override. Revoked-key preferences remain inert on disk and do not authorize access. A rule affects all requests under that key, including helper calls; use separate keys for applications.

Without an override, the official host considers providers advertising the exact model ID. Different model generations are not equivalent aliases. Provider-specific tool, context, thread and streaming compatibility still matters. The plugin does not manufacture model support.

## Routing policy

Read [ROUTING-POLICY.md](ROUTING-POLICY.md) for formulas, schedule boundaries, credit safety and limitations. In order:

1. Native availability, exact model support, recent complete quota and billing safeguards determine eligibility.
2. Shared Claude weekly/five-hour reserves constrain admission. Owned Claude accounts are exempt from those reserves only.
3. Ranking compares usable weekly allowance against remaining five-hour opportunities and observed demand. Imminent predicted five-hour waste can take precedence.
4. Warm conversations retain their account when urgency is similar. Material expiry urgency can move the next request; active requests continue. Ownership only breaks equal scores.

There is no 85% avoidance, 70% ownership switch or ten-minute affinity lock. Prompt order, cache-control directives and provider cache keys are preserved. Native usage callbacks measure cache reads/writes; token shares are not subscription billing weights. Missing measurements are not cache misses.

Strict billing mode (new-install default) requires provider credits explicitly disabled. Best-effort mode permits enabled/unknown provider credit settings while included quota is usable, rejects explicit paid requests and fences observed paid use. It cannot guarantee zero spillover charges while provider credits are enabled.

## Policy API

All routes require the management key and use base `/v0/management/plugins/team-routing`.

| Route | Purpose |
| --- | --- |
| `GET /schema` | JSON Schema draft 2020-12, editable field metadata/groups, read-only engine parameters |
| `GET /state` | Saved policy/revision, evaluated rules, active keys, runtime catalog and diagnostics |
| `PUT /policy` | Validate and atomically save using the current revision, with a private backup |
| `POST /evaluate` | Explain cached eligibility for `model`, optional `client_id` and `provider`; no inference |
| `POST /refresh` | Check due readings without bypassing cache or provider backoff |

Runtime validation and UI bounds share one backend field registry. Semantic checks cover time zones, weekday uniqueness, hour ordering and freshness relationships. Unknown settings and invalid provider/model pairs are rejected. The plugin reloads saved policy on every request; the browser defines no routing thresholds.

## Reliability and quota cache

Auth Files, the dashboard and background monitor share one persisted quota cache. Only quota endpoints are read through native authenticated HTTP callbacks; no promotional or reset-credit requests are sent. Concurrent reads coalesce. Cache hits and failed refreshes do not advance successful sample timestamps. Provider 429 backs off from 15 minutes up to two hours; longer Retry-After is honored. Freshness safeguards remain enforced.

Claude admission retains completed-request reservations until a fresh reading covers them and constrains headroom with numeric provider response quota headers. Both survive restarts. Variable request cost, delayed counters, already-running requests and external usage prevent exact provider-side reserve guarantees.

`claude_stateless_threads` negotiates Message Threads beta off before inference. Clients must support unsupported-beta fallback and retry with full history. The plugin never guesses missing context or removes continuation fields. Normal provider prompt caching remains available.

Public capacity errors are generic HTTP 429 with Retry-After. Private bounded journals contain sanitized selection reasons, cache/token usage and host outcomes, without raw prompts, credentials or headers. A host cancellation does not independently prove provider failure. Corrupt/unwritable admission or billing state fails closed.

## Portable operation

Self-calls reuse the authenticated dashboard caller and its origin. If the origin cannot be reached internally, set `management_url` to the proxy's internal base URL. Non-browser clients may send `X-Team-Routing-Origin`; otherwise localhost:8317 is the fallback. Redirects and URL user-info are rejected. Reverse proxies must expose the standard root API/resource paths.

Preserve `team-routing.json`, `team-quota-cache.json`, `team-routing-admission.json`, paid-use fences and policy backups across upgrades. No management key is required on disk. The ABI shim derives from upstream's MIT example and Claude Remote Login; see UPSTREAM-LICENSE.


## Per-account reserves

Under **Policy > Account reserves**, each discovered Claude credential can inherit defaults or use custom five-hour and weekly percentages. Custom rules use the native credential ID, survive token refreshes and remain stored while a credential is disconnected. Reconnecting under a different ID uses defaults until configured again. Labels and matching email text cannot move a rule to a different credential.

Custom percentages replace the default percentages; they use the existing protected weekdays/hours, whole-session protection, weekly-release schedule and request safety buffers. Defaults still apply to other shared accounts. Owned accounts stay exempt unless explicitly assigned a custom rule. The master reserve-protection switch applies to both defaults and custom rules.

The policy field is `account_reserves`, mapping credential IDs to objects containing both `five_hour_remaining_floor` and `weekly_remaining_floor` (fractions from 0 to 0.9). Zero is an explicit value. Removing an entry restores inheritance. The schema endpoint supplies the editor metadata and bounds; state/evaluation responses include each account's effective reserve source and percentages. Omitted fields in older dashboard clients preserve existing rules; `{}` explicitly clears them. Inactive rules remain editable/removable, while new rules must target a connected Claude credential.
