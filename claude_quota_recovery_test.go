package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestClaudeRecoveryRespectsRetryAfterAndRequiresNewAdoption(t *testing.T) {
	for _, retry := range []time.Duration{time.Hour, 0} {
		t.Run(retry.String(), func(t *testing.T) {
			now := time.Now()
			cache := &quotaCache{Entries: map[string]*quotaEntry{}, Path: filepath.Join(t.TempDir(), "cache.json"), Now: func() time.Time { return now }}
			ttl := 5 * time.Minute
			oldGrant := now
			cache.get("pool", ttl, func() (snapshot, error) { return fixtureQuota(), nil })
			now = now.Add(ttl)
			cache.getForAccount("pool", "test-claude", time.Time{}, ttl, func() (snapshot, error) {
				return snapshot{}, &quotaProblem{Code: 401}
			})
			now = now.Add(15 * time.Minute)
			cache.Entries["pool"].Failures = 39
			cache.getForAccount("pool", "test-claude", time.Time{}, ttl, func() (snapshot, error) {
				return snapshot{}, &quotaProblem{Code: 429, Retry: retry}
			})
			newGrant := now.Add(30 * time.Minute)
			localRetry := cache.Entries["pool"].Next
			deadline := localRetry
			if retry > 0 {
				deadline = now.Add(retry)
				if !deadline.Before(localRetry) {
					t.Fatal("test must exercise recovery before local backoff expires")
				}
			}
			blocked := func(c *quotaCache, id string, adopted time.Time) {
				t.Helper()
				if _, err := c.getForAccount("pool", id, adopted, ttl, func() (snapshot, error) {
					t.Fatal("unexpected upstream request")
					return snapshot{}, nil
				}); err != errQuotaRecoveryNotDue {
					t.Fatalf("expected recovery conflict, got %v", err)
				}
			}
			now = deadline.Add(-time.Nanosecond)
			blocked(cache, "test-claude", newGrant)
			now = deadline
			blocked(cache, "other-claude", newGrant)
			blocked(cache, "test-claude", oldGrant)
			blocked(cache, "test-claude", now.Add(time.Second))
			blocked(&quotaCache{Path: cache.Path, Now: cache.Now}, "test-claude", newGrant)
			fresh, err := cache.getForAccount("pool", "test-claude", newGrant, ttl, func() (snapshot, error) {
				return fixtureQuota(), nil
			})
			if err != nil || !fresh.At.Equal(now) || cache.Entries["pool"].Failures != 0 {
				t.Fatalf("fresh reading not recorded: %v, %v", fresh.At, err)
			}
			blocked(cache, "test-claude", newGrant)
		})
	}
}

func TestClaudeRecoveryEndpointPublishesFreshQuotaOnce(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now()
	previousMonitor, previousHostCall := monitor, hostCall
	monitor = &quotaCache{Entries: map[string]*quotaEntry{}, Now: func() time.Time { return now }}
	t.Cleanup(func() { monitor, hostCall = previousMonitor, previousHostCall })
	const account = "synthetic-claude"
	defer func() { state.Lock(); delete(state.Quota, account); state.Unlock() }()
	var adopted time.Time
	var reads atomic.Int32
	var upstreamStatus = http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v8/management/credentials" || r.Header.Get("Authorization") != "Bearer test-management" {
			t.Error("incorrect adoption lookup")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{{
			"id": account, "provider": "claude", "auth_index": "test-index",
			"adopted_at": adopted, "requires_sign_in": false,
		}}})
	}))
	defer server.Close()
	settings.Lock()
	previousURL, previousDir := settings.ManagementURL, settings.StateDir
	settings.ManagementURL, settings.StateDir = server.URL, ""
	settings.Unlock()
	defer func() {
		settings.Lock()
		settings.ManagementURL, settings.StateDir = previousURL, previousDir
		settings.Unlock()
	}()
	if err := ensurePolicyFile(); err != nil {
		t.Fatal(err)
	}
	hostCall = func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case "host.auth.get":
			if payload.(map[string]string)["auth_index"] != "test-index" {
				t.Fatal("wrong credential selected")
			}
			return json.RawMessage(`{"json":{"access_token":"synthetic-access"}}`), nil
		case "host.http.do":
			reads.Add(1)
			quota := map[string]any{
				"five_hour":   map[string]any{"utilization": 2, "resets_at": now.Add(time.Hour)},
				"seven_day":   map[string]any{"utilization": 0, "resets_at": now.Add(48 * time.Hour)},
				"extra_usage": map[string]any{"is_enabled": false},
			}
			raw, _ := json.Marshal(quota)
			return json.Marshal(map[string]any{"StatusCode": upstreamStatus, "Body": raw})
		default:
			t.Fatalf("unexpected host call %s", method)
			return nil, nil
		}
	}
	identity := accountIdentity(quotaAccount{ID: account, Provider: "claude"})
	monitor.getForAccount(identity, account, time.Time{}, time.Minute, func() (snapshot, error) {
		return snapshot{}, &quotaProblem{Code: 401}
	})
	adopted = now.Add(time.Second)
	now = adopted.Add(time.Second)
	caller := managementCaller{Authorization: "Bearer test-management", BaseURL: "http://127.0.0.1:1"}
	body := []byte(`{"auth_id":"synthetic-claude"}`)
	if response := recoverClaudeQuota(body, caller); response.StatusCode != 200 {
		t.Fatalf("recovery failed: %d %s", response.StatusCode, response.Body)
	}
	state.Lock()
	fresh := state.Quota[account]
	state.Unlock()
	if !fresh.At.Equal(now) || fresh.ExtraUsage == nil || reads.Load() != 1 {
		t.Fatalf("fresh provider quota not published: at=%v, reads=%d", fresh.At, reads.Load())
	}
	c := candidate{ID: account, Provider: "claude"}
	p := defaultPolicy()
	if rank, eligible := rankAccount(p, c, "claude-sonnet-4-6", fresh, now, 0); !eligible {
		t.Fatalf("fresh included quota did not restore eligibility: %s", rank.Reason)
	}
	paid := true
	fresh.ExtraUsage = &extraUsageStatus{Enabled: &paid}
	if rank, eligible := rankAccount(p, c, "claude-sonnet-4-6", fresh, now, 0); eligible || rank.Reason != "Extra usage is enabled at the provider; account is excluded" {
		t.Fatalf("billing guard did not reject fresh paid quota: %s", rank.Reason)
	}
	if response := recoverClaudeQuota(body, caller); response.StatusCode != 409 || reads.Load() != 1 {
		t.Fatal("same adoption was accepted a second time")
	}
	// A failed recovery must not report success or overwrite routing quota.
	now = now.Add(time.Hour)
	monitor.getForAccount(identity, account, time.Time{}, time.Minute, func() (snapshot, error) {
		return snapshot{}, &quotaProblem{Code: 401}
	})
	adopted = now.Add(time.Second)
	now = adopted.Add(time.Second)
	upstreamStatus = http.StatusTooManyRequests
	if response := recoverClaudeQuota(body, caller); response.StatusCode != 502 {
		t.Fatalf("failed recovery reported %d", response.StatusCode)
	}
	state.Lock()
	unchanged := state.Quota[account].At.Equal(fresh.At)
	state.Unlock()
	if !unchanged || reads.Load() != 2 {
		t.Fatal("failed recovery changed routing quota or retried")
	}
}
