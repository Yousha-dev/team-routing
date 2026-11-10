# Native features and plugin responsibilities

Comparison against official CLIProxyAPI 8.0.51 and management UI 1.27.2. Similar names do not establish equivalent behavior. Team Routing delegates the protocol and credential machinery to the host, retaining only behavior its quota-aware policy needs.

| Feature | Native behavior | Plugin decision |
| --- | --- | --- |
| Session identity | Canonical extraction for protocol headers, body IDs, subagents and inferred sessions before scheduling | Use native `canonical_session_id`; remove the duplicate parser and private affinity-header transport in 1.7.1. Retain caller/provider/model isolation. |
| Sticky account selection | Native session affinity holds a credential until unavailable; optional child inheritance and failure rebinding | Keep quota-aware retention: native binding cannot express release deadlines, reserve eligibility, measured cache warmth or a materially more urgent budget. Native affinity lookup reports unsupported when a plugin scheduler is installed. |
| Credential availability | Exact model support, disabled credentials, native cooldowns and provider executors | Already delegated. Plugin diagnostics read native state; the plugin does not create another native cooldown/retry engine. |
| Tiers and weights | Static priorities, round-robin, weighted round-robin and fill-first | Keep deadline ranking: static weights do not express remaining usable quota versus reset opportunities or protected reserves. The plugin explicitly opts into all native priority tiers. Its optional account priority applies after eligibility and before deadline ranking; native credential priority is not stacked on top. All unset plugin priorities are zero. |
| Parallel load | Native strategies distribute ordinary selections without this policy's quota/deadline assessment | Prefer least-active eligible accounts before deadline ranking; avoid concentrating subagent bursts on one score. Host cooldowns and retry limits still apply. |
| Retry/failover | Host executes credential retries and retry rounds | Already delegated; no plugin retry loop. Host limits can still cap how many credentials are attempted. |
| Prompt/signature/protocol caches | Host preserves/translates provider cache directives, reasoning/signature state, tool IDs and provider cache keys | Already delegated; no plugin prompt, response, signature or token replay cache. Plugin cache metrics consume host usage callbacks. |
| Remaining-quota data | Passive response-header observations; management quota fetch dispatches to a plugin quota provider or declarative probe | Keep shared polling/cache/backoff and normalization: the host does not expose an equivalent durable, coalesced subscription quota cache with this policy's freshness and billing fields. Routing the plugin's quota calls through the same management endpoint would recurse into itself. |
| Admission and reserves | Native exhaustion/cooldown state prevents provider failures from selecting unavailable credentials | Keep per-account/native-window floors, whole-session protection, weekly unlock schedules, in-flight/completed reservations and monotone numeric feedback. Passive header snapshots do not enforce these reserves. |
| Credit safeguards | Provider settings and credit-related response observations; some provider-specific credit fallback support | Keep explicit paid-request rejection, strict/best-effort policy and durable observed-paid-use fences. Host observation alone does not establish included-only admission or zero credit spillover. |
| Model aliases/routes | Host provider model catalogs, aliases, protocol conversion and plugin routing hook | Keep optional per-client exact source rules, pass-through exceptions and defaults; provider aliases are not equivalent per-client policies. Use the native registry and conversion. |
| Message Threads | Native protocol traffic forwards available upstream features | Keep optional unsupported-beta negotiation for deployments that change accounts; stateless retry avoids losing account-local continuation context. It does not fabricate context or disable prompt caching. |
| Usage and outcomes | Host publishes parsed usage and lifecycle callbacks | Already delegated parsing; keep bounded account/selection correlation and measured cache evidence needed by ranking/admission. A host terminal outcome is distinct from a provider usage record. |

## Runtime contract

Official `session.Enrich` supplies canonical session metadata before scheduler selection, including when native sticky account selection is disabled. The plugin hashes the host identity; it does not rewrite upstream session IDs or request bodies. Native handling covers more identity forms than the removed parser. Hosts omitting canonical metadata receive a caller/provider/model fallback rather than guessed identity. Version 1.7.1 is verified on official 8.0.51; other embedding hosts must preserve the metadata contract.

The native Routing page previews its own priorities and strategy. Its percentages do not include Team Routing's quota scores, reserves or affinity. Native account disabling, model support and cooldowns still filter the plugin's pool. A higher-priority scheduler plugin can replace Team Routing's scheduling capability. Keep only the intended scheduler authoritative.

Native 8.0.51 `schedulerAuthCandidates` publishes ID, provider, priority, status and safe attributes, but omits mutable auth metadata. Management inventory can still expose email, so legacy email-based ownership can disagree between the dashboard and scheduler. Prefer the plugin's existing explicit `account_ownership` map, configured through the per-account Owned selector. No email or credential data needs to be added to scheduler headers or public source.

The same host's scheduler-rejection wrapper creates an auth error without an HTTP status. A plugin `rate_limit_error` rejection may therefore surface as HTTP 500 instead of 429. This is distinct from an upstream provider failure. The plugin's earlier availability interceptor supplies HTTP 429 for exhausted/protected pools it can establish before selection; later scheduler rejection can still follow the native wrapper path. Ownership configuration does not change that native error contract.

No native feature is disabled or removed by this cleanup. Existing policy settings remain compatible. The plugin's conversation bindings are in memory and naturally warm again after its upgrade.

## Source references

- [Native session enrichment and extraction](https://github.com/router-for-me/CLIProxyAPI/tree/v8.0.51/sdk/cliproxy/session)
- [Scheduling and native fallback ordering](https://github.com/router-for-me/CLIProxyAPI/blob/v8.0.51/sdk/cliproxy/auth/conductor_selection.go)
- [Execution and retry ownership](https://github.com/router-for-me/CLIProxyAPI/blob/v8.0.51/sdk/cliproxy/auth/conductor_execution.go)
- [Passive quota observations](https://github.com/router-for-me/CLIProxyAPI/blob/v8.0.51/sdk/cliproxy/auth/quota_signals.go)
- [Management quota-provider dispatch](https://github.com/router-for-me/CLIProxyAPI/blob/v8.0.51/internal/api/handlers/management/plugin_quota.go)
- [Native Routing preview](https://github.com/router-for-me/Cli-Proxy-API-Management-Center/blob/v1.27.2/src/features/routing/logic.ts)
