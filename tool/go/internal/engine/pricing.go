package engine

import "math"

// PricingRate is a bundled, dated standard API rate, in USD per million tokens.
// It is not a subscription charge or an observed provider billing tier.
type PricingRate struct {
	InputUSDPerMillion        float64 `json:"inputUsdPerMillion"`
	CachedInputUSDPerMillion  float64 `json:"cachedInputUsdPerMillion"`
	CacheWriteUSDPerMillion   float64 `json:"cacheWriteUsdPerMillion"`
	CacheWrite1hUSDPerMillion float64 `json:"cacheWrite1hUsdPerMillion,omitempty"`
	OutputUSDPerMillion       float64 `json:"outputUsdPerMillion"`
	SourceURL                 string  `json:"sourceUrl"`
	SupplementalSourceURL     string  `json:"supplementalSourceUrl"`
	CheckedAt                 string  `json:"checkedAt"`
}

// CostEstimate records one invocation whose provider did not report a price.
type CostEstimate struct {
	Provider                string      `json:"provider"`
	Phase                   string      `json:"phase"`
	RequestedModel          string      `json:"requestedModel"`
	CostUSD                 float64     `json:"costUsd"`
	UncachedInputTokens     int64       `json:"uncachedInputTokens"`
	CachedInputTokens       int64       `json:"cachedInputTokens"`
	CacheWriteInputTokens   int64       `json:"cacheWriteInputTokens"`
	CacheWrite1hInputTokens int64       `json:"cacheWrite1hInputTokens,omitempty"`
	OutputTokens            int64       `json:"outputTokens"`
	ReasoningOutputTokens   int64       `json:"reasoningOutputTokens,omitempty"`
	Rates                   PricingRate `json:"rates"`
	Assumption              string      `json:"assumption"`
	Notes                   []string    `json:"notes,omitempty"`
}

type CostEstimateMissing struct {
	Provider       string `json:"provider"`
	Phase          string `json:"phase"`
	RequestedModel string `json:"requestedModel"`
	Reason         string `json:"reason"`
}

func bundledRate(provider, model string) (PricingRate, bool) {
	rate := PricingRate{CheckedAt: "2026-10-07"}
	switch {
	case provider == "codex" && model == "gpt-5.6-sol":
		rate.InputUSDPerMillion, rate.CachedInputUSDPerMillion, rate.CacheWriteUSDPerMillion, rate.OutputUSDPerMillion = 4, 0.4, 5, 20
		rate.SourceURL = "https://artificialanalysis.ai/models/comparisons/gpt-5-6-sol-vs-gpt-5-4"
		rate.SupplementalSourceURL = "https://developers.openai.com/api/docs/models/gpt-5.6-sol"
	case provider == "codex" && model == "gpt-6.1-sol":
		rate.InputUSDPerMillion, rate.CachedInputUSDPerMillion, rate.CacheWriteUSDPerMillion, rate.OutputUSDPerMillion = 2, 0.1, 2.5, 10
		rate.SourceURL = "https://artificialanalysis.ai/models/comparisons/gpt-6-1-sol-xhigh-vs-claude-sonnet-5-5"
		rate.SupplementalSourceURL = "https://developers.openai.com/api/docs/models/gpt-6.1-sol"
	case provider == "claude" && model == "claude-sonnet-5-5":
		rate.InputUSDPerMillion, rate.CachedInputUSDPerMillion, rate.CacheWriteUSDPerMillion, rate.OutputUSDPerMillion = 2, 0.2, 2.5, 10
		rate.CacheWrite1hUSDPerMillion = 4
		rate.SourceURL = "https://artificialanalysis.ai/models/comparisons/gpt-6-1-sol-xhigh-vs-claude-sonnet-5-5"
		rate.SupplementalSourceURL = "https://platform.claude.com/docs/en/models/sonnet-5-5/overview"
	default:
		return PricingRate{}, false
	}
	return rate, true
}

