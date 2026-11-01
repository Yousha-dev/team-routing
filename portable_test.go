package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPortableFreshInstall(t *testing.T) {
	t.Chdir(t.TempDir())
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	if err := ensurePolicyFile(); err != nil {
		t.Fatal(err)
	}
	p, err := readPolicy()
	if err != nil || p.WeeklyRatio != 5 || len(p.Owned) != 0 {
		t.Fatalf("bad portable defaults: %+v %v", p, err)
	}
	if _, err := os.Stat("secrets.json"); !os.IsNotExist(err) {
		t.Fatal("test unexpectedly has secrets")
	}
	if _, err := secretsForDashboard(); err != nil {
		t.Fatal("secrets file required", err)
	}
}
func TestPortableBrowserOriginAndOverride(t *testing.T) {
	applyConfig(nil)
	t.Cleanup(func() { applyConfig(nil) })
	r := httptest.NewRequest("GET", "https://proxy.example:19317/path", nil)
	// Host is deliberately not consulted; it is absent from core's header map.
	r.Header.Set("X-Team-Routing-Origin", "https://proxy.example:19317")
	if c := callerFromRequest(r.Header); c.BaseURL != "https://proxy.example:19317" {
		t.Fatal(c.BaseURL)
	}
	applyConfig([]byte("management_url: https://internal.example:9443/base\n"))
	if c := callerFromRequest(r.Header); c.BaseURL != "https://internal.example:9443/base" {
		t.Fatal("explicit override ignored")
	}
}
func TestPortableConcurrentCallerIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v8/management/config/access/api-keys" {
			json.NewEncoder(w).Encode([]string{strings.Replace(r.Header.Get("Authorization"), "admin-", "client-", 1)[7:]})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"auth": r.Header.Get("Authorization")})
	}))
	defer server.Close()
	var wg sync.WaitGroup
	for _, name := range []string{"a", "b", "c", "d"} {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				c := managementCaller{BaseURL: server.URL, Authorization: "Bearer admin-" + name}
				var out struct{ Auth string }
				if e := localJSON("GET", "/v1/models", &out, c); e != nil {
					t.Error(e)
					return
				}
				if out.Auth != "Bearer client-"+name {
					t.Errorf("caller crossed: %s: %s", name, out.Auth)
				}
			}
		}()
	}
	wg.Wait()
}
func TestPortableRejectsRedirectAndInvalidURL(t *testing.T) {
	var leaked atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer sink.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, sink.URL, 302) }))
	defer redirect.Close()
	c := managementCaller{BaseURL: redirect.URL, Authorization: "Bearer own-test-key"}
	var out any
	if localJSON("GET", "/config", &out, c) == nil || leaked.Load() != 0 {
		t.Fatal("redirect followed")
	}
	c.BaseURL = "http://user:password@localhost"
	if localJSON("GET", "/config", &out, c) == nil {
		t.Fatal("userinfo accepted")
	}
}
