package surface

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaultHelpDoesNotStartRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(nil, t.TempDir(), &stdout, &stderr, Callbacks{Run: func(Context) (RunResult, error) {
		t.Fatal("default command started run")
		return RunResult{}, nil
	}})
	if code != ExitOK || stdout.String() != Help || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if code := Execute([]string{"run", "--help"}, t.TempDir(), &stdout, &stderr, Callbacks{}); code != ExitOK || stdout.String() != Help {
		t.Fatal("command help must not start run", code, stderr.String())
	}
	for _, args := range [][]string{{"--repo", ""}, {"--provider", "codex"}, {"install", "/tmp"}, {"uninstall", "/tmp"}} {
		stdout.Reset()
		stderr.Reset()
		if code := Execute(args, t.TempDir(), &stdout, &stderr, Callbacks{}); code != ExitAborted {
			t.Fatalf("unexpected exit for %v: %d", args, code)
		}
	}
}

func TestRepoSelectionAndTargetBase(t *testing.T) {
	repo := testRepo(t)
	canonical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"app", "app/web", "other"} {
		if err := os.MkdirAll(filepath.Join(repo, path), 0755); err != nil {
			t.Fatal(err)
		}
	}
	caller := t.TempDir()
	relative, err := filepath.Rel(caller, repo)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(caller, "repo alias")
	if err := os.Symlink(repo, alias); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, cwd     string
		args, targets []string
	}{
		{"explicit", caller, []string{"doctor", "--repo", repo, "--target", "app/web"}, []string{"app/web"}},
		{"relative", caller, []string{"doctor", "--repo", relative, "--target", "app"}, []string{"app"}},
		{"symlink", caller, []string{"doctor", "--repo", alias}, []string{}},
		{"implicit", filepath.Join(repo, "app"), []string{"doctor", "--target", "web"}, []string{"app/web"}},
		{"multiple", caller, []string{"doctor", "--repo", repo, "--target", "app", "--target", "app/web", "--target", "other"}, []string{"app", "other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			var stdout, stderr bytes.Buffer
			args := append(append([]string{}, tc.args...), "--no-live-probe")
			code := Execute(args, tc.cwd, &stdout, &stderr, Callbacks{Doctor: func(c Context) (DoctorResult, error) {
				called = true
				if c.Repo != canonical || strings.Join(c.Targets, ",") != strings.Join(tc.targets, ",") {
					t.Fatalf("context=%+v", c)
				}
				return DoctorResult{OK: true}, nil
			}})
			if code != ExitOK || !called {
				t.Fatalf("exit=%d %s", code, stderr.String())
			}
		})
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repo, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"doctor", "--repo", outside}, {"doctor", "--repo", repo, "--target", "escape"}, {"doctor", "--repo", repo, "--target", "../outside"}} {
		var stdout, stderr bytes.Buffer
		if code := Execute(args, caller, &stdout, &stderr, Callbacks{}); code != ExitAborted {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestInitIsOptionalAndPreservesFiles(t *testing.T) {
	repo := testRepo(t)
	cfg, err := LoadConfig(repo)
	if err != nil || !reflect.DeepEqual(cfg, DefaultConfig()) {
		t.Fatal(cfg, err)
	}
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"init", "--repo", repo}, t.TempDir(), &stdout, &stderr, Callbacks{}); code != ExitOK {
		t.Fatal(stderr.String())
	}
	config := filepath.Join(repo, ".refactor", "config.json")
	custom := []byte(`{"schema_version":2,"policy":{"max_commits":1}}`)
	if err := os.WriteFile(config, custom, 0600); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(repo, ".refactor", "runs", "evidence.txt")
	if err := os.WriteFile(evidence, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Init(repo); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]byte{config: custom, evidence: []byte("keep")} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("changed %s: %v", path, err)
		}
	}
	for _, path := range []string{"bin", "lib", "skills"} {
		if _, err := os.Stat(filepath.Join(repo, ".refactor", path)); !os.IsNotExist(err) {
			t.Fatalf("unexpected %s", path)
		}
	}
}

