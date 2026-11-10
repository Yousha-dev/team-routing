package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func priorityPolicy(t *testing.T, p policy, values map[string]int) policy {
	t.Helper()
	data := jsonObject(p)
	if values != nil {
		data["account_priority"] = values
	}
	return policyFromObject(t, data)
}

func priorityFixture(t *testing.T) (policy, []candidate, time.Time) {
	t.Helper()
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	now := localTime("2026-11-02 14:00")
	p := protectedPolicy()
	p.Owned = nil
	resetTraffic := func() {
		traffic.Lock()
		traffic.Active = map[string]activeRequest{}
		traffic.Pending = map[string]activeRequest{}
		traffic.Unlock()
	}
	resetTraffic()
	t.Cleanup(resetTraffic)
	state.Pins = map[string]binding{}
	state.Quota = map[string]snapshot{"a": sample(now, .9, .9, time.Hour), "b": sample(now, .9, .9, time.Hour)}
	return p, []candidate{{ID: "a", Provider: "claude"}, {ID: "b", Provider: "claude"}}, now
}

func TestPriorityAppliesAfterEligibilityBeforeAffinityAndSpreading(t *testing.T) {
	p, candidates, now := priorityFixture(t)
	// Establish a warm binding, then change preference without restarting.
	if got := choose(p, "claude", "opus", nil, nil, candidates[1:], now)["AuthID"]; got != "b" {
		t.Fatal(got)
	}
	p = priorityPolicy(t, p, map[string]int{"a": 100})
	if got := choose(p, "claude", "opus", nil, nil, candidates, now)["AuthID"]; got != "a" {
		t.Fatal("affinity overrode priority", got)
	}
	traffic.Lock()
	traffic.Active["busy"] = activeRequest{Auth: "a", At: now}
	traffic.Unlock()
	if got := choose(p, "claude", "opus", nil, nil, candidates, now)["AuthID"]; got != "a" {
		t.Fatal("spreading crossed priority tier", got)
	}
	state.Quota["a"] = sample(now, .9, .01, time.Hour)
	if got := choose(p, "claude", "opus", nil, nil, candidates, now)["AuthID"]; got != "b" {
		t.Fatal("priority bypassed reserve", got)
	}
	state.Quota["b"] = sample(now, 0, 0, time.Hour)
	if got := choose(p, "claude", "opus", nil, nil, candidates, now); got["Reject"] != true {
		t.Fatal("all-held pool served", got)
	}
}

func TestPriorityCannotBypassEligibility(t *testing.T) {
	for _, reason := range []string{"five-hour reserve", "weekly reserve", "stale", "billing enabled", "billing unknown", "exhausted", "paid fence"} {
		t.Run(reason, func(t *testing.T) {
			p, candidates, now := priorityFixture(t)
			p = priorityPolicy(t, p, map[string]int{"a": 1000000, "b": -1000000})
			q := sample(now, .9, .9, time.Hour)
			switch reason {
			case "five-hour reserve":
				q.Groups[0].Buckets[0].Remaining = .1
			case "weekly reserve":
				q.Groups[0].Buckets[1].Remaining = .01
			case "stale":
				q.At = now.Add(-time.Hour)
			case "billing enabled":
				on := true
				q.ExtraUsage.Enabled = &on
			case "billing unknown":
				q.ExtraUsage = nil
			case "exhausted":
				q.Groups[0].Buckets[0].Remaining = 0
			case "paid fence":
				creditFences.Lock()
				loadCreditFences()
				creditFences.Denied["a"] = now
				creditFences.Unlock()
			}
			state.Quota["a"] = q
			if _, ok := rankAccount(p, candidates[0], "opus", q, now, 0); ok {
				t.Fatal("fixture must be ineligible")
			}
			if got := choose(p, "claude", "opus", nil, nil, candidates, now)["AuthID"]; got != "b" {
				t.Fatal("priority bypassed eligibility", got)
			}
		})
	}
}

