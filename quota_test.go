package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureQuota() snapshot {
	return snapshot{Groups: []group{{Name: "Claude", Buckets: []bucket{{Window: "five_hour", Remaining: .8}}}}}
}
func TestQuotaCacheTTLAndBackoff(t *testing.T) {
	now := time.Now()
	q := &quotaCache{Entries: map[string]*quotaEntry{}, Path: filepath.Join(t.TempDir(), "cache.json"), Now: func() time.Time { return now }}
	calls := 0
	fetch := func() (snapshot, error) { calls++; return fixtureQuota(), nil }
	first, e := q.get("account", 5*time.Minute, fetch)
	if e != nil {
		t.Fatal(e)
	}
	now = now.Add(time.Minute)
	again, e := q.get("account", 5*time.Minute, fetch)
	if e != nil || calls != 1 || !again.At.Equal(first.At) {
		t.Fatal("fresh quota re-fetched or timestamp advanced")
	}
	now = now.Add(5 * time.Minute)
	limited := func() (snapshot, error) { calls++; return snapshot{}, &quotaProblem{429, 30 * time.Minute} }
	stale, e := q.get("account", 5*time.Minute, limited)
	if e != nil || !stale.At.Equal(first.At) {
		t.Fatal("last good quota not retained")
	}
	q.get("account", 5*time.Minute, limited)
	if calls != 2 {
		t.Fatal("backoff bypassed")
	}
	next := q.Entries["account"].Next
	if next.Sub(now) < 30*time.Minute {
		t.Fatal("Retry-After ignored")
	}
	reloaded := &quotaCache{Entries: map[string]*quotaEntry{}, Path: q.Path, Now: func() time.Time { return now }}
	reloaded.get("account", 5*time.Minute, limited)
	if calls != 2 {
		t.Fatal("restart bypassed backoff")
	}
}
func TestQuotaRequestsCoalesce(t *testing.T) {
	q := &quotaCache{Entries: map[string]*quotaEntry{}, Now: time.Now}
	var count atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	fetch := func() (snapshot, error) {
		if count.Add(1) == 1 {
			close(entered)
		}
		<-release
		return fixtureQuota(), nil
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); q.get("same", time.Minute, fetch) }()
	<-entered
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); q.get("same", time.Minute, fetch) }()
	}
	close(release)
	wg.Wait()
	if count.Load() != 1 {
		t.Fatal("concurrent duplicate provider reads")
	}
}
func TestQuotaParsersAndIdentity(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		provider, raw, window string
		remaining             float64
	}{{"claude", `{"five_hour":{"utilization":25,"resets_at":"2026-10-29T21:00:00Z"}}`, "five_hour", .75}, {"codex", `{"rate_limit":{"primary_window":{"used_percent":10,"limit_window_seconds":18000,"reset_at":1800000000}}}`, "five_hour", .9}, {"antigravity", `{"groups":[{"displayName":"Gemini Models","buckets":[{"displayName":"Weekly","remainingFraction":0.6,"resetTime":"2026-11-01T00:00:00Z"}]}]}`, "weekly", .6}} {
		q, e := parseProviderQuota(test.provider, []byte(test.raw), now)
		if e != nil || q.Groups[0].Buckets[0].Window != test.window || q.Groups[0].Buckets[0].Remaining != test.remaining {
			t.Fatalf("parser %s: %v", test.provider, e)
		}
	}
	a := quotaAccount{ID: "file-one", Provider: "claude", Metadata: map[string]any{"email": "Owner@example.com", "organization_uuid": "org"}}
	b := a
	b.ID = "file-two"
	if accountIdentity(a) != accountIdentity(b) {
		t.Fatal("duplicate account not deduplicated")
	}
	if _, e := parseProviderQuota("claude", []byte(`{"five_hour":{}}`), now); e == nil {
		t.Fatal("missing quota treated as zero usage")
	}
	if retryAfter(http.Header{"Retry-After": []string{"120"}}, now) != 2*time.Minute {
		t.Fatal("retry header")
	}
	if _, e := (&quotaCache{Entries: map[string]*quotaEntry{}, Now: time.Now}).get("bad", time.Minute, func() (snapshot, error) { return snapshot{}, fmt.Errorf("network") }); e == nil {
		t.Fatal("failed read created quota")
	}
}
