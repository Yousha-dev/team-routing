package main

// Versioned, read-only engine parameters. Both execution and the dashboard's
// rule document use these definitions; they are not editable UI settings.
const (
	fiveRequestFraction       = .005
	weeklyRequestFraction     = .001
	fiveProtectedBuffer       = .02
	weeklyProtectedBuffer     = .005
	headerRoundingFraction    = .01
	settlementLagSeconds      = 15
	quotaAgeGraceSeconds      = 60
	quotaResetJitterSeconds   = 90
	warmCacheSeconds          = 300
	ledgerRetentionSeconds    = 7200
	ordinarySwitchFraction    = .20
	warmSwitchFraction        = .35
	ordinarySwitchAbsolute    = .02
	warmSwitchAbsolute        = .04
	expiryWindowSeconds       = 3600
	expiryMinimumWaste        = .05
	weeklyRescueHours         = 24
	weeklyRescuePressureRatio = 1.5
	weeklyRescuePressureDelta = .25
)

func engineProfile() map[string]any {
	return map[string]any{
		"parallel_work_policy":       "Prefer the least-active eligible accounts, then reset urgency; affinity yields when its account is busier. No hard concurrency limit or queue.",
		"five_hour_request_fraction": fiveRequestFraction, "weekly_request_fraction": weeklyRequestFraction,
		"protected_five_hour_buffer": fiveProtectedBuffer, "protected_weekly_buffer": weeklyProtectedBuffer,
		"header_rounding_fraction": headerRoundingFraction, "settlement_lag_seconds": settlementLagSeconds,
		"quota_freshness_grace_seconds": quotaAgeGraceSeconds, "reset_jitter_seconds": quotaResetJitterSeconds,
		"warm_cache_seconds": warmCacheSeconds, "ledger_retention_seconds": ledgerRetentionSeconds,
		"ordinary_switch_fraction": ordinarySwitchFraction, "warm_switch_fraction": warmSwitchFraction,
		"ordinary_switch_absolute": ordinarySwitchAbsolute, "warm_switch_absolute": warmSwitchAbsolute,
		"expiry_window_seconds": expiryWindowSeconds, "expiry_minimum_waste": expiryMinimumWaste,
		"weekly_rescue_hours": weeklyRescueHours, "weekly_rescue_pressure_ratio": weeklyRescuePressureRatio, "weekly_rescue_pressure_delta": weeklyRescuePressureDelta,
	}
}
