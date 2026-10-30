# Team Routing

A CLIProxyAPI plugin that schedules requests across Claude, Codex and Antigravity subscription accounts using their live quota: it avoids accounts near their five-hour limit, prefers quota that is about to reset, keeps conversations on one account for prompt-cache reuse, and can pin a client key to a specific provider and model. It includes its own quota provider and cache and a dashboard in the management UI.

## Install

Install **Team Routing** from the Plugin Store in the CLIProxyAPI management UI. To install manually, download the zip for your platform from the [latest release](https://github.com/Yousha-dev/team-routing/releases/latest), extract the library into the proxy's plugin directory, and enable it:

```yaml
plugins:
  enabled: true
  configs:
    team-routing:
      enabled: true
      # state_dir: /path/to/writable/folder   # optional
```

Team Routing owns conversation affinity, so turn off the core's own affinity (`routing.session-affinity: false`); otherwise the core can bypass its quota checks.

## Dashboard

Open **Team Routing** under **PLUGINS** in the official management sidebar, or visit <http://127.0.0.1:8317/v0/resource/plugins/team-routing/dashboard>. Enter the proxy management key to connect. The dashboard keeps it only in page memory.

Choose **Override all requests** for a client, select its provider and model, then **Save changes**. Choose **Use requested model** to remove the override. Account preferences, thresholds, quota freshness, live quota readings, and manual quota refresh are available on the same page. Changes are persisted and apply without restarting the proxy. Unsaved browser edits are preserved during automatic status refresh; stale saves are rejected when another editor changed the policy.

The page and its authenticated API are provided by this standalone plugin. The official management HTML and core source stay unmodified. Client keys and upstream tokens are never included in dashboard responses. The save API validates models, keeps a private backup, and atomically replaces the policy.

## Dynamic client keys

The authoritative client list is the official proxy's `access.api-keys`, read through `GET /v8/management/config/access/api-keys`. Create, copy, or revoke keys in **Config Panel → Access & Authentication → Client API Keys**. The dashboard links to that page. It discovers all active keys, including keys created directly in the official UI, and hides revoked keys. Its 15-second status refresh updates the client rows automatically when there are no unsaved edits. A changed key list invalidates stale saves.

Each row shows a masked key and a stable key ID. Names can come from an optional `secrets.json` in the state directory; otherwise newly discovered keys receive a generated label. Editable labels are stored under `client_labels` in the private Team Routing policy. Model overrides and labels use the full SHA-256 key ID, so changing a label or key order cannot redirect a rule to another key. A rule affects any app using that key. The dashboard does not expose full client keys or automatically configure applications to use a newly created key.

The dashboard API's `clients` and `policy.routes` now use stable key IDs; `client_details` supplies display names, masked keys and short fingerprints. Existing on-disk routing rules already used key hashes and require no migration.

## Portable installation

Team Routing 1.0.1 is a standalone Go/cgo plugin using ABI 1 and schema 7. Install the binary for your host OS and architecture through CLIProxyAPI's plugin support. Building from source requires Go plus a C compiler: run `go test ./...`, then `go build -buildmode=c-shared -trimpath -o team-routing.so .` (use `.dll` on Windows or `.dylib` on macOS). Release builds for Windows, Linux and macOS are produced by `.github/workflows/release.yml`. The plugin has been exercised on Windows; reports from other platforms are welcome.

State defaults to the proxy working directory, or set `plugins.configs.team-routing.state_dir` to a writable folder. First startup creates a policy with an 85% soft threshold, no owned emails, and no model overrides. Existing policy files are preserved. The selected state directory holds team-routing.json, team-quota-cache.json, and policy backups. An optional secrets.json can supply local client labels; no on-disk management key is used for authentication.

The dashboard sends its own browser origin with each authenticated request, preserving custom ports and HTTPS. Server-side self-calls use that request's caller context, never shared global credentials. They reject URL user-info and redirects. If the proxy cannot reach its public browser origin, needs a different internal address, or a non-browser client has no origin header, set `plugins.configs.team-routing.management_url` to the correct management API base URL. Non-browser clients can alternatively send `X-Team-Routing-Origin: https://your-proxy:port`. Without an explicit URL or origin, the standard localhost:8317 fallback applies. No assumption is made that the HTTP Host header reaches a plugin. Reverse proxies must expose the standard root API/resource paths; mounting only the dashboard under a path prefix is not supported.

Quota reads use the native host auth/HTTP callbacks and honor the host's outbound transport settings. No quota data or inference is sent to an additional third-party service. Management secrets and upstream tokens are never embedded in source or dashboard responses.

## Models across subscriptions

Without a per-key model override, the official core already looks up every provider advertising the exact requested model ID. Its mixed credential pool is passed to Team Routing's adaptive scheduler, which can select Claude or Antigravity subscriptions and applies each provider's applicable quota checks. One request runs on one account; pooling does not mean sending the same request to every account. An explicit per-key provider override intentionally narrows the pool.

No extra opt-in is required for this existing behavior. A provider must actually advertise the requested model. Similar family names, older versions, and thinking/non-thinking variants are not interchangeable. For example, an Antigravity account advertising claude-sonnet-4-6 can join that exact model's pool, but does not supply claude-sonnet-5-5. Provider-specific prefixes, excluded models and host cooldowns still constrain eligibility. The plugin never fabricates model access, downgrades a request, or uses paid credits to expand eligibility.

## Selection

The host filters disabled accounts, model support and cooldowns first. Adaptive v1 then scores all remaining priority tiers. Claude/Codex still require a quota sample newer than max_quota_age_seconds (currently 20 minutes). Any exhausted included quota window blocks the account. Extra usage/credits never provide eligibility, and Antigravity credit fallback remains disabled.

85% five-hour usage is now a soft avoidance threshold, not a hard exclusion. Above it, avoidance rises smoothly toward exhaustion. A five-hour or weekly reset within 30 minutes reduces this penalty when the sample is recent (poll interval plus 30 seconds). Non-exhausted capacity remains usable outside that grace period when alternatives are worse or unavailable.

For each eligible account the score starts at 1 and adds 0.7 times projected five-hour remaining quota, 0.4 times projected weekly remaining quota, and smooth expiry bonuses (up to 1.5 for five-hour quota and 1 for weekly quota). Bonuses favor quota predicted to remain unused at reset and fade continuously as resets move farther away. The dashboard horizon controls the five-hour bonus time scale; its reference usage controls how that bonus fades with low remaining headroom. The score subtracts weekly runway pressure, short five-hour runway risk, data age, active load and a gradual above-threshold penalty. Your Claude emails receive only a 0.08 preference bonus, removed at 70% usage in any reported weekly window. Better headroom or expiring quota can outweigh it. No ownership bonus applies to other providers.

Consumption rates are quota fractions/hour learned from successive successful samples, using a 50/50 exponential moving average. Resets, rising remaining allowance, and observation gaps over six hours invalidate the rate. Rates persist with the quota cache. Missing rates use headroom only. Projected remaining quota subtracts estimated consumption since the sample and a heuristic reserve for each active request: the greater of 0.5% of that window or one minute of its observed burn. This is an estimate, not exact per-request accounting or a token reservation guarantee. Projected exhaustion also blocks selection.

A current conversation remains bound while eligible unless a better account has both a score advantage above 0.5 and a score more than 35% higher, after at least 10 minutes on the current binding. Bindings expire after one hour of inactivity. This hysteresis trades a substantial quota benefit against cache reuse instead of moving on small score fluctuations. Failure, exhausted quota or stale Claude/Codex readings can move a request immediately through normal eligibility rules. Bindings and request counts are memory-only; rates and quota cooldowns persist.

Request lifecycle hooks track actual active requests by selected account, handle retries without double counting, and release them on terminal callbacks. Orphaned entries expire after two hours. Dashboard/API routing diagnostics report active counts, terminal tracked requests and the adaptive engine version. Hook payloads omit request bodies (schema 7); the plugin does not modify prompts, tools, credentials or model responses.

Quota polling remains conservative at 300 seconds with existing cache/cooldown protections. Sampling, external account use and rounding limit prediction accuracy; this is a transparent heuristic, not a claim of a globally optimal schedule. Provider-side extra billing must also be disabled for a strict spending ceiling. Antigravity applies available matching model-group quota only; other providers without normalized quota rely on core eligibility. There is no automatic model-family switch or paid credit fallback.

The plugin owns affinity; core affinity stays disabled. Explicit Claude Code/Codex session headers are preferred; without session metadata the binding falls back to client key + provider + model. Switching model/provider creates a separate binding. Configuration is re-read for new selections and survives official proxy updates.

## Model overrides

`routes` maps SHA-256 hashes of downstream client keys to `{provider,model}`. Empty means preserve the requested model. Overrides affect every model request under that key, including helper requests. Use separate keys for applications. Start fresh conversations when switching model families; provider-specific tools/context may differ.

Edit overrides in the dashboard, or edit `team-routing.json` directly. The plugin reloads policy on every request without a restart. Disabling Team Routing restores builtin scheduling and removes its per-key overrides. To restore builtin cache affinity as well, turn routing.session-affinity back on.

The C ABI shim is derived from the Claude Remote Login plugin and upstream's MIT example (notice in UPSTREAM-LICENSE). Tests cover owner preference, expiring quota, cache affinity, adaptive scoring, soft 85% avoidance, rate/reset handling, request lifecycle tracking and affinity hysteresis, total exhaustion and stale readings.


## Integrated quota cache

Team Routing 0.3 implements the host QuotaProvider contract. Auth Files refreshes, the dashboard, and background routing checks share the same cache. Only the provider usage/quota endpoint is read; no profile, reset-credit, promotional-grant, or reset/redemption requests are made. Native authenticated management HTTP transport injects account tokens; the monitor never returns tokens to a browser.

The cache is keyed by provider and account identity (including organization when present), so duplicate credential files share reads. Concurrent reads coalesce. Manual refresh respects the normal polling/cache interval and backoff. Successful quota timestamps never advance on cache hits or failed reads. Last good readings remain available with their true age; the scheduler still excludes stale Claude/Codex readings using max_quota_age_seconds.

HTTP 429 starts at 15 minutes of backoff and increases exponentially to 2 hours. A longer Retry-After is honored. Authentication errors cool down for 15 minutes; other failures back off from 30 seconds to 10 minutes. The private team-quota-cache.json in the state directory contains quota readings and cooldowns, so restarting cannot reset a cooldown. It contains no access tokens or client keys. The dashboard reports cache hits, upstream reads and per-account cooldown information. Its 15-second display refresh only reads local state.

The separate CPA Quota plugin is not needed. The original proxy binary, core source and official management HTML are unchanged. Prompt-cache reuse continues to rely on the existing account/conversation affinity; quota caching does not change model responses.
