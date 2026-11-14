package main

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"
)

func TestAccountReserveFloorsUseEachCredentialAndSameSchedule(t *testing.T) {
	p := scheduledPolicy()
	now := localTime("2026-11-02 14:00")
	p.AccountReserves = map[string]accountReserve{"custom": {FiveHourFloor: .30, WeeklyFloor: .20}, "standard": {FiveHourFloor: .20, WeeklyFloor: .15}}
	five := bucket{Window: "five_hour", Reset: localTime("2026-11-02 19:00")}
	week := bucket{Window: "seven_day", Reset: localTime("2026-11-04 16:00")}
	for _, tc := range []struct {
		id         string
		five, week float64
	}{{"custom", .3, .2}, {"standard", .2, .15}, {"inherited", .2, .15}} {
		c := candidate{ID: tc.id, Provider: "claude"}
		if reserveFloor(p, c, five, now) != tc.five || reserveFloor(p, c, week, now) != tc.week {
			t.Fatal("per-account percentages not applied", tc.id)
		}
		if reserveFloor(p, c, week, localTime("2026-11-04 11:00")) != 0 {
			t.Fatal("custom weekly reserve did not unlock on the existing schedule")
		}
	}
	if p.SharedClaude.FiveHourFloor != .2 || p.SharedClaude.WeeklyFloor != .15 {
		t.Fatal("resolver mutated shared defaults")
	}
	if reserveFloor(p, candidate{ID: "custom", Provider: "claude"}, bucket{Window: "five_hour", Reset: localTime("2026-11-03 02:00")}, localTime("2026-11-02 21:00")) != 0 {
		t.Fatal("custom floor leaked into unprotected session")
	}
}

func TestAccountReservesAffectAdmissionAndFutureCapacity(t *testing.T) {
	p, c, q, now := admissionFixture(t)
	p.AccountReserves = map[string]accountReserve{c.ID: {FiveHourFloor: .3, WeeklyFloor: .2}}
	q.Groups[0].Buckets[0].Remaining = .28
	if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatal("default floor bypassed custom five-hour reserve")
	}
	q.Groups[0].Buckets[0].Remaining = .8
	q.Groups[0].Buckets[1].Remaining = .18
	if _, ok := rankAccount(p, c, "opus", q, now, 0); ok {
		t.Fatal("default floor bypassed custom weekly reserve")
	}
	q.Groups[0].Buckets[1].Remaining = .5
	if _, ok := rankAccount(p, c, "opus", q, now, 0); !ok {
		t.Fatal("usable custom account blocked")
	}
	start := localTime("2026-11-02 14:00")
	end := start.Add(5 * time.Hour)
	got := quotaOpportunities(p, c, start, end, .5, end.Add(5*time.Hour))
	if math.Abs(got-1.2) > 1e-8 {
		t.Fatal("future capacity ignored custom floor", got)
	}
}

func TestAccountReservePrecedenceOwnerMasterAndProvider(t *testing.T) {
	p := protectedPolicy()
	c := candidate{ID: "owned", Provider: "claude", Metadata: map[string]any{"email": "mine@example.com"}}
	if sharedSettings(p, c) != nil {
		t.Fatal("owned default exemption changed")
	}
	p.AccountReserves = map[string]accountReserve{c.ID: {FiveHourFloor: .4, WeeklyFloor: .25}}
	if s := sharedSettings(p, c); s == nil || s.FiveHourFloor != .4 || s.WeeklyFloor != .25 {
		t.Fatal("explicit owned-account protection ignored")
	}
	c.Provider = "codex"
	if sharedSettings(p, c) != nil {
		t.Fatal("Claude reserves affected another provider")
	}
	c.Provider = "claude"
	p.SharedClaude.Enabled = false
	if sharedSettings(p, c) != nil {
		t.Fatal("master switch did not disable custom floors")
	}
	p.SharedClaude.Enabled = true
	p.AccountReserves[c.ID] = accountReserve{}
	if s := sharedSettings(p, c); s == nil || s.FiveHourFloor != 0 || s.WeeklyFloor != 0 {
		t.Fatal("explicit zero interpreted as inheritance")
	}
}

func TestAccountReserveHeadersEnforceWholeSessionAfterEight(t *testing.T) {
	p, c, q, _ := admissionFixture(t)
	p.AccountReserves = map[string]accountReserve{c.ID: {FiveHourFloor: .3, WeeklyFloor: .2}}
	now := localTime("2026-11-01 20:30")
	q.At = now
	q.RequestedAt = q.At
	q.Groups[0].Buckets[0].Reset = time.Time{}
	recordAdmissionSignals(c.ID, quotaHeaders(.69, .3, localTime("2026-11-01 23:00"), q.Groups[0].Buckets[1].Reset), now, now)
	if r, ok := rankAccount(p, c, "opus", q, now, 0); ok || r.FiveHourFloor != .3 {
		t.Fatal("custom whole-session floor was bypassed after 8 PM", r)
	}
}

