package engine

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestMigrationProviderRing(t *testing.T) {
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

func TestMigrationProviderParsing(t *testing.T) {
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

func TestMigrationUsageAccounting(t *testing.T) {
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

func TestMigrationAuditHistory(t *testing.T) {
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

func TestMigrationVerdictEnforcers(t *testing.T) {
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

func TestMigrationExternalSurface(t *testing.T) {
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

func TestMigrationPromptSafety(t *testing.T) {
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

func TestMigrationWorkflowExamples(t *testing.T) {
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

func TestMigrationSkillCatalog(t *testing.T) {
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

func TestMigrationScopedInventory(t *testing.T) {
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
