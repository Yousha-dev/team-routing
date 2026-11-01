package main

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
)

// These floors are eligibility constraints, never score bonuses that affinity
// or a near-reset exception can override. They apply only to non-owned Claude.
type sharedClaudePolicy struct {
	Enabled            bool    `json:"enabled"`
	WeeklyFloor        float64 `json:"weekly_remaining_floor"`
	WeeklyReleaseHours int     `json:"weekly_release_within_hours"`
	WeeklyReleaseMode  string  `json:"weekly_release_mode,omitempty"`
	FiveHourFloor      float64 `json:"five_hour_remaining_floor"`
	Weekdays           []int   `json:"protected_weekdays"`
	StartHour          int     `json:"protected_start_hour"`
	EndHour            int     `json:"protected_end_hour"`
	Timezone           string  `json:"timezone"`
}

func defaultSharedPolicy() sharedClaudePolicy {
	return sharedClaudePolicy{WeeklyFloor: .15, WeeklyReleaseHours: 24, FiveHourFloor: .20, Weekdays: []int{0, 1, 2, 3, 4}, StartHour: 11, EndHour: 20}
}
func validateSharedPolicy(s *sharedClaudePolicy) error {
	if s == nil {
		return nil
	}
	if s.WeeklyReleaseMode != "" && s.WeeklyReleaseMode != "hours" && s.WeeklyReleaseMode != "reset_schedule" {
		return fmt.Errorf("invalid weekly reserve release mode")
	}
	if s.WeeklyFloor < 0 || s.WeeklyFloor > .9 || s.FiveHourFloor < 0 || s.FiveHourFloor > .9 || s.WeeklyReleaseHours < 1 || s.WeeklyReleaseHours > 72 || s.StartHour < 0 || s.EndHour > 24 || s.EndHour <= s.StartHour || len(s.Weekdays) == 0 {
		return fmt.Errorf("invalid shared Claude reserves or protected hours")
	}
	seen := map[int]bool{}
	for _, day := range s.Weekdays {
		if day < 0 || day > 6 || seen[day] {
			return fmt.Errorf("invalid or duplicate protected weekday")
		}
		seen[day] = true
	}
	if s.Timezone != "" {
		if _, err := time.LoadLocation(s.Timezone); err != nil {
			return fmt.Errorf("unknown shared reserve timezone")
		}
	}
	return nil
}
func ownedClaude(p policy, c candidate) bool {
	if c.Provider != "claude" {
		return false
	}
	email, _ := c.Metadata["email"].(string)
	if strings.TrimSpace(email) == "" {
		email = c.Attributes["email"]
	}
	for _, value := range p.Owned {
		value = strings.TrimSpace(value)
		if value != "" && strings.EqualFold(strings.TrimSpace(email), value) {
			return true
		}
	}
	return false
}
func sharedSettings(p policy, c candidate) *sharedClaudePolicy {
	if c.Provider != "claude" || ownedClaude(p, c) || p.SharedClaude == nil || !p.SharedClaude.Enabled {
		return nil
	}
	return p.SharedClaude
}

func reserveLocation(s *sharedClaudePolicy) *time.Location {
	loc := time.Local
	if s.Timezone != "" {
		if selected, err := time.LoadLocation(s.Timezone); err == nil {
			loc = selected
		}
	}
	return loc
}

// Use the same release instant for hard eligibility and deadline planning.
// Unset mode preserves the legacy rolling-hour rule for portable installs.
func weeklyReserveReleaseAt(s *sharedClaudePolicy, reset time.Time) time.Time {
	if reset.IsZero() {
		return time.Time{}
	}
	if s.WeeklyReleaseMode != "reset_schedule" {
		return reset.Add(-time.Duration(s.WeeklyReleaseHours) * time.Hour)
	}
	local := reset.In(reserveLocation(s))
	protectedDay := false
	for _, day := range s.Weekdays {
		if int(local.Weekday()) == day {
			protectedDay = true
			break
		}
	}
	// Match wall-clock minutes: 20:00 belongs to daytime; 20:01 to night.
	minute := local.Hour()*60 + local.Minute()
	endMidnight := false
	if s.EndHour == 24 && minute == 0 {
		for _, day := range s.Weekdays {
			endMidnight = endMidnight || int(local.AddDate(0, 0, -1).Weekday()) == day
		}
	}
	if endMidnight || protectedDay && minute >= s.StartHour*60 && minute <= s.EndHour*60 {
		return reset.Add(-time.Duration(s.WeeklyReleaseHours) * time.Hour)
	}
	// Last protected-hours end strictly before the reset: same evening for
	// night resets, previous evening for morning and unprotected daytime resets.
	evening := time.Date(local.Year(), local.Month(), local.Day(), s.EndHour, 0, 0, 0, local.Location())
	for !evening.Before(reset) {
		evening = evening.AddDate(0, 0, -1)
	}
	boundary := time.Date(local.Year(), local.Month(), local.Day(), s.EndHour%24, 0, 0, 0, local.Location())
	if minute == (s.EndHour%24)*60 && evening.Equal(boundary) {
		evening = evening.AddDate(0, 0, -1)
	}
	return evening
}

func protectedFiveHourWindow(s *sharedClaudePolicy, end, now time.Time) bool {
	loc := reserveLocation(s)
	// With no active provider window, the next request starts a prospective
	// five-hour session. Never assume a missing reset means no protection.
	start := end.Add(-5 * time.Hour)
	if end.IsZero() || !end.After(now) {
		start = now
		end = now.Add(5 * time.Hour)
	}
	localStart := start.In(loc)
	day := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, loc)
	day = day.AddDate(0, 0, -1) // End hour 24 includes the preceding day's final minute.
	for !day.After(end.In(loc)) {
		selected := false
		for _, n := range s.Weekdays {
			if int(day.Weekday()) == n {
				selected = true
				break
			}
		}
		if selected {
			from := time.Date(day.Year(), day.Month(), day.Day(), s.StartHour, 0, 0, 0, loc)
			until := time.Date(day.Year(), day.Month(), day.Day(), s.EndHour, 1, 0, 0, loc)
			if !end.Before(from) && start.Before(until) {
				return true
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return false
}

func reserveFloor(p policy, c candidate, b bucket, now time.Time) float64 {
	s := sharedSettings(p, c)
	if s == nil {
		return 0
	}
	if weeklyWindow(b.Window) {
		// Preserve the weekly tail until its configured release instant.
		// A passed/unknown timestamp cannot reopen capacity on stale evidence.
		if !b.Reset.After(now) || now.Before(weeklyReserveReleaseAt(s, b.Reset)) {
			return s.WeeklyFloor
		}
	}
	if b.Window == "five_hour" && protectedFiveHourWindow(s, b.Reset, now) {
		return s.FiveHourFloor
	}
	return 0
}

func accountEligibility(p policy, c candidate, q snapshot, now time.Time, active int) string {
	if unavailable, _ := c.Metadata["native_unavailable"].(bool); unavailable {
		if retry, ok := c.Metadata["native_retry_after"].(time.Time); ok && retry.After(now) {
			return "Blocked: Provider cooldown until " + retry.Format(time.RFC3339)
		}
	}
	if c.Provider != "claude" && c.Provider != "codex" {
		return "Provider/model eligibility is checked for each request"
	}
	if rank, ok := rankAccount(p, c, "", q, now, active); !ok {
		return "Blocked: " + rank.Reason
	}
	if sharedSettings(p, c) != nil {
		return "Eligible above shared-account reserves"
	}
	return "Eligible"
}
