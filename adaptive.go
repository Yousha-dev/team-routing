package main

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Rates are fractions of a quota window consumed per hour, not token counts.
// A reset, increased allowance, or long observation gap invalidates the rate.
func estimateRates(old, next snapshot, now time.Time) map[string]float64 {
	rates := map[string]float64{}
	hours := now.Sub(old.At).Hours()
	if old.At.IsZero() || hours < 1.0/120 || hours > 6 {
		return rates
	}
	previous := map[string]bucket{}
	for _, g := range old.Groups {
		for _, b := range g.Buckets {
			previous[g.Name+"|"+b.Window] = b
		}
	}
	for _, g := range next.Groups {
		for _, b := range g.Buckets {
			k := g.Name + "|" + b.Window
			prior, ok := previous[k]
			if !ok || !prior.Reset.Equal(b.Reset) || b.Remaining > prior.Remaining {
				continue
			}
			observed := (prior.Remaining - b.Remaining) / hours
			if rate, ok := old.Rates[k]; ok {
				observed = .5*rate + .5*observed
			}
			rates[k] = observed
		}
	}
	return rates
}

type activeRequest struct {
	Auth string
	At   time.Time
}

var traffic = struct {
	sync.Mutex
	Active    map[string]activeRequest
	Completed uint64
}{Active: map[string]activeRequest{}}