func TestPriorityDefaultsAndEqualTiersKeepDynamicRanking(t *testing.T) {
	for _, values := range []map[string]int{nil, {}, {"a": 0, "b": 0}, {"a": 5, "b": 5}} {
		t.Run(fmt.Sprint(values), func(t *testing.T) {
			p, candidates, now := priorityFixture(t)
			p = priorityPolicy(t, p, values)
			state.Quota["a"] = sample(now, .8, .3, 3*time.Hour)
			state.Quota["b"] = sample(now, .9, .9, time.Hour)
			if got := choose(p, "claude", "opus", nil, nil, candidates, now)["AuthID"]; got != "b" {
				t.Fatal("dynamic ranking changed", got)
			}
			traffic.Lock()
			traffic.Active["busy"] = activeRequest{Auth: "b", At: now}
			traffic.Unlock()
			if got := choose(p, "claude", "opus", nil, nil, candidates, now)["AuthID"]; got != "a" {
				t.Fatal("equal-tier spreading changed", got)
			}
		})
	}
}

func TestPriorityOrdersEligibleAccountsBeforeDynamicRanking(t *testing.T) {
	p, candidates, now := priorityFixture(t)
	state.Quota["a"] = sample(now, .8, .3, 3*time.Hour)
	if got := choose(p, "claude", "opus", nil, nil, candidates, now)["AuthID"]; got != "b" {
		t.Fatal("fixture must dynamically prefer b", got)
	}
	p = priorityPolicy(t, p, map[string]int{"a": 1, "b": -1})
	if got := choose(p, "claude", "opus", nil, nil, candidates, now)["AuthID"]; got != "a" {
		t.Fatal("priority did not order eligible accounts", got)
	}
}

