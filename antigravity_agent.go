package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Correlate model quotaInfo with an observed limiting bucket. Only an
// unambiguous match assigns a group; public model families are not pool IDs.
func bindAntigravityModelGroups(q *snapshot, raw []byte) error {
	var catalog struct {
		Models map[string]struct {
			Disabled bool `json:"disabled"`
			Quota    *struct {
				BucketID string `json:"bucketId"`
				Window   string `json:"window"`
				Reset    string `json:"resetTime"`
				Disabled bool   `json:"disabled"`
			} `json:"quotaInfo"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &catalog) != nil || catalog.Models == nil {
		return fmt.Errorf("invalid Antigravity model quota catalog")
	}
	for i := range q.Groups {
		q.Groups[i].Models = []string{}
	}
	at := q.At
	if at.IsZero() {
		at = time.Now()
	}
	for id, model := range catalog.Models {
		if model.Disabled || model.Quota == nil || model.Quota.Disabled {
			continue
		}
		reset := parseReset(model.Quota.Reset)
		if !reset.After(at) {
			continue
		}
		matched, compatible := -1, false
		window := normalizedQuotaWindow("antigravity", model.Quota.Window, "")
		for i, g := range q.Groups {
			for _, b := range g.Buckets {
				// Fractions can change between sequential provider reads. Without
				// a bucket ID, the exact reset must identify only one group,
				// even when a window or fraction could select among collisions.
				if reset.Equal(b.Reset) && (model.Quota.BucketID == "" || model.Quota.BucketID == b.ID) {
					if matched >= 0 && matched != i {
						matched = -2
						break
					}
					matched = i
					compatible = compatible || (!b.Disabled && (window == "" || window == normalizedQuotaWindow("antigravity", b.Window, "")))
				}
			}
			if matched == -2 {
				break
			}
		}
		if matched >= 0 && compatible {
			q.Groups[matched].Models = append(q.Groups[matched].Models, id)
		}
	}
	for i := range q.Groups {
		sort.Strings(q.Groups[i].Models)
	}
	return nil
}

// ABI 7 does not expose the runtime UA. Use the same official Hub manifest
// as CLIProxyAPI instead of maintaining a second pinned client version.
const antigravityManifest = "https://antigravity-hub-auto-updater-974169037036.us-central1.run.app/manifest/latest-arm64-mac.yml"

var hubVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
var antigravityAgent = struct {
	sync.Mutex
	value string
	next  time.Time
}{}

func antigravityQuotaAgent() (string, error) {
	antigravityAgent.Lock()
	defer antigravityAgent.Unlock()
	if time.Now().Before(antigravityAgent.next) {
		if antigravityAgent.value == "" {
			return "", fmt.Errorf("Antigravity Hub version temporarily unavailable")
		}
		return antigravityAgent.value, nil
	}
	response, err := hostHTTP("GET", antigravityManifest, map[string]string{"Accept": "application/yaml"}, "")
	var manifest struct {
		Version string `yaml:"version"`
	}
	if err != nil || response.Code != http.StatusOK || len(response.Body) > 64<<10 || yaml.Unmarshal([]byte(response.Body), &manifest) != nil || !hubVersionPattern.MatchString(manifest.Version) {
		antigravityAgent.next = time.Now().Add(time.Minute)
		if antigravityAgent.value != "" {
			return antigravityAgent.value, nil
		}
		return "", fmt.Errorf("Antigravity Hub version unavailable")
	}
	antigravityAgent.value = "antigravity/hub/" + manifest.Version + " (aidev_client; os_type=darwin; arch=arm64)"
	antigravityAgent.next = time.Now().Add(6 * time.Hour)
	return antigravityAgent.value, nil
}