func TestAccountReserveJSONAndBounds(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"five_hour_remaining_floor":0.3}`, `{"five_hour_remaining_floor":null,"weekly_remaining_floor":0.2}`, `{"five_hour_remaining_floor":0.3,"weekly_remaining_floor":0.2,"unknown":1}`} {
		var value accountReserve
		if json.Unmarshal([]byte(raw), &value) == nil {
			t.Fatal("incomplete or unknown account rule accepted", raw)
		}
	}
	var zero accountReserve
	if err := json.Unmarshal([]byte(`{"five_hour_remaining_floor":0,"weekly_remaining_floor":0}`), &zero); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []float64{-.01, .91, math.NaN(), math.Inf(1)} {
		if validateAccountReserves(map[string]accountReserve{"id": {FiveHourFloor: bad, WeeklyFloor: .2}}) == nil {
			t.Fatal("invalid reserve accepted", bad)
		}
	}
	for _, id := range []string{"", " bad", "id\n"} {
		if validateAccountReserves(map[string]accountReserve{id: {}}) == nil {
			t.Fatal("invalid identity accepted")
		}
	}
	schema := accountReserveSchema()["additionalProperties"].(map[string]any)
	if len(schema["required"].([]string)) != 2 {
		t.Fatal("schema does not require complete percentage pair")
	}
	for _, f := range accountReserveFields() {
		if *f.Minimum != 0 || *f.Maximum != maximumReserveFraction {
			t.Fatal("editor bounds differ from execution bounds")
		}
	}
}

func TestAccountReserveSaveInheritanceAndOldClientCompatibility(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	oldKeys, oldHost := fetchActiveClientKeys, hostCall
	fetchActiveClientKeys = func(...managementCaller) ([]string, error) { return []string{"synthetic-client"}, nil }
	hostCall = func(method string, _ any) (json.RawMessage, error) {
		if method == "host.auth.list" {
			return []byte(`{"files":[{"id":"shared","provider":"claude"},{"id":"other","provider":"codex"}]}`), nil
		}
		return nil, fmt.Errorf("unexpected host call")
	}
	t.Cleanup(func() { fetchActiveClientKeys = oldKeys; hostCall = oldHost })
	p := scheduledPolicy()
	writePolicyFixture(t, p)
	clients := localSecrets{Clients: []struct{ Name, Key string }{{"test", "synthetic-client"}}}
	input := publicPolicy(p, clients)
	input.AccountReserves = map[string]accountReserve{"shared": {FiveHourFloor: .3, WeeklyFloor: .2}}
	saved, err := saveDashboard(input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readPolicy()
	if err != nil || got.AccountReserves["shared"].FiveHourFloor != .3 {
		t.Fatal("custom policy did not persist", err)
	}
	// A 1.5 client does not know the new field and must not erase it.
	m := jsonObject(saved)
	delete(m, "account_reserves")
	raw, _ := json.Marshal(m)
	var oldClient dashboardPolicy
	if err := json.Unmarshal(raw, &oldClient); err != nil {
		t.Fatal(err)
	}
	saved, err = saveDashboard(oldClient)
	if err != nil || saved.AccountReserves["shared"].WeeklyFloor != .2 {
		t.Fatal("old client erased new rules", err)
	}
	// Explicit {} removes custom rules and restores inheritance.
	saved.AccountReserves = map[string]accountReserve{}
	saved.EditedFields = map[string]bool{"account_reserves": true}
	if _, err = saveDashboard(saved); err != nil {
		t.Fatal(err)
	}
	got, _ = readPolicy()
	if len(got.AccountReserves) != 0 {
		t.Fatal("remove did not restore default inheritance")
	}
	for _, id := range []string{"typo"} {
		input = publicPolicy(got, clients)
		input.AccountReserves = map[string]accountReserve{id: {FiveHourFloor: .3, WeeklyFloor: .2}}
		if _, err := saveDashboard(input); err == nil {
			t.Fatal("new rule accepted disconnected or non-Claude credential")
		}
	}
	if err := validateNewAccountReserves(map[string]accountReserve{"disconnected": {}}, map[string]accountReserve{"disconnected": {}}); err != nil {
		t.Fatal("existing disconnected rule could not be retained", err)
	}
}
