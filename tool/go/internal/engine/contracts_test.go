package engine

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

func TestContractProviderRing(t *testing.T) {
	fresh := func() *RingState {
		return &RingState{Providers: NewProviderRing([]string{"claude", "codex"}), ProviderOrder: []string{"codex", "claude"}}
	}
	s := fresh()
	if PickProvider(s, "", "") != "codex" {
		t.Fatal("operator order lost")
	}
	s.ProviderOrder = []string{"claude"}
	if PickProvider(s, "codex", "") != "claude" {
		t.Fatal("excluded provider selected")
	}
	s = fresh()
	NoteFailure(s, "codex", Result{Failure: "QUOTA", Hard: true}, time.Minute)
	if got := ReadyProviders(s, time.Now()); !reflect.DeepEqual(got, []string{"claude"}) {
		t.Fatal(got)
	}
	if got := ReadyProviders(s, time.Now().Add(2*time.Minute)); !reflect.DeepEqual(got, []string{"codex", "claude"}) {
		t.Fatal(got)
	}
	NoteFailure(s, "codex", Result{Failure: "QUOTA"}, time.Minute)
	if s.Providers["codex"].Status != "READY" {
		t.Fatal("soft quota removed provider")
	}
	NoteFailure(s, "codex", Result{Failure: "AUTH"}, time.Minute)
	if s.Providers["codex"].Status != "DEAD" {
		t.Fatal("auth not permanent")
	}
	if PickReviewer(s, "claude") != "claude" {
		t.Fatal("single provider review")
	}
	s = fresh()
	if PickReviewer(s, "claude") != "codex" {
		t.Fatal("review independence")
	}
}

func TestContractProviderParsing(t *testing.T) {
	if got := LastJSONObject("banner\n{\"old\":1}\n{\"message\":\"brace } and \\\" quote\"}"); got["message"] != "brace } and \" quote" {
		t.Fatal(got)
	}
	if got := StripFences("```json\n{\"ok\":true}\n```"); got != `{"ok":true}` {
		t.Fatal(got)
	}
	for _, tc := range []struct {
		name, body, want string
		hard             bool
	}{
		{"success-subtype", `{"type":"result","subtype":"error","is_error":false,"structured_output":{}}`, "OK", false},
		{"hard-quota", `{"type":"result","is_error":true,"result":"usage limit reached"}`, "QUOTA", true},
		{"soft-quota", `{"type":"result","is_error":true,"result":"rate limit"}`, "QUOTA", false},
		{"schema-exhausted", `{"type":"result","is_error":true,"subtype":"error_max_structured_output_retries","result":"failed"}`, "SCHEMA", false},
		{"missing-structured", `{"type":"result","is_error":false,"result":"plain text"}`, "SCHEMA", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyClaude(processResult{stdout: tc.body}, true)
			if got.failure != tc.want || got.hard != tc.hard {
				t.Fatalf("%+v", got)
			}
		})
	}
	for _, message := range []string{"usage limit reached", "invalid api key"} {
		one := 1
		got := classifyCodex(processResult{stderr: message, exitCode: &one}, "", true)
		want := "QUOTA"
		if strings.Contains(message, "key") {
			want = "AUTH"
		}
		if got.failure != want {
			t.Fatal(got)
		}
	}
	if classifyClaude(processResult{killed: true}, true).failure != "TIMEOUT" || classifyCodex(processResult{killed: true}, "", true).failure != "TIMEOUT" {
		t.Fatal("timeout parsed output")
	}
}

