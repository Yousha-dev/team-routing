package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Account IDs come from the native credential registry and remain stable across
// token refreshes. Missing entries inherit shared defaults (or owned exemption).
type accountReserve struct {
	FiveHourFloor float64 `json:"five_hour_remaining_floor"`
	WeeklyFloor   float64 `json:"weekly_remaining_floor"`
}

func (r *accountReserve) UnmarshalJSON(raw []byte) error {
	var value struct {
		Five *float64 `json:"five_hour_remaining_floor"`
		Week *float64 `json:"weekly_remaining_floor"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if value.Five == nil || value.Week == nil {
		return fmt.Errorf("account reserves require both five-hour and weekly values; remove the entry to inherit defaults")
	}
	r.FiveHourFloor, r.WeeklyFloor = *value.Five, *value.Week
	return nil
}

// Derive per-account controls and bounds from the same reserve field registry
// used by the defaults. Neither the browser nor account rules invent bounds.
func accountReserveFields() []policyField {
	fields := []policyField{}
	for _, f := range policyFields() {
		if f.Path == "shared_claude.five_hour_remaining_floor" || f.Path == "shared_claude.weekly_remaining_floor" {
			f.Path = strings.TrimPrefix(f.Path, "shared_claude.")
			f.Title = strings.ReplaceAll(strings.TrimPrefix(f.Title, "Default "), "weekly reserve", "Weekly reserve")
			f.Title = strings.ReplaceAll(f.Title, "five-hour reserve", "Five-hour reserve")
			f.Description = strings.ReplaceAll(f.Description, " when an account has no custom reserve", "")
			fields = append(fields, f)
		}
	}
	return fields
}

func accountReserveSchema() map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, f := range accountReserveFields() {
		props[f.Path] = map[string]any{"type": "number", "minimum": *f.Minimum, "maximum": *f.Maximum, "title": f.Title, "description": f.Description}
		required = append(required, f.Path)
	}
	return map[string]any{"type": "object", "maxProperties": 200, "propertyNames": map[string]any{"minLength": 1, "maxLength": 512}, "additionalProperties": map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props}}
}

func validateAccountReserves(values map[string]accountReserve) error {
	if len(values) > 200 {
		return fmt.Errorf("too many account reserve rules")
	}
	for id, r := range values {
		if id == "" || len(id) > 512 || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\r\n\t") {
			return fmt.Errorf("invalid account reserve identity")
		}
		for _, n := range []float64{r.FiveHourFloor, r.WeeklyFloor} {
			if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > maximumReserveFraction {
				return fmt.Errorf("account reserves must be between 0 and %.0f%%", maximumReserveFraction*100)
			}
		}
	}
	return nil
}

func cloneAccountReserves(values map[string]accountReserve) map[string]accountReserve {
	copy := map[string]accountReserve{}
	for id, r := range values {
		copy[id] = r
	}
	return copy
}

func effectiveAccountReserve(p policy, c candidate) map[string]any {
	source := "shared"
	if _, ok := p.AccountReserves[c.ID]; ok {
		source = "account"
	} else if ownedAccount(p, c) {
		source = "owned_exempt"
	}
	s := sharedSettings(p, c)
	if s == nil {
		if base := providerSettings(p, c.Provider); base == nil || !base.Enabled {
			source = "disabled"
		}
		return map[string]any{"source": source, "enabled": false, "five_hour_remaining_floor": 0, "weekly_remaining_floor": 0}
	}
	return map[string]any{"source": source, "enabled": true, "five_hour_remaining_floor": s.FiveHourFloor, "weekly_remaining_floor": s.WeeklyFloor}
}

// Prevent newly introduced web rules targeting a typo or a non-Claude account.
// Existing disconnected rules remain editable/removable and inert until that
// same credential ID returns; a new credential never inherits them by label.
func validateNewAccountReserves(next, current map[string]accountReserve) error {
	newIDs := map[string]bool{}
	for id := range next {
		if _, ok := current[id]; !ok {
			newIDs[id] = true
		}
	}
	if len(newIDs) == 0 {
		return nil
	}
	files, err := hostAuthList()
	if err != nil {
		return fmt.Errorf("unable to verify new account rules; reload after credentials finish loading")
	}
	for _, f := range files {
		delete(newIDs, f.ID)
	}
	if len(newIDs) > 0 {
		if entries, err := fetchHostModelInventory(); err == nil {
			for _, entry := range entries {
				delete(newIDs, entry.ID)
			}
		}
	}
	if len(newIDs) > 0 {
		return fmt.Errorf("account rules must target connected credentials; reload the account list")
	}
	return nil
}