// parseTokenUsage keeps provider-specific cache semantics outside aggregation.
// Codex cache counts are subsets of input; Claude reports separate categories.
func parseTokenUsage(data map[string]any, provider string) Usage {
	u := Usage{}
	readFrom := func(values map[string]any, key string) (int64, bool) {
		value, present := values[key]
		if !present {
			return 0, false
		}
		f, ok := value.(float64)
		if !ok || f < 0 || f >= math.Exp2(63) || math.Trunc(f) != f || math.IsNaN(f) || math.IsInf(f, 0) {
			u.invalidUsage = true
			return 0, false
		}
		return int64(f), true
	}
	read := func(key string) (int64, bool) { return readFrom(data, key) }
	var inputOK, outputOK bool
	u.InputTokens, inputOK = read("input_tokens")
	u.OutputTokens, outputOK = read("output_tokens")
	u.usageReported = inputOK && outputOK
	if provider == "codex" {
		u.CachedInputTokens, u.cacheReadReported = read("cached_input_tokens")
		u.CacheWriteInputTokens, u.cacheWriteReported = read("cache_write_input_tokens")
		u.ReasoningOutputTokens, _ = read("reasoning_output_tokens")
		if u.CachedInputTokens > u.InputTokens || u.CacheWriteInputTokens > u.InputTokens-u.CachedInputTokens || u.ReasoningOutputTokens > u.OutputTokens {
			u.invalidUsage = true
		} else {
			u.UncachedInputTokens = u.InputTokens - u.CachedInputTokens - u.CacheWriteInputTokens
		}
		return u
	}
	u.UncachedInputTokens = u.InputTokens
	u.CachedInputTokens, u.cacheReadReported = read("cache_read_input_tokens")
	u.CacheWriteInputTokens, u.cacheWriteReported = read("cache_creation_input_tokens")
	if creation, ok := data["cache_creation"].(map[string]any); ok {
		u.CacheWrite1hInputTokens, u.cacheWriteTTLReported = readFrom(creation, "ephemeral_1h_input_tokens")
		u.invalidUsage = u.invalidUsage || u.CacheWrite1hInputTokens > u.CacheWriteInputTokens
	}
	if u.CachedInputTokens > math.MaxInt64-u.InputTokens || u.CacheWriteInputTokens > math.MaxInt64-u.InputTokens-u.CachedInputTokens {
		u.invalidUsage = true
	} else {
		u.InputTokens += u.CachedInputTokens + u.CacheWriteInputTokens
	}
	return u
}

// EstimateUsage only supplements a missing provider price. Invoke it before
// merging calls so mixed providers and schema repairs retain their own rates.
func EstimateUsage(u Usage, provider, requestedModel, phase string) Usage {
	if u.CostUSD != nil || u.EstimatedCostUSD != nil || u.EstimateMissing > 0 {
		return u
	}
	rate, known := bundledRate(provider, requestedModel)
	reason := ""
	switch {
	case !known:
		reason = "UNKNOWN_MODEL"
	case u.invalidUsage:
		reason = "INVALID_USAGE"
	case !u.usageReported:
		reason = "MISSING_USAGE"
	}
	if reason != "" {
		u.EstimateMissing = 1
		u.EstimateMissingReasons = []CostEstimateMissing{{Provider: provider, Phase: phase, RequestedModel: requestedModel, Reason: reason}}
		return u
	}
	cost := (float64(u.UncachedInputTokens)*rate.InputUSDPerMillion + float64(u.CachedInputTokens)*rate.CachedInputUSDPerMillion +
		float64(u.CacheWriteInputTokens-u.CacheWrite1hInputTokens)*rate.CacheWriteUSDPerMillion + float64(u.CacheWrite1hInputTokens)*rate.CacheWrite1hUSDPerMillion +
		float64(u.OutputTokens)*rate.OutputUSDPerMillion) / 1_000_000
	estimate := CostEstimate{Provider: provider, Phase: phase, RequestedModel: requestedModel, CostUSD: cost,
		UncachedInputTokens: u.UncachedInputTokens, CachedInputTokens: u.CachedInputTokens, CacheWriteInputTokens: u.CacheWriteInputTokens,
		CacheWrite1hInputTokens: u.CacheWrite1hInputTokens, OutputTokens: u.OutputTokens, ReasoningOutputTokens: u.ReasoningOutputTokens,
		Rates: rate, Assumption: "standard API rates for the requested model; no speed, region, batch or long-context modifiers; not subscription billing"}
	if u.invalidReportedCost {
		estimate.Notes = append(estimate.Notes, "INVALID_REPORTED_COST_IGNORED")
	}
	if !u.cacheReadReported {
		estimate.Notes = append(estimate.Notes, "MISSING_CACHE_READ_ASSUMED_ZERO")
	}
	if !u.cacheWriteReported {
		estimate.Notes = append(estimate.Notes, "MISSING_CACHE_WRITE_ASSUMED_ZERO")
	}
	if provider == "claude" && u.CacheWriteInputTokens > 0 && !u.cacheWriteTTLReported {
		estimate.Notes = append(estimate.Notes, "CACHE_WRITE_TTL_ASSUMED_5M")
	}
	u.EstimatedCostUSD = &cost
	u.CostEstimates = []CostEstimate{estimate}
	return u
}

func sumCost(a, b *float64) *float64 {
	if a == nil && b == nil {
		return nil
	}
	value := 0.0
	if a != nil {
		value += *a
	}
	if b != nil {
		value += *b
	}
	return &value
}