func TestPriorityValidationRejectsEveryInvalidOccurrence(t *testing.T) {
	for _, raw := range []string{
		`{"account_priority":null}`,
		`{"ACCOUNT_PRIORITY":null}`,
		`{"account_priority":[]}`,
		`{"account_priority":{"a":null,"b":50}}`,
		`{"Account_Priority":{"a":null}}`,
		`{"account_priority":{"a":1.5}}`,
		`{"account_priority":{"a":1.0}}`,
		`{"account_priority":{"a":1e2}}`,
		`{"account_priority":{"a":"1"}}`,
		`{"account_priority":{"a":true}}`,
		`{"account_priority":{"a":1000001}}`,
		`{"account_priority":{"a":-1000001}}`,
		`{"account_priority":{"a":9223372036854775808}}`,
		`{"account_priority":{"a":null},"account_priority":{"b":50}}`,
		`{"account_priority":{"a":100},"ACCOUNT_PRIORITY":{"a":null}}`,
		`{"account_priority":{"a":null,"a":100}}`,
		`{"account_priority":{"a":1000001,"a":100}}`,
		`{"account_priority":{"a":1.5},"account_priority":{"a":100}}`,
		`{"account_priority":{"a":1000001},"Account_Priority":{"a":100}}`,
		`{"account_priority":{"":1}}`,
		`{"account_priority":{" spaced":1}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var input dashboardPolicy
			if err := json.Unmarshal([]byte(raw), &input); err == nil {
				p := priorityPolicy(t, defaultPolicy(), map[string]int{})
				if json.Unmarshal([]byte(raw), &p) == nil && validatePolicy(p) == nil {
					t.Error("accepted invalid dashboard priority")
				}
			}
			p := defaultPolicy()
			if json.Unmarshal([]byte(raw), &p) == nil && validatePolicy(p) == nil {
				t.Error("accepted invalid persisted priority")
			}
		})
	}
}

func TestPriorityDecoderPreservesIntegerAndMapSemantics(t *testing.T) {
	for _, raw := range []string{
		`{"account_priority":{"a":0,"b":-5}}`,
		`{"ACCOUNT_PRIORITY":{"a":0,"b":-5}}`,
		`{"account_priority":{"a":0},"account_priority":{"b":-5}}`,
		`{"account_priority":{"a":1},"Account_Priority":{"a":0,"b":-5}}`,
		`{"account_priority":{"a":5,"a":0,"b":-5}}`,
	} {
		var input dashboardPolicy
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			t.Fatal(err)
		}
		var saved policy
		if err := json.Unmarshal([]byte(raw), &saved); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"a": float64(0), "b": float64(-5)}
		for _, value := range []any{input, saved} {
			if got := jsonObject(value)["account_priority"]; !reflect.DeepEqual(got, want) {
				t.Fatal("integer priorities changed", raw, got)
			}
		}
	}
}

func TestPriorityPublicCopyAndPolicyBounds(t *testing.T) {
	p := priorityPolicy(t, defaultPolicy(), map[string]int{"a": -1000000, "b": 1000000})
	if err := validatePolicy(p); err != nil {
		t.Fatal(err)
	}
	public := publicPolicy(p, localSecrets{})
	priorities := reflect.ValueOf(public).FieldByName("AccountPriority")
	if !priorities.IsValid() || priorities.IsNil() {
		t.Fatal("public policy omitted account priorities")
	}
	priorities.SetMapIndex(reflect.ValueOf("a"), reflect.ValueOf(8))
	if got := jsonObject(p)["account_priority"]; !reflect.DeepEqual(got, map[string]any{"a": float64(-1000000), "b": float64(1000000)}) {
		t.Fatal("public policy changed saved priorities", got)
	}
	for _, values := range []map[string]int{{"a": 1000001}, {"a": -1000001}, {"": 1}, {" spaced": 1}} {
		data := jsonObject(defaultPolicy())
		data["account_priority"] = values
		raw, _ := json.Marshal(data)
		var bad policy
		if json.Unmarshal(raw, &bad) == nil && validatePolicy(bad) == nil {
			t.Fatal("invalid priorities accepted", values)
		}
	}
}

func TestPriorityDashboardPolicyAPIRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	// Keep this API test synchronous; quota monitoring is independent.
	state.Running = true
	oldKeys, oldHost := fetchActiveClientKeys, hostCall
	fetchActiveClientKeys = func(...managementCaller) ([]string, error) { return []string{"synthetic-client"}, nil }
	hostCall = func(method string, _ any) (json.RawMessage, error) {
		if method == "host.auth.list" {
			return []byte(`{"files":[{"id":"a","provider":"claude"},{"id":"b","provider":"codex"}]}`), nil
		}
		return nil, fmt.Errorf("unexpected host call: %s", method)
	}
	t.Cleanup(func() { stopWorker(); applyConfig(nil); fetchActiveClientKeys = oldKeys; hostCall = oldHost })
	p := defaultPolicy()
	writePolicyFixture(t, p)
	clients := localSecrets{Clients: []struct{ Name, Key string }{{"test", "synthetic-client"}}}
	input := jsonObject(publicPolicy(p, clients))
	put := func(values map[string]any, status int) dashboardPolicy {
		t.Helper()
		raw, _ := json.Marshal(values)
		response := handleDashboard(dashboardRequest{Method: "PUT", Path: dashboardAPI + "/policy", Body: raw})
		if response.StatusCode != status {
			t.Fatalf("save: got %d want %d: %s", response.StatusCode, status, response.Body)
		}
		var body struct {
			Policy dashboardPolicy `json:"policy"`
		}
		if err := json.Unmarshal(response.Body, &body); err != nil {
			t.Fatal(err)
		}
		return body.Policy
	}
	want := map[string]any{"a": float64(20), "b": float64(-5)}
	input["account_priority"] = want
	saved := put(input, 200)
	got, err := readPolicy()
	if err != nil || !reflect.DeepEqual(jsonObject(got)["account_priority"], want) || !reflect.DeepEqual(jsonObject(saved)["account_priority"], want) {
		t.Fatal("priority did not round-trip through API and disk", err)
	}
	input = jsonObject(saved)
	delete(input, "account_priority")
	saved = put(input, 200)
	if !reflect.DeepEqual(jsonObject(saved)["account_priority"], want) {
		t.Fatal("older client erased priority")
	}
	for _, invalid := range []any{nil, map[string]any{"a": nil}, map[string]any{"a": 1.5}, map[string]any{"a": 1000001}, map[string]any{"unknown": 10}} {
		input = jsonObject(saved)
		input["account_priority"] = invalid
		put(input, 400)
		got, err = readPolicy()
		if err != nil || !reflect.DeepEqual(jsonObject(got)["account_priority"], want) {
			t.Fatal("rejected save changed policy", err)
		}
	}
	// Disconnected rules remain editable; new unknown credential IDs are rejected.
	hostCall = func(string, any) (json.RawMessage, error) { return []byte(`{"files":[]}`), nil }
	input = jsonObject(saved)
	input["account_priority"] = map[string]int{"a": 21, "b": -5}
	saved = put(input, 200)
	input = jsonObject(saved)
	input["account_priority"] = map[string]int{}
	saved = put(input, 200)
	got, err = readPolicy()
	if err != nil || jsonObject(got)["account_priority"] != nil || jsonObject(saved)["account_priority"] != nil {
		t.Fatal("explicit empty map did not clear priority", err)
	}
}