func TestContractUsageAccounting(t *testing.T) {
	a := Usage{InputTokens: 10, OutputTokens: 1}
	b := Usage{InputTokens: 20, OutputTokens: 2}
	m := MergeUsage(a, b)
	if m.CostUSD != nil || m.CostMissing != 2 || m.InputTokens != 30 {
		t.Fatal(m)
	}
	cost := 0.3
	a.CostUSD = &cost
	m = MergeUsage(a, b)
	if m.CostUSD == nil || *m.CostUSD != cost || m.CostMissing != 1 {
		t.Fatal(m)
	}
	s := &RingState{Providers: NewProviderRing([]string{"codex"})}
	NoteUsage(s, "codex", "audit", Result{Usage: b, Processes: 2, DurationMS: 8000})
	got := s.Usage.ByPhase["audit"]
	if got.Calls != 1 || got.Processes != 2 || got.CostMissing != 2 || got.FailedCalls != 1 || got.MS != 8000 {
		t.Fatal(got)
	}
	if s.Providers["codex"].Usage.InputTokens != 20 {
		t.Fatal("provider bucket lost")
	}
	p := parseClaude(`{"type":"result","is_error":false,"structured_output":{},"usage":{"input_tokens":18,"cache_creation_input_tokens":42886,"cache_read_input_tokens":249075,"output_tokens":14146},"total_cost_usd":0.5488}`)
	if p.Usage.InputTokens != 291979 || p.Usage.OutputTokens != 14146 || p.Usage.CostUSD == nil || *p.Usage.CostUSD != 0.5488 {
		t.Fatal(p)
	}
	if parseClaude(`{"type":"result","usage":{"input_tokens":5}}`).Usage.CostUSD != nil {
		t.Fatal("unreported cost became zero")
	}
	path := filepath.Join(t.TempDir(), "answer.json")
	if err := os.WriteFile(path, []byte(`{"ok":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := parseCodex("{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":900,\"output_tokens\":40}}", path)
	if c.Usage.InputTokens != 900 || c.Usage.OutputTokens != 40 || c.Usage.CostUSD != nil {
		t.Fatal(c)
	}
	if FmtTokens(412) != "412" || FmtTokens(38200) != "38.2k" || FmtTokens(812400) != "812k" || FmtMS(372000) != "6m12s" {
		t.Fatal("format changed")
	}
	if !strings.Contains(UsageLine("audit", "codex", Result{}), "cost n/a") {
		t.Fatal("unknown cost rendered zero")
	}
}

func TestContractUsagePricing(t *testing.T) {
	codexOutput := `{"type":"turn.completed","usage":{"input_tokens":1000000,"cached_input_tokens":600000,"cache_write_input_tokens":100000,"output_tokens":100000,"reasoning_output_tokens":20000}}`
	codex := parseCodex(codexOutput, "").Usage
	claudeOutput := `{"type":"result","usage":{"input_tokens":300000,"cache_read_input_tokens":600000,"cache_creation_input_tokens":100000,"output_tokens":100000}}`
	claude := parseClaude(claudeOutput).Usage
	for _, tc := range []struct {
		provider, model string
		usage           Usage
		want            float64
	}{
		{"codex", "gpt-6.1-sol", codex, 1.91},
		{"codex", "gpt-5.6-sol", codex, 3.94},
		{"claude", "claude-sonnet-5-5", claude, 1.97},
		{"claude", "claude-sonnet-5-5", parseClaude(`{"type":"result","usage":{"input_tokens":300000,"cache_read_input_tokens":600000,"cache_creation_input_tokens":100000,"cache_creation":{"ephemeral_5m_input_tokens":100000},"output_tokens":100000}}`).Usage, 1.97},
		{"claude", "claude-sonnet-5-5", parseClaude(`{"type":"result","usage":{"input_tokens":300000,"cache_read_input_tokens":600000,"cache_creation_input_tokens":100000,"cache_creation":{"ephemeral_1h_input_tokens":50000},"output_tokens":100000}}`).Usage, 2.045},
	} {
		t.Run(tc.model, func(t *testing.T) {
			got := EstimateUsage(tc.usage, tc.provider, tc.model, "audit")
			if got.CostUSD != nil || got.EstimatedCostUSD == nil || math.Abs(*got.EstimatedCostUSD-tc.want) > 1e-9 || len(got.CostEstimates) != 1 || got.EstimateMissing != 0 {
				t.Fatalf("incorrect estimate: %+v", got)
			}
			detail := got.CostEstimates[0]
			if detail.Rates.CheckedAt != "2026-10-07" || !strings.HasPrefix(detail.Rates.SourceURL, "https://artificialanalysis.ai/") || detail.Rates.SupplementalSourceURL == "" || detail.RequestedModel != tc.model || detail.Assumption == "" {
				t.Fatalf("missing price provenance: %+v", detail)
			}
			if got.InputTokens != 1000000 || got.UncachedInputTokens != 300000 {
				t.Fatalf("cache categories were double counted: %+v", got)
			}
		})
	}
	reported := parseClaude(`{"type":"result","usage":{"input_tokens":1,"output_tokens":1},"total_cost_usd":0.5488}`).Usage
	if got := EstimateUsage(reported, "claude", "claude-sonnet-5-5", "review"); got.EstimatedCostUSD != nil || *got.CostUSD != 0.5488 || len(got.CostEstimates) != 0 {
		t.Fatal("provider-reported price was replaced", got)
	}
	invalidPrice := EstimateUsage(parseClaude(`{"type":"result","usage":{"input_tokens":1000000,"output_tokens":0},"total_cost_usd":-1}`).Usage, "claude", "claude-sonnet-5-5", "audit")
	if invalidPrice.CostUSD != nil || invalidPrice.EstimatedCostUSD == nil || *invalidPrice.EstimatedCostUSD != 2 || invalidPrice.CostEstimates[0].Notes[0] != "INVALID_REPORTED_COST_IGNORED" {
		t.Fatal("invalid provider price bypassed token estimate", invalidPrice)
	}
	for _, tc := range []struct {
		model, body, reason string
	}{
		{"custom-model", codexOutput, "UNKNOWN_MODEL"},
		{"gpt-6.1-sol", `{}`, "MISSING_USAGE"},
		{"gpt-6.1-sol", `{"type":"turn.completed","usage":{"input_tokens":5,"output_tokens":1,"cached_input_tokens":6}}`, "INVALID_USAGE"},
		{"gpt-6.1-sol", `{"type":"turn.completed","usage":{"input_tokens":-1,"output_tokens":1}}`, "INVALID_USAGE"},
	} {
		got := EstimateUsage(parseCodex(tc.body, "").Usage, "codex", tc.model, "execute")
		if got.EstimatedCostUSD != nil || got.EstimateMissing != 1 || len(got.EstimateMissingReasons) != 1 || got.EstimateMissingReasons[0].Reason != tc.reason {
			t.Fatal("unknown usage became a zero estimate", got)
		}
	}
	withoutCache := EstimateUsage(parseCodex(`{"type":"turn.completed","usage":{"input_tokens":1000000,"output_tokens":0}}`, "").Usage, "codex", "gpt-6.1-sol", "audit")
	if withoutCache.EstimatedCostUSD == nil || *withoutCache.EstimatedCostUSD != 2 || len(withoutCache.CostEstimates[0].Notes) != 2 {
		t.Fatal("missing cache counts were not qualified", withoutCache)
	}
	estimated := EstimateUsage(codex, "codex", "gpt-6.1-sol", "audit")
	merged := MergeUsage(estimated, reported)
	summary := UsageSummaryForResult(Result{OK: true, Usage: merged, Processes: 2})
	missing := EstimateUsage(Usage{}, "codex", "gpt-6.1-sol", "execute")
	summary = MergeUsageSummaries(summary, UsageSummaryForResult(Result{Usage: missing, Processes: 1}))
	if summary.Processes != 3 || summary.Calls != 2 || summary.FailedCalls != 1 || summary.CostMissing != 2 || summary.EstimateMissing != 1 || len(summary.CostEstimates) != 1 || len(summary.EstimateMissingReasons) != 1 || *summary.CostUSD != 0.5488 || math.Abs(*summary.EstimatedCostUSD-1.91) > 1e-9 {
		t.Fatalf("repair/provider summaries lost usage: %+v", summary)
	}
	one := 1
	failed := classifyCodex(processResult{stdout: codexOutput, stderr: "invalid api key", exitCode: &one}, "", false)
	if failed.failure != "AUTH" || failed.parsed.Usage.InputTokens != 1000000 {
		t.Fatal("failure classification discarded reported usage", failed)
	}
	killed := classifyClaude(processResult{stdout: `{"type":"result","total_cost_usd":0.5488}`, killed: true}, false)
	if killed.failure != "TIMEOUT" || killed.parsed.Usage.CostUSD == nil || *killed.parsed.Usage.CostUSD != 0.5488 {
		t.Fatal("timeout discarded provider price", killed)
	}
}

func TestContractProviderTranscriptFailure(t *testing.T) {
	for _, scenario := range []string{"claude", "codex", "unsafe-claude"} {
		t.Run(scenario, func(t *testing.T) {
			provider := strings.TrimPrefix(scenario, "unsafe-")
			repo, runDir := t.TempDir(), t.TempDir()
			model := "gpt-6.1-sol"
			body := "#!/bin/sh\ncat >/dev/null\n"
			cfg := Config{Agents: map[string]AgentConfig{}}
			if scenario == "unsafe-claude" {
				c, fixtureRepo := catalogFixture(t)
				cfg.Skills, repo = c, fixtureRepo
				body += "echo changed >> '" + filepath.Join(c.Entries[0].Path, "SKILL.md") + "'\n"
			}
			if provider == "claude" {
				model = "claude-sonnet-5-5"
				body += "printf '%s\\n' '{\"type\":\"result\",\"is_error\":false,\"usage\":{\"input_tokens\":1000000,\"output_tokens\":0},\"total_cost_usd\":0.25}'\n"
			} else {
				body += "printf '%s\\n' '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1000000,\"output_tokens\":0}}'\n"
			}
			bin := filepath.Join(t.TempDir(), "provider")
			if err := os.WriteFile(bin, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			cfg.Agents[provider] = AgentConfig{Bin: bin, Model: model}
			// A directory at the stdout destination fails after the process ran,
			// independently of OS permission handling and the current user.
			rawPath := filepath.Join(runDir, "provider", "audit-"+provider+"-primary.stdout.jsonl")
			if err := os.MkdirAll(rawPath, 0700); err != nil {
				t.Fatal(err)
			}
			log := &providerEventRecorder{}
			got, err := CallProvider(context.Background(), provider, Request{Phase: "audit", CWD: repo, RunDir: runDir}, cfg, log)
			if err == nil || got.OK || got.Processes != 1 || got.Usage.InputTokens != 1000000 || !strings.Contains(err.Error(), "write stdout transcript") {
				t.Fatalf("transcript failure lost invocation: %+v, %v", got, err)
			}
			if provider == "claude" && (got.Usage.CostUSD == nil || *got.Usage.CostUSD != 0.25) || provider == "codex" && (got.Usage.EstimatedCostUSD == nil || *got.Usage.EstimatedCostUSD != 2) {
				t.Fatal("transcript failure lost cost", got.Usage)
			}
			if scenario == "unsafe-claude" && (!errors.Is(err, workspace.ErrUnsafe) || got.Failure != "UNSAFE" || !got.Fatal) {
				t.Fatal("transcript failure hid safety failure", got, err)
			}
			last := log.events[len(log.events)-1]
			if last.Action != "provider" || last.Status != "failed" {
				t.Fatal("provider completion concealed transcript failure", last)
			}
		})
	}
}

func TestContractAuditHistory(t *testing.T) {
	for phase, want := range map[string]string{"audit": "high", "deep_check": "high", "preflight": "medium", "characterization": "medium", "execute": "high", "review": "high", "handoff": "low", "doctor": "low"} {
		if got := EffortFor(phase, AgentConfig{}); got != want {
			t.Fatal(phase, got)
		}
	}
	if EffortFor("audit", AgentConfig{Effort: "low", EffortByPhase: map[string]string{"audit": "medium"}}) != "medium" {
		t.Fatal("phase override precedence")
	}

	for _, tc := range []struct {
		name         string
		incremental  bool
		committed    int
		want, absent string
	}{
		{"initial", false, 0, "", "PRIOR_WORK"}, {"uncommitted", true, 0, "No earlier cycle of this run landed a commit", "ALREADY CONTAINS"}, {"continued", true, 2, "landed 2 commit(s)", "No earlier cycle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := BuildPrompt("audit", PromptArgs{Provider: "codex", Incremental: tc.incremental, Committed: tc.committed, Cycle: 2})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body, tc.want) || strings.Contains(body, tc.absent) || strings.Contains(body, "must not touch") {
				t.Fatal(body)
			}
		})
	}
	body, err := BuildPrompt("audit", PromptArgs{Provider: "claude", Incremental: true, Violated: []string{"pnpm-lock.yaml"}})
	if err != nil || !strings.Contains(body, "must not touch: pnpm-lock.yaml") {
		t.Fatal(body, err)
	}
	for _, phase := range []string{"audit", "deep_check", "preflight", "review", "doctor", "unknown"} {
		if ModeFor(phase) != "read" {
			t.Fatal(phase)
		}
	}
	for _, phase := range []string{"execute", "characterization"} {
		if ModeFor(phase) != "write" {
			t.Fatal(phase)
		}
	}
	for _, phase := range []string{"audit", "deep_check", "execute", "review"} {
		if EffortFor(phase, AgentConfig{Effort: "low"}) != "low" {
			t.Fatal("configured effort ignored")
		}
	}
}

func TestContractVerdictEnforcers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result map[string]any
		want   string
	}{
		{"unexplained", map[string]any{"verdict": "PASS", "hunks": []any{map[string]any{"classification": "UNEXPLAINED"}}}, "FAIL"},
		{"contract-change", map[string]any{"verdict": "PASS", "hunks": []any{map[string]any{"classification": "CONTRACT_CHANGING"}}}, "FAIL"},
		{"scope", map[string]any{"verdict": "PASS", "scope_expansion_required": true}, "FAIL"},
		{"delete-outside", map[string]any{"verdict": "PASS", "deleted_files": []any{"outside.go"}}, "FAIL"},
		{"pass", map[string]any{"verdict": "PASS", "changed_files": []any{"a.go"}}, "PASS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := EnforceExecutionVerdict(tc.result, map[string]any{"category": "DEAD_CODE", "allowlist": []any{"a.go"}})
			if got != tc.want {
				t.Fatal(got)
			}
		})
	}
	if got, _ := EnforcePreflightVerdict(map[string]any{"verdict": "CLEARED", "falsification_result": "UNKNOWN"}); got != "BLOCKED" {
		t.Fatal(got)
	}
	if got, _ := EnforceReviewVerdict(map[string]any{"verdict": "PASS", "behavior_preservation_assessment": "PRESERVED", "findings": []any{map[string]any{"severity": "BLOCKER"}}}); got != "FAIL" {
		t.Fatal(got)
	}
	if got, _ := EnforceCharacterizationVerdict(map[string]any{"verdict": "PASS", "production_source_changed": true}); got != "FAIL" {
		t.Fatal(got)
	}
	for _, phase := range []string{"audit", "deepcheck", "execute", "review", "preflight", "characterization"} {
		schema := Schema(phase)
		if schema == nil || len(Validate(schema, map[string]any{"unexpected": true})) == 0 {
			t.Fatal(phase)
		}
	}
}

func TestContractExternalSurface(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{{"package.json", `{"private":true}`, "NONE_DETECTED"}, {"package.json", `{"private":true,"exports":"./index.js"}`, "POSSIBLE"}, {"go.mod", "module github.com/example/fixture\n", "POSSIBLE"}, {".github/workflows/publish.yml", "run: npm publish\n", "POSSIBLE"}} {
		t.Run(tc.name+tc.want, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, tc.name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			if got := ExternalSurface(root); got.Verdict != tc.want {
				t.Fatal(got)
			}
		})
	}
}

func TestContractPromptSafety(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		prompt, err := BuildPrompt("audit", PromptArgs{Provider: provider})
		if err != nil {
			t.Fatal(err)
		}
		flat := strings.Join(strings.Fields(prompt), " ")
		for _, want := range []string{"hypothetical new caller", "reachable from an entrypoint that exists in this repository TODAY", "module that nothing in the repository imports", "shape of a stack trace", "internal helper frame", "does not soften the evidence requirements", "Invoke ONLY the skills named in the SKILLS line", "Do not invoke skills that are not listed for your phase", "The loop owns git"} {
			if !strings.Contains(flat, want) {
				t.Fatalf("missing %q", want)
			}
		}
		for _, phase := range []string{"execute", "characterization"} {
			prompt, err := BuildPrompt(phase, PromptArgs{Provider: provider, Packet: map[string]any{"category": "DEAD_CODE"}})
			if err != nil {
				t.Fatal(err)
			}
			if phase == "execute" {
				flat := strings.Join(strings.Fields(prompt), " ")
				for _, want := range []string{"list its path in `deleted_files`", "do not report FAIL because you lack a deletion primitive", "Do not blank a file's contents as a substitute"} {
					if !strings.Contains(flat, want) {
						t.Fatal(want)
					}
				}
			}
			if !strings.Contains(strings.ToLower(prompt), "build output") {
				t.Fatalf("%s build-output boundary absent", phase)
			}
		}
	}
	for _, mode := range []string{"read", "write"} {
		args := strings.Join(ClaudeArgs(mode, nil, "", "", 0), " ")
		for _, flag := range []string{"--safe-mode", "--bare", "--disable-slash-commands", "--add-dir"} {
			if strings.Contains(args, flag) {
				t.Fatal(flag, args)
			}
		}
		if !strings.Contains(args, "Skill,") || strings.Contains(args, "--bare") || strings.Contains(args, "--dangerously-skip-permissions") {
			t.Fatal(args)
		}
	}
	for _, mode := range []string{"read", "write"} {
		args := strings.Join(CodexArgs(mode, "/w", "/schema", "/out", "", "high"), " ")
		if !strings.Contains(args, `model_reasoning_effort="high"`) {
			t.Fatal(args)
		}
		want := "read-only"
		if mode == "write" {
			want = "workspace-write"
		}
		if !strings.Contains(args, want) {
			t.Fatal(args)
		}
	}
}

func TestContractWorkflowExamples(t *testing.T) {
	re := regexp.MustCompile("(?s)<!-- example: ([\\w-]+) schema: (\\w+) -->\n```json\n(.*?)\n```")
	read := func(name string) map[string]map[string]any {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", name))
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]map[string]any{}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			var value map[string]any
			if err := json.Unmarshal([]byte(m[3]), &value); err != nil {
				t.Fatal(err)
			}
			if errors := Validate(Schema(m[2]), value); len(errors) > 0 {
				t.Fatal(m[1], errors)
			}
			out[m[1]] = value
		}
		return out
	}
	en, ko := read("WORKFLOW.md"), read("WORKFLOW.ko.md")
	if len(en) != 10 || !reflect.DeepEqual(en, ko) {
		t.Fatal("workflow examples diverged")
	}
	packet := en["deepcheck-success"]
	if got, _ := EnforceExecutionVerdict(en["execute-success"], packet); got != "PASS" {
		t.Fatal(got)
	}
	if got, _ := EnforcePreflightVerdict(en["preflight-success"]); got != "READY_TO_EXECUTE" {
		t.Fatal(got)
	}
	if got, _ := EnforcePreflightVerdict(en["preflight-blocked"]); got != "BLOCKED" {
		t.Fatal(got)
	}
	if got, _ := EnforceReviewVerdict(en["review-success"]); got != "PASS" {
		t.Fatal(got)
	}
	if got, _ := EnforceReviewVerdict(en["review-rejected"]); got != "FAIL" {
		t.Fatal(got)
	}
	if got, _ := EnforceCharacterizationVerdict(en["characterization-success"]); got != "PASS" {
		t.Fatal(got)
	}
}

func TestContractSkillCatalog(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Skills map[string]struct {
			Source string `json:"source"`
			Path   string `json:"skillPath"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(b, &lock); err != nil {
		t.Fatal(err)
	}
	if len(RequiredSkills) != 8 {
		t.Fatal(RequiredSkills)
	}
	seen := map[string]bool{}
	for _, name := range RequiredSkills {
		if seen[name] {
			t.Fatal("duplicate skill")
		}
		seen[name] = true
		entry := lock.Skills[name]
		if entry.Source != "soom-kang/sharpen-me" || entry.Path != "skills/"+name+"/SKILL.md" {
			t.Fatal(name, entry)
		}
		canonical := filepath.Join(root, ".agents", "skills", name)
		raw, err := os.ReadFile(filepath.Join(canonical, "SKILL.md"))
		if err != nil || !strings.Contains(string(raw), "name: "+name) {
			t.Fatal(name, err)
		}
		a, err := filepath.EvalSymlinks(canonical)
		if err != nil {
			t.Fatal(err)
		}
		b, err := filepath.EvalSymlinks(filepath.Join(root, ".claude", "skills", name))
		if err != nil || a != b {
			t.Fatal(name, a, b, err)
		}
	}
	for _, phase := range []string{"audit", "deep_check", "execute", "review", "preflight", "characterization", "handoff"} {
		body, err := BuildPrompt(phase, PromptArgs{Provider: "claude", Packet: map[string]any{"category": "DEDUPLICATION"}, Candidate: map[string]any{"category": "DEDUPLICATION"}})
		if err != nil {
			t.Fatal(err)
		}
		re := regexp.MustCompile(`/sharpen-[a-z-]+`)
		for _, match := range re.FindAllString(body, -1) {
			if !seen[strings.TrimPrefix(match, "/")] {
				t.Fatal("unprobed routed skill", match)
			}
		}
	}
}

func TestContractScopedInventory(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	for name, body := range map[string]string{"app/web/a.ts": "export const a = 1;\n", "app/api/a.go": strings.Repeat("// large\n", 501), "src/a.py": "x = 1\n"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	for _, tc := range []struct {
		targets         []string
		present, absent []string
		large           string
	}{{nil, []string{"app/web/a.ts", "app/api/a.go", "src/a.py"}, nil, "files at or over 500 lines: 1"}, {[]string{"app/web"}, []string{"app/web/a.ts", "The other 2 tracked source file(s)"}, []string{"app/api/a.go", "src/a.py"}, "files at or over 500 lines: none"}, {[]string{"app/web", "app/api"}, []string{"app/web/a.ts", "app/api/a.go"}, []string{"src/a.py"}, "files at or over 500 lines: 1"}} {
		got, err := CollectRepoFacts(root, FactsOptions{Targets: tc.targets})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range append(tc.present, tc.large) {
			if !strings.Contains(got, s) {
				t.Fatal(s, got)
			}
		}
		for _, s := range tc.absent {
			if strings.Contains(got, s) {
				t.Fatal(s, got)
			}
		}
	}
}
