package engine

import (
	"math"
	"testing"
)

func TestAstraPricingRecordedUsageAndTimeout(t *testing.T) {
	// Token counts from one successful audit in run 20261016-1424-532d.
	usage := parseCodex(`{"type":"turn.completed","usage":{"input_tokens":1001883,"cached_input_tokens":885248,"output_tokens":9193,"reasoning_output_tokens":4356}}`, "").Usage
	estimated := EstimateUsage(usage, "codex", "gpt-6-astra", "audit")
	if estimated.EstimatedCostUSD == nil || math.Abs(*estimated.EstimatedCostUSD-2.511248) > 1e-9 || estimated.EstimateMissing != 0 {
		t.Fatalf("recorded Astra usage was not priced: %+v", estimated)
	}
	rate := estimated.CostEstimates[0].Rates
	if rate.CheckedAt != "2026-10-17" || rate.SourceURL != "https://artificialanalysis.ai/models/gpt-6-astra" || rate.SupplementalSourceURL != "https://developers.openai.com/api/docs/models/gpt-6-astra" {
		t.Fatalf("missing Astra source evidence: %+v", rate)
	}
	// The same run ended with a timed-out audit with no completed usage event.
	missing := EstimateUsage(parseCodex("", "").Usage, "codex", "gpt-6-astra", "audit")
	summary := MergeUsageSummaries(UsageSummaryForResult(Result{OK: true, Usage: estimated, Processes: 1}), UsageSummaryForResult(Result{Usage: missing, Processes: 1}))
	if summary.EstimatedCostUSD == nil || math.Abs(*summary.EstimatedCostUSD-2.511248) > 1e-9 || summary.EstimateMissing != 1 || summary.FailedCalls != 1 || len(summary.EstimateMissingReasons) != 1 || summary.EstimateMissingReasons[0].Reason != "MISSING_USAGE" {
		t.Fatalf("timeout was priced as zero or subtotal lost: %+v", summary)
	}
	if len(estimated.CostEstimates[0].Notes) != 1 || estimated.CostEstimates[0].Notes[0] != "MISSING_CACHE_WRITE_ASSUMED_ZERO" {
		t.Fatalf("missing cache writes were not qualified: %+v", estimated)
	}
}

func TestAstraPricingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, provider, model, body, reason string
	}{
		{"alias", "codex", "gpt-6-astra-latest", `{"input_tokens":1,"output_tokens":1}`, "UNKNOWN_MODEL"},
		{"provider", "claude", "gpt-6-astra", `{"input_tokens":1,"output_tokens":1}`, "UNKNOWN_MODEL"},
		{"invalid-cache", "codex", "gpt-6-astra", `{"input_tokens":1,"cached_input_tokens":2,"output_tokens":1}`, "INVALID_USAGE"},
		{"missing-output", "codex", "gpt-6-astra", `{"input_tokens":1}`, "MISSING_USAGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := EstimateUsage(parseCodex(`{"type":"turn.completed","usage":`+tc.body+`}`, "").Usage, tc.provider, tc.model, "audit")
			if u.EstimatedCostUSD != nil || u.EstimateMissing != 1 || len(u.EstimateMissingReasons) != 1 || u.EstimateMissingReasons[0].Reason != tc.reason {
				t.Fatalf("incorrect missing-price classification: %+v", u)
			}
		})
	}
	price := 0.25
	u := EstimateUsage(Usage{CostUSD: &price}, "codex", "gpt-6-astra", "audit")
	if u.CostUSD == nil || *u.CostUSD != price || u.EstimatedCostUSD != nil || u.EstimateMissing != 0 {
		t.Fatalf("provider price was replaced: %+v", u)
	}
}
