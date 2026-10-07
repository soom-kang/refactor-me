package engine

import (
	"fmt"
	"math"
	"time"
)

type ProviderState struct {
	Status        string       `json:"status"`
	CooldownUntil *time.Time   `json:"cooldownUntil"`
	Calls         int          `json:"calls"`
	QuotaHits     int          `json:"quotaHits"`
	LastError     *string      `json:"lastError"`
	Usage         UsageSummary `json:"usage"`
}
type RingState struct {
	Providers      map[string]*ProviderState `json:"providers"`
	ProviderOrder  []string                  `json:"providerOrder"`
	ActiveProvider string                    `json:"activeProvider"`
	Usage          *RingUsage                `json:"usage,omitempty"`
}
type RingUsage struct {
	ByPhase map[string]UsageSummary `json:"byPhase"`
}
type UsageSummary struct {
	Processes               int                   `json:"processes"`
	MS                      int64                 `json:"ms"`
	InputTokens             int64                 `json:"inputTokens"`
	OutputTokens            int64                 `json:"outputTokens"`
	UncachedInputTokens     int64                 `json:"uncachedInputTokens,omitempty"`
	CachedInputTokens       int64                 `json:"cachedInputTokens,omitempty"`
	CacheWriteInputTokens   int64                 `json:"cacheWriteInputTokens,omitempty"`
	CacheWrite1hInputTokens int64                 `json:"cacheWrite1hInputTokens,omitempty"`
	ReasoningOutputTokens   int64                 `json:"reasoningOutputTokens,omitempty"`
	CostUSD                 *float64              `json:"costUsd"`
	CostMissing             int                   `json:"costMissing"`
	EstimatedCostUSD        *float64              `json:"estimatedCostUsd,omitempty"`
	EstimateMissing         int                   `json:"estimateMissing,omitempty"`
	CostEstimates           []CostEstimate        `json:"costEstimates,omitempty"`
	EstimateMissingReasons  []CostEstimateMissing `json:"estimateMissingReasons,omitempty"`
	FailedCalls             int                   `json:"failedCalls"`
	Calls                   int                   `json:"calls,omitempty"`
}

func NewProviderRing(names []string) map[string]*ProviderState {
	ring := make(map[string]*ProviderState, len(names))
	for _, name := range names {
		ring[name] = &ProviderState{Status: "READY"}
	}
	return ring
}
func ReadyProviders(state *RingState, now time.Time) []string {
	order := state.ProviderOrder
	if len(order) == 0 {
		order = Providers
	}
	var ready []string
	for _, name := range order {
		p := state.Providers[name]
		if p == nil || p.Status == "DISABLED" || p.Status == "DEAD" {
			continue
		}
		if p.Status == "COOLDOWN" {
			if p.CooldownUntil != nil && p.CooldownUntil.After(now) {
				continue
			}
			p.Status = "READY"
			p.CooldownUntil = nil
		}
		ready = append(ready, name)
	}
	return ready
}
func PickProvider(state *RingState, prefer, exclude string) string {
	ready := ReadyProviders(state, time.Now())
	for _, p := range ready {
		if p == prefer && p != exclude {
			return p
		}
	}
	for _, p := range ready {
		if p == state.ActiveProvider && p != exclude {
			return p
		}
	}
	for _, p := range ready {
		if p != exclude {
			return p
		}
	}
	return ""
}
func PickReviewer(state *RingState, writer string) string {
	ready := ReadyProviders(state, time.Now())
	other := ""
	for _, p := range Providers {
		if p != writer {
			other = p
			break
		}
	}
	for _, p := range ready {
		if p == other {
			return p
		}
	}
	for _, p := range ready {
		if p == writer {
			return p
		}
	}
	return ""
}
func NoteFailure(state *RingState, provider string, res Result, cooldown time.Duration) {
	p := state.Providers[provider]
	if p == nil {
		return
	}
	message := res.Detail
	if message == "" {
		message = res.Failure
	}
	p.LastError = &message
	if res.Failure == "QUOTA" {
		p.QuotaHits++
		if res.Hard {
			until := time.Now().Add(cooldown).UTC()
			p.Status = "COOLDOWN"
			p.CooldownUntil = &until
		}
	} else if res.Failure == "AUTH" {
		p.Status = "DEAD"
	}
}
func NoteCall(state *RingState, provider string) {
	if p := state.Providers[provider]; p != nil {
		p.Calls++
	}
	state.ActiveProvider = provider
}