func TestInitRejectsUnknownOrLinkedState(t *testing.T) {
	for _, target := range []string{".refactor", ".refactor/runs", ".refactor/config.json", ".refactor/.gitignore"} {
		t.Run(target, func(t *testing.T) {
			repo, outside := testRepo(t), t.TempDir()
			path := filepath.Join(repo, target)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
			if err := Init(repo); err == nil {
				t.Fatal("accepted linked state")
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatal("wrote outside repo", err)
			}
		})
	}
	repo := testRepo(t)
	if err := os.Mkdir(filepath.Join(repo, ".refactor"), 0755); err != nil {
		t.Fatal(err)
	}
	ignore := filepath.Join(repo, ".refactor", ".gitignore")
	if err := os.WriteFile(ignore, []byte("custom\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Init(repo); err == nil {
		t.Fatal("accepted custom ignore")
	}
	if _, err := os.Stat(filepath.Join(repo, ".refactor", "config.json")); !os.IsNotExist(err) {
		t.Fatal("wrote config on failure")
	}
}

func TestUnsupportedVersionsAreRejectedWithoutModification(t *testing.T) {
	repo := testRepo(t)
	if err := Init(repo); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, ".refactor", "config.json")
	for _, raw := range []string{`{}`, `null`, `{"schema_version":1}`, `{"schema_version":99}`} {
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(repo); err == nil {
			t.Fatalf("accepted config %s", raw)
		}
		if err := Init(repo); err == nil {
			t.Fatalf("init accepted config %s", raw)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != raw {
			t.Fatal("rewrote config", err)
		}
	}
	for _, raw := range []string{`{}`, `null`, `{"schemaVersion":2}`, `{"schemaVersion":99}`} {
		if _, err := RenderReport([]byte(raw), "en"); err == nil {
			t.Fatalf("accepted report %s", raw)
		}
	}
}

func TestKnownConfigValidationBeforeRunPreservesInput(t *testing.T) {
	repo := testRepo(t)
	if err := os.Mkdir(filepath.Join(repo, ".refactor"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, ".refactor", "config.json")
	cases := []struct{ fields, key string }{
		{`"workspace":null`, "workspace"},
		{`"agents":[]`, "agents"},
		{`"policy":"not-an-object"`, "policy"},
		{`"verification":false`, "verification"},
		{`"workspace":{"branch_prefix":1}`, "workspace.branch_prefix"},
		{`"workspace":{"worktree_parent":false}`, "workspace.worktree_parent"},
		{`"workspace":{"keep_worktree":"false"}`, "workspace.keep_worktree"},
		{`"agents":{"primary":false}`, "agents.primary"},
		{`"agents":{"fallback":null}`, "agents.fallback"},
		{`"agents":{"codex":"bin"}`, "agents.codex"},
		{`"agents":{"claude":{"bin":[]}}`, "agents.claude.bin"},
		{`"agents":{"codex":{"model":3}}`, "agents.codex.model"},
		{`"agents":{"claude":{"enabled":"true"}}`, "agents.claude.enabled"},
		{`"agents":{"codex":{"effort":{}}}`, "agents.codex.effort"},
		{`"agents":{"codex":{"effort_by_phase":null}}`, "agents.codex.effort_by_phase"},
		{`"agents":{"claude":{"effort_by_phase":{"audit":false}}}`, "agents.claude.effort_by_phase.audit"},
		{`"agents":{"claude":{"max_budget_usd":-1}}`, "agents.claude.max_budget_usd"},
		{`"agents":{"claude":{"max_budget_usd":-1e-1000000000}}`, "agents.claude.max_budget_usd"},
		{`"agents":{"claude":{"max_budget_usd":1e-1000000000}}`, "agents.claude.max_budget_usd"},
		{`"agents":{"claude":{"max_budget_usd":"0"}}`, "agents.claude.max_budget_usd"},
		{`"policy":{"auto_characterization":"false"}`, "policy.auto_characterization"},
		{`"policy":{"cross_provider_review":0}`, "policy.cross_provider_review"},
		{`"policy":{"unknown_risk":false}`, "policy.unknown_risk"},
		{`"policy":{"unknown_risk":"unsupported"}`, "policy.unknown_risk"},
		{`"policy":{"allowed_risks":{}}`, "policy.allowed_risks"},
		{`"policy":{"allowed_risks":["L0_LOW",false]}`, "policy.allowed_risks[1]"},
		{`"policy":{"extra_forbidden_globs":[1]}`, "policy.extra_forbidden_globs[0]"},
		{`"policy":{"max_cycles":"1"}`, "policy.max_cycles"},
		{`"policy":{"max_wall_clock_min":null}`, "policy.max_wall_clock_min"},
		{`"policy":{"max_audit_candidates":1.5}`, "policy.max_audit_candidates"},
		{`"policy":{"max_audit_candidates":1e-1000000000}`, "policy.max_audit_candidates"},
		{`"policy":{"max_wall_clock_min":180.00000000000000001}`, "policy.max_wall_clock_min"},
		{`"policy":{"max_cycles":9007199254740993}`, "policy.max_cycles"},
		{`"policy":{"max_cycles":9223372036854775808}`, "policy.max_cycles"},
		{`"verification":{"locked":"false"}`, "verification.locked"},
		{`"verification":{"commands":{}}`, "verification.commands"},
		{`"schema_version":2.00000000000000001`, "schema_version"},
	}
	for _, key := range []string{"max_cycles", "max_commits", "max_consecutive_failures", "max_attempts_per_fingerprint", "empty_audits_to_stop", "max_files_per_candidate", "max_changed_lines", "max_audit_candidates", "max_wall_clock_min", "cooldown_minutes"} {
		cases = append(cases, struct{ fields, key string }{fmt.Sprintf(`"policy":{%q:-1}`, key), "policy." + key})
	}
	for _, key := range []string{"max_cycles", "max_commits", "empty_audits_to_stop", "max_wall_clock_min"} {
		cases = append(cases, struct{ fields, key string }{fmt.Sprintf(`"policy":{%q:0}`, key), "policy." + key})
	}
	cases = append(cases,
		struct{ fields, key string }{fmt.Sprintf(`"policy":{"max_wall_clock_min":%d}`, maxRuntimeMinutes+1), "policy.max_wall_clock_min"},
		struct{ fields, key string }{fmt.Sprintf(`"policy":{"cooldown_minutes":%d}`, maxRuntimeMinutes+1), "policy.cooldown_minutes"},
		struct{ fields, key string }{fmt.Sprintf(`"agents":{"codex":{"timeout_sec":%d}}`, int64(1<<63-1)/int64(time.Second)+1), "agents.codex.timeout_sec"},
	)
	for _, tc := range cases {
		t.Run(tc.key+tc.fields, func(t *testing.T) {
			raw := []byte(`{"schema_version":2,` + tc.fields + `}`)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadConfig(repo)
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("expected %s error, got %v", tc.key, err)
			}
			var stdout, stderr bytes.Buffer
			code := Execute([]string{"run", "--model", "fixture-model", "--fallback", "none", "--json"}, repo, &stdout, &stderr, Callbacks{Run: func(Context) (RunResult, error) {
				t.Fatal("malformed configuration reached run preparation")
				return RunResult{}, nil
			}})
			if code != ExitAborted || !strings.Contains(stderr.String(), tc.key) {
				t.Fatal("wrong CLI error", code, stderr.String())
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(raw, got) {
				t.Fatal("configuration changed", err)
			}
			for _, name := range []string{"runs", "lock.json", "last-run.json"} {
				if _, err := os.Stat(filepath.Join(repo, ".refactor", name)); !os.IsNotExist(err) {
					t.Fatal("created run state before validation", name, err)
				}
			}
		})
	}
}

func TestKnownConfigValidationKeepsZeroDefaultsAndExtensions(t *testing.T) {
	repo := testRepo(t)
	if err := os.Mkdir(filepath.Join(repo, ".refactor"), 0700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"schema_version":2,"workspace":{"future":{"mode":true}},"agents":{"codex":{"timeout_sec":0,"future":[1]},"claude":{"max_budget_usd":0,"effort_by_phase":{"future":"custom"}}},"policy":{"allowed_risks":["L3_CRITICAL","UNKNOWN"],"max_cycles":1e2,"max_consecutive_failures":0,"max_attempts_per_fingerprint":0,"max_files_per_candidate":0,"max_changed_lines":0,"max_audit_candidates":0,"cooldown_minutes":0,"future":false},"verification":{"future":true},"future":null}`)
	path := filepath.Join(repo, ".refactor", "config.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if object(cfg["policy"])["max_cycles"] != float64(100) || object(cfg["policy"])["max_commits"] != float64(20) || object(cfg["policy"])["future"] != false || object(object(cfg["workspace"])["future"])["mode"] != true || ConfigString(cfg, "agents", "claude", "effort_by_phase", "future") != "custom" {
		t.Fatal("valid settings/defaults/extensions changed", cfg)
	}
	for _, key := range []string{"max_consecutive_failures", "max_attempts_per_fingerprint", "max_files_per_candidate", "max_changed_lines", "max_audit_candidates", "cooldown_minutes"} {
		if object(cfg["policy"])[key] != float64(0) {
			t.Fatal("lost zero policy value", key)
		}
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(raw, got) {
		t.Fatal("configuration changed", err)
	}
}

func TestRunPointerIsVersionedAndProjectScoped(t *testing.T) {
	first, second := testRepo(t), testRepo(t)
	for _, repo := range []string{first, second} {
		if err := Init(repo); err != nil {
			t.Fatal(err)
		}
		run := filepath.Join(repo, ".refactor", "runs", "r")
		if err := os.Mkdir(run, 0755); err != nil {
			t.Fatal(err)
		}
		if err := writeLastRun(repo, RunResult{RunID: "r", RunDir: run, Status: "DONE"}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(repo, ".refactor", "last-run.json"))
		if err != nil {
			t.Fatal(err)
		}
		var pointer map[string]any
		if err := json.Unmarshal(data, &pointer); err != nil {
			t.Fatal(err)
		}
		if pointer["schemaVersion"] != float64(LastRunSchemaVersion) || pointer["runDir"] != run {
			t.Fatal(pointer)
		}
		if err := os.WriteFile(filepath.Join(run, "report.json"), []byte(`{"schemaVersion":3,"status":"DONE"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	foreign := filepath.Join(second, ".refactor", "runs", "r")
	if err := writeLastRun(first, RunResult{RunID: "r", RunDir: foreign}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"report", "--json", "--repo", first}, t.TempDir(), &stdout, &stderr, Callbacks{}); code != ExitAborted || stdout.Len() != 0 {
		t.Fatal("foreign report read", code, stdout.String())
	}
}

func TestRawReportRejectsUnsupportedAndMalformedJSON(t *testing.T) {
	repo := testRepo(t)
	if err := Init(repo); err != nil {
		t.Fatal(err)
	}
	run := filepath.Join(repo, ".refactor", "runs", "r")
	if err := os.Mkdir(run, 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeLastRun(repo, RunResult{RunID: "r", RunDir: run}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(run, "report.json")
	for _, raw := range []string{`{"schemaVersion":2,"status":"DONE"}`, `null`, `{broken`} {
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if code := Execute([]string{"report", "--json"}, repo, &stdout, &stderr, Callbacks{}); code != ExitAborted || stdout.Len() != 0 {
			t.Fatalf("accepted invalid report %q", raw)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != raw {
			t.Fatal("changed invalid evidence", err)
		}
	}
}