// Observe lifecycle only; never rewrite prompts, credentials, or responses.
func trackRequest(method string, raw []byte) (any, error) {
	var r struct {
		RequestID string
		Metadata  map[string]any
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.RequestID == "" {
		return map[string]any{}, nil
	}
	traffic.Lock()
	defer traffic.Unlock()
	if method == "request.complete" {
		if _, ok := traffic.Active[r.RequestID]; ok {
			traffic.Completed++
		}
		delete(traffic.Active, r.RequestID)
	}
	if method == "request.intercept_after" {
		if id, ok := r.Metadata["selected_auth_id"].(string); ok && id != "" {
			traffic.Active[r.RequestID] = activeRequest{id, time.Now()}
		}
	}
	return map[string]any{}, nil
}
func activeCounts(now time.Time) map[string]int {
	traffic.Lock()
	defer traffic.Unlock()
	counts := map[string]int{}
	for k, r := range traffic.Active {
		if now.Sub(r.At) > 2*time.Hour {
			delete(traffic.Active, k)
		} else {
			counts[r.Auth]++
		}
	}
	return counts
}

type adaptiveRank struct {
	ID               string
	Score, Remaining float64
	Known            bool
}

func weeklyWindow(w string) bool {
	return w == "weekly" || w == "seven_day" || strings.HasPrefix(w, "seven_day_")
}
func relevantGroup(provider, model, name string) bool {
	if provider != "antigravity" {
		return true
	}
	name, model = strings.ToLower(name), strings.ToLower(model)
	return strings.Contains(name, model) || (strings.HasPrefix(model, "gemini") && strings.Contains(name, "gemini")) || ((strings.HasPrefix(model, "claude") || strings.HasPrefix(model, "gpt")) && strings.Contains(name, "claude"))
}
func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func rankAccount(p policy, c candidate, model string, q snapshot, now time.Time, active int) (adaptiveRank, bool) {
	rank := adaptiveRank{ID: c.ID, Score: 1, Remaining: 1}
	fresh := !q.At.IsZero() && now.Sub(q.At) < time.Duration(p.MaxAge)*time.Second
	if (c.Provider == "claude" || c.Provider == "codex") && !fresh {
		return rank, false
	}
	five, week := 1.0, 1.0
	fiveExpiry, weekExpiry, pressure := 0.0, 0.0, 0.0
	resetMinutes := math.Inf(1)
	fiveRate := 0.0
	hasFive, hasWeek := false, false
	if fresh {
		for _, g := range q.Groups {
			if !relevantGroup(c.Provider, model, g.Name) {
				continue
			}
			for _, b := range g.Buckets {
				if b.Window == "extra_usage" {
					continue
				}
				// No soft rule can reopen exhausted included quota, even if a reset passed.
				if b.Remaining <= 0 {
					return rank, false
				}
				if b.Window != "five_hour" && !weeklyWindow(b.Window) {
					continue
				}
				rank.Known = true
				rate := q.Rates[g.Name+"|"+b.Window]
				// Reserve for unreported consumption since the sample and active requests.
				// The 0.5%/request floor is a conservative heuristic, not a billing guarantee.
				pending := float64(active) * math.Max(.005, rate/60)
				remaining := b.Remaining - rate*math.Max(0, now.Sub(q.At).Hours()) - pending
				if remaining <= 0 {
					return rank, false
				}
				rank.Remaining = math.Min(rank.Remaining, remaining)
				minutes := b.Reset.Sub(now).Minutes()
				if b.Reset.After(now) {
					resetMinutes = math.Min(resetMinutes, minutes)
				}
				waste := remaining
				if rate > 0 && minutes > 0 {
					waste = math.Max(0, remaining-rate*minutes/60)
				}
				if b.Window == "five_hour" {
					hasFive = true
					five = math.Min(five, remaining)
					fiveRate = math.Max(fiveRate, rate)
					if minutes > 0 {
						fiveExpiry = math.Max(fiveExpiry, 1.5*waste*clamp(remaining/(1-p.ExpiringUsed+1e-9), 0, 1)*float64(p.ExpiringSeconds)/(float64(p.ExpiringSeconds)+minutes*60))
					}
				} else {
					hasWeek = true
					week = math.Min(week, remaining)
					if minutes > 0 {
						weekExpiry = math.Max(weekExpiry, waste*60/(60+minutes))
						if rate > 0 {
							pressure = math.Max(pressure, clamp(1-remaining/(rate*minutes/60), 0, 1))
						}
					}
				}
			}
		}
	}
	rank.Score += .7*five + .4*week + fiveExpiry + weekExpiry - .4*pressure
	if preferredClaude(p, c, q, now) {
		rank.Score += .08
	}
	if hasFive && 1-five >= p.Cutoff {
		// Crossing 85% increases avoidance smoothly. Recent near resets reduce that
		// penalty, but lack of alternatives never strands non-exhausted quota.
		penalty := .5 * clamp((1-five-p.Cutoff)/(1-p.Cutoff+1e-9), 0, 1)
		if now.Sub(q.At) <= time.Duration(p.Poll+30)*time.Second {
			penalty *= clamp(resetMinutes/30, .1, 1)
		}
		rank.Score -= penalty
	}
	// Short runway is risky even if a broad quota window still looks healthy.
	if fiveRate > 0 && hasFive {
		rank.Score -= .2 * clamp(1-(five/fiveRate)/.25, 0, 1)
	}
	if hasWeek {
		rank.Score -= .1 * (1 - week)
	}
	if fresh {
		rank.Score -= .15 * clamp(now.Sub(q.At).Seconds()/float64(p.MaxAge), 0, 1)
	}
	rank.Score -= .04 * float64(active)
	return rank, true
}

func adaptiveChoose(p policy, provider, model string, h http.Header, meta map[string]any, candidates []candidate, now time.Time) map[string]any {
	counts := activeCounts(now)
	ranks := []adaptiveRank{}
	for _, c := range candidates {
		if r, ok := rankAccount(p, c, model, state.Quota[c.ID], now, counts[c.ID]); ok {
			ranks = append(ranks, r)
		}
	}
	if len(ranks) == 0 {
		return map[string]any{"Handled": true, "Reject": true, "RejectCode": "routing_quota_unavailable", "RejectReason": "No eligible included subscription quota with sufficiently fresh readings. Check Team Routing."}
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		if math.Abs(ranks[i].Score-ranks[j].Score) > 1e-9 {
			return ranks[i].Score > ranks[j].Score
		}
		return ranks[i].ID < ranks[j].ID
	})
	pk := digest(key(h) + "|" + session(h, meta) + "|" + provider + "|" + model)
	best := ranks[0]
	pin, exists := state.Pins[pk]
	if exists && now.Sub(pin.At) < time.Hour {
		for _, r := range ranks {
			if r.ID != pin.ID {
				continue
			}
			boundAt := pin.BoundAt
			if boundAt.IsZero() {
				boundAt = pin.At
			}
			// No tiny score-driven moves. Rebalance at most once per ten minutes and
			// only for both an absolute and relative material improvement.
			move := now.Sub(boundAt) >= 10*time.Minute && best.Known && r.Known && best.Score > r.Score+.5 && best.Score > r.Score*1.35
			if !move {
				state.Pins[pk] = binding{ID: pin.ID, At: now, BoundAt: boundAt}
				return map[string]any{"Handled": true, "AuthID": pin.ID}
			}
			break
		}
	}
	if len(state.Pins) > 10000 {
		for k, v := range state.Pins {
			if now.Sub(v.At) > time.Hour {
				delete(state.Pins, k)
			}
		}
	}
	state.Pins[pk] = binding{ID: best.ID, At: now, BoundAt: now}
	return map[string]any{"Handled": true, "AuthID": best.ID}
}