// MergeUsageSummaries sums already accounted calls without inferring a price.
func MergeUsageSummaries(a, b UsageSummary) UsageSummary {
	return UsageSummary{Processes: a.Processes + b.Processes, MS: a.MS + b.MS, InputTokens: a.InputTokens + b.InputTokens,
		OutputTokens: a.OutputTokens + b.OutputTokens, UncachedInputTokens: a.UncachedInputTokens + b.UncachedInputTokens,
		CachedInputTokens: a.CachedInputTokens + b.CachedInputTokens, CacheWriteInputTokens: a.CacheWriteInputTokens + b.CacheWriteInputTokens,
		CacheWrite1hInputTokens: a.CacheWrite1hInputTokens + b.CacheWrite1hInputTokens, ReasoningOutputTokens: a.ReasoningOutputTokens + b.ReasoningOutputTokens,
		CostUSD: sumCost(a.CostUSD, b.CostUSD), CostMissing: a.CostMissing + b.CostMissing,
		EstimatedCostUSD: sumCost(a.EstimatedCostUSD, b.EstimatedCostUSD), EstimateMissing: a.EstimateMissing + b.EstimateMissing,
		CostEstimates:          append(append([]CostEstimate{}, a.CostEstimates...), b.CostEstimates...),
		EstimateMissingReasons: append(append([]CostEstimateMissing{}, a.EstimateMissingReasons...), b.EstimateMissingReasons...),
		FailedCalls:            a.FailedCalls + b.FailedCalls, Calls: a.Calls + b.Calls}
}

// UsageSummaryForResult preserves provider-reported and estimated amounts as
// separate totals, including calls merged by schema repair.
func UsageSummaryForResult(result Result) UsageSummary {
	missing := result.Usage.CostMissing
	if result.Usage.CostUSD == nil && missing == 0 {
		missing = result.Processes
	}
	failed := 0
	if !result.OK {
		failed = 1
	}
	u := result.Usage
	return UsageSummary{Processes: result.Processes, MS: result.DurationMS, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
		UncachedInputTokens: u.UncachedInputTokens, CachedInputTokens: u.CachedInputTokens, CacheWriteInputTokens: u.CacheWriteInputTokens,
		CacheWrite1hInputTokens: u.CacheWrite1hInputTokens, ReasoningOutputTokens: u.ReasoningOutputTokens,
		CostUSD: u.CostUSD, CostMissing: missing, EstimatedCostUSD: u.EstimatedCostUSD, EstimateMissing: u.EstimateMissing,
		CostEstimates: append([]CostEstimate{}, u.CostEstimates...), EstimateMissingReasons: append([]CostEstimateMissing{}, u.EstimateMissingReasons...),
		FailedCalls: failed, Calls: 1}
}

func NoteUsage(state *RingState, provider, phase string, result Result) {
	one := UsageSummaryForResult(result)
	if p := state.Providers[provider]; p != nil {
		p.Usage = MergeUsageSummaries(p.Usage, one)
	}
	if state.Usage == nil {
		state.Usage = &RingUsage{ByPhase: map[string]UsageSummary{}}
	}
	if state.Usage.ByPhase == nil {
		state.Usage.ByPhase = map[string]UsageSummary{}
	}
	state.Usage.ByPhase[phase] = MergeUsageSummaries(state.Usage.ByPhase[phase], one)
}
func FmtTokens(n int64) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	if n < 100000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.0fk", math.Round(float64(n)/1000))
}
func FmtMS(ms int64) string {
	sec := int64(math.Round(float64(ms) / 1000))
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	return fmt.Sprintf("%dm%02ds", sec/60, sec%60)
}
func UsageLine(phase, provider string, res Result) string {
	cost := "cost n/a"
	if res.Usage.CostUSD != nil {
		cost = fmt.Sprintf("$%.2f", *res.Usage.CostUSD)
		if res.Usage.CostMissing > 0 {
			cost += "+"
		}
	}
	return fmt.Sprintf("%s · %s · %s · %s↓ / %s↑ · %s", phase, provider, FmtMS(res.DurationMS), FmtTokens(res.Usage.InputTokens), FmtTokens(res.Usage.OutputTokens), cost)
}
