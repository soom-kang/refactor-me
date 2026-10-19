package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

func newCandidateSafetyRunner(t *testing.T) *runner {
	t.Helper()
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package fixture\n\nfunc unused() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", "a.go")
	gitTest(t, repo, "commit", "-qm", "fixture")
	base := gitTest(t, repo, "rev-parse", "HEAD")
	wt := filepath.Join(t.TempDir(), "wt")
	if err := workspace.WorktreeAdd(repo, wt, base); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := workspace.SourceFingerprint(repo)
	if err != nil {
		t.Fatal(err)
	}
	p := contractPolicy()
	p.MaxWallClockMin, p.MaxCommits, p.MaxCycles = 10, 10, 10
	r := &runner{ctx: context.Background(), surface: surface.Context{Repo: repo, Stderr: io.Discard},
		wt: wt, runDir: t.TempDir(), sourceFingerprint: fingerprint, policy: p, started: time.Now(),
		state: newState("r", repo, wt, base, "main", "result", nil, []string{"codex"}), discovery: workspace.Discovery{Areas: []string{"."}},
		commands: []workspace.Command{{ID: "test", Area: ".", Tier: "T1", Name: "test", Argv: []string{"/usr/bin/true"}, Cwd: ".", TimeoutMS: 10000}}}
	r.baseline = workspace.RunBaseline(r.ctx, r.commands, wt, nil)
	// Both ends are TempDir fixtures; cleanup also covers the deliberately
	// retained dirty worktree without weakening WorktreeRemove's safety checks.
	t.Cleanup(r.endProgress)
	return r
}

func TestSourceInvariantBlocksPublicationAfterGate(t *testing.T) {
	for _, change := range []string{"none", "file", "head", "hook"} {
		t.Run(change, func(t *testing.T) {
			r := newCandidateSafetyRunner(t)
			gitTest(t, r.surface.Repo, "branch", r.state.BranchName, r.state.BaseOID)
			r.state.PublishedOID = r.state.BaseOID
			if err := os.WriteFile(filepath.Join(r.wt, "a.go"), []byte("package fixture\n\n"), 0600); err != nil {
				t.Fatal(err)
			}
			packet := map[string]any{"fp": "fp", "category": "DEAD_CODE", "allowlist": []string{"a.go"}, "title": "remove unused"}
			if ok, err := r.gate(packet, r.runDir, r.state.BaseOID); err != nil || !ok {
				t.Fatalf("valid change did not pass gate: %v, %v", ok, err)
			}
			if change == "file" {
				if err := os.WriteFile(filepath.Join(r.surface.Repo, "a.go"), []byte("user edit\n"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if change == "head" {
				gitTest(t, r.surface.Repo, "commit", "--allow-empty", "-qm", "user commit")
			} else if change == "hook" {
				hook := "#!/bin/sh\nprintf 'user edit\\n' > " + progressShellQuote(filepath.Join(r.surface.Repo, "a.go")) + "\n"
				if err := os.WriteFile(filepath.Join(r.surface.Repo, ".git", "hooks", "pre-commit"), []byte(hook), 0700); err != nil {
					t.Fatal(err)
				}
			}
			err := r.commit(packet, contractCandidate(nil), r.runDir)
			if change == "none" {
				if err != nil || r.state.Counters.Commits != 1 || r.state.PublishedOID == r.state.BaseOID {
					t.Fatalf("unchanged source blocked publication: %v", err)
				}
				return
			}
			var stopped halt
			if !errors.As(err, &stopped) || stopped.Status != "HALTED_UNSAFE" || !strings.Contains(stopped.Reason, "SOURCE_MUTATED") {
				t.Fatalf("source drift was not halted: %v", err)
			}
			if got := gitTest(t, r.surface.Repo, "rev-parse", "refs/heads/"+r.state.BranchName); got != r.state.BaseOID || r.state.Counters.Commits != 0 {
				t.Fatal("source drift changed the result ref or accepted state")
			}
			if raw, err := os.ReadFile(filepath.Join(r.wt, "a.go")); err != nil || string(raw) != "package fixture\n\n" {
				t.Fatal("unpublished worktree edit was lost", string(raw), err)
			}
			if change != "head" {
				if raw, err := os.ReadFile(filepath.Join(r.surface.Repo, "a.go")); err != nil || string(raw) != "user edit\n" {
					t.Fatal("user source edit was restored", string(raw), err)
				}
			}
		})
	}
}

func prepareCharacterizationProvider(t *testing.T, r *runner) map[string]any {
	t.Helper()
	const path = "a_test.go"
	answer := map[string]any{"schema_version": "1", "candidate_id": "c", "changed_files": []string{path}, "characterized_contracts": []string{"C1"},
		"production_source_changed": false, "assertions_weakened": false, "verdict": "PASS", "notes": "fixture"}
	fixture := filepath.Join(t.TempDir(), "answer.json")
	progressFixtureJSON(t, fixture, answer)
	bin := filepath.Join(t.TempDir(), "fake-codex")
	script := "#!/bin/sh\ncat >/dev/null\nout=''\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = '--output-last-message' ]; then shift; out=$1; fi\n shift\ndone\nprintf '%s\\n' 'package fixture' '// characterization evidence' > " + path + "\ncp " + progressShellQuote(fixture) + " \"$out\"\n"
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	r.engineConfig = engine.Config{Agents: map[string]engine.AgentConfig{"codex": {Bin: bin}}}
	return map[string]any{"fp": "fp", "candidate_id": "c", "category": "DEAD_CODE", "allowlist": []string{"a.go", path}, "characterization_files": []string{path}}
}

func TestCharacterizationSourceInvariantAfterValidation(t *testing.T) {
	r := newCandidateSafetyRunner(t)
	packet := prepareCharacterizationProvider(t, r)
	r.commands[0].Argv = []string{"/bin/sh", "-c", "printf 'user edit\\n' > " + progressShellQuote(filepath.Join(r.surface.Repo, "a.go"))}
	ok, err := r.characterize(packet, r.runDir, r.state.BaseOID)
	var stopped halt
	if ok || !errors.As(err, &stopped) || stopped.Status != "HALTED_UNSAFE" || !strings.Contains(stopped.Reason, "SOURCE_MUTATED") {
		t.Fatalf("characterization source drift was not halted: %v, %v", ok, err)
	}
	if workspace.RefExists(r.surface.Repo, "refs/heads/"+r.state.BranchName) || gitTest(t, r.wt, "rev-parse", "HEAD") != r.state.BaseOID {
		t.Fatal("characterization drift was committed or published")
	}
	if _, err := os.Stat(filepath.Join(r.wt, "a_test.go")); err != nil {
		t.Fatal("unfinished characterization was lost", err)
	}
	if raw, err := os.ReadFile(filepath.Join(r.surface.Repo, "a.go")); err != nil || string(raw) != "user edit\n" {
		t.Fatal("source drift was restored", string(raw), err)
	}
}

func TestCancelledValidationPreservesUnfinishedEdits(t *testing.T) {
	for _, phase := range []string{"gate", "characterization", "regression"} {
		t.Run(phase, func(t *testing.T) {
			r := newCandidateSafetyRunner(t)
			packet := map[string]any{"fp": "fp", "category": "DEAD_CODE", "allowlist": []string{"a.go"}}
			path, evidence := "a.go", "validation.json"
			if phase == "characterization" {
				packet = prepareCharacterizationProvider(t, r)
				path, evidence = "a_test.go", "characterization-validation.json"
			} else if err := os.WriteFile(filepath.Join(r.wt, path), []byte("package fixture\n\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if phase == "regression" {
				r.commands[0].Argv = []string{"/usr/bin/false"}
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				r.ctx = ctx
				marker := filepath.Join(t.TempDir(), "started")
				r.commands[0].Argv = []string{"/bin/sh", "-c", "touch " + progressShellQuote(marker) + "; sleep 30"}
				go func() {
					ticker := time.NewTicker(5 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							if _, err := os.Stat(marker); err == nil {
								cancel()
								return
							}
						}
					}
				}()
			}
			var ok bool
			var err error
			if phase == "characterization" {
				ok, err = r.characterize(packet, r.runDir, r.state.BaseOID)
			} else {
				ok, err = r.gate(packet, r.runDir, r.state.BaseOID)
			}
			if phase == "regression" {
				if err != nil || ok || len(r.state.Seen.Skipped) != 1 || r.state.Seen.Skipped[0].Reason != "REGRESSION" || gitTest(t, r.wt, "status", "--porcelain") != "" {
					t.Fatalf("genuine regression was not rolled back: %v, %v, %+v", ok, err, r.state.Seen.Skipped)
				}
				return
			}
			if ok || !errors.Is(err, context.Canceled) || len(r.state.Seen.Skipped) != 0 || r.state.Counters.ConsecutiveFailures != 0 {
				t.Fatalf("cancelled validation was counted as a failure: %v, %v, %+v", ok, err, r.state.Counters)
			}
			if gitTest(t, r.wt, "status", "--porcelain") == "" {
				t.Fatal("cancelled validation discarded unfinished edits")
			}
			if _, err := os.Stat(filepath.Join(r.wt, path)); err != nil {
				t.Fatal("unfinished file was lost", err)
			}
			raw, err := os.ReadFile(filepath.Join(r.runDir, evidence))
			var ladder workspace.Ladder
			if err != nil || json.Unmarshal(raw, &ladder) != nil || len(ladder.Checks) != 1 {
				t.Fatalf("cancelled validation evidence was lost: %s: %v", raw, err)
			}
			if !ladder.Checks[0].Cancelled || ladder.Checks[0].FailureKind != "VALIDATION_CANCELLED" {
				t.Fatalf("cancelled check was recorded as a regression: %+v", ladder.Checks[0])
			}
			r.stopFromError(context.Canceled)
			if r.state.Terminal.Status != "ABORTED" || r.state.Providers["codex"].Calls != map[bool]int{false: 0, true: 1}[phase == "characterization"] {
				t.Fatal("cancellation started recovery or lost ABORTED state", r.state.Terminal)
			}
		})
	}
}

func TestPacketCriticalRiskCannotBeEnabledByConfiguration(t *testing.T) {
	f := newProgressRunFixture(t)
	f.responses["deep_check"]["risk_level"] = "L3_CRITICAL"
	f.config["policy"] = map[string]any{"empty_audits_to_stop": 1, "allowed_risks": []string{"L0_LOW", "L1_MODERATE", "L2_HIGH", "L3_CRITICAL"}}
	f.prepare(t)
	_, report, _ := f.run(t, "")
	skipped := mapsOf(report["skipped"])
	if report["branch"] != nil || len(skipped) != 1 || skipped[0]["reason"] != "RISK_EXCLUDED" || obj(report["counters"])["commits"] != float64(0) {
		t.Fatalf("deep-check packet bypassed the critical-risk gate: %#v", report)
	}
}

func TestRunPreservesExactEditBytesAndRejectsUnrepairedEOF(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserved", true: "unrepaired-eof"}[broken], func(t *testing.T) {
			f := newProgressRunFixture(t)
			const path = "src/format.ts"
			original := []byte("export const retained = 1;\r\nexport const monthOf = (date: string) => String(date).slice(0, 7);\r\n\r\n")
			expected := bytes.Replace(original, []byte("export const monthOf = (date: string) => String(date).slice(0, 7);"), nil, 1)
			if err := os.WriteFile(filepath.Join(f.repo, path), original, 0644); err != nil {
				t.Fatal(err)
			}
			gitTest(t, f.repo, "add", path)
			gitTest(t, f.repo, "commit", "-qm", "byte preservation fixture")
			f.base = gitTest(t, f.repo, "rev-parse", "HEAD")
			for _, phase := range []string{"audit", "deep_check", "preflight", "execute", "review"} {
				data, err := json.Marshal(f.responses[phase])
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.NewReplacer("src/legacy-parser.mjs", path, "parseLegacy", "monthOf").Replace(string(data)))
				var response map[string]any
				if err := json.Unmarshal(data, &response); err != nil {
					t.Fatal(err)
				}
				f.responses[phase] = response
			}
			f.responses["deep_check"]["minimal_change"] = "Remove only the monthOf declaration contents; preserve its newline and all unrelated bytes."
			f.responses["execute"]["changed_files"] = []string{path}
			f.responses["execute"]["deleted_files"] = []string{}
			for name, data := range map[string][]byte{"before": original, "after": expected} {
				if err := os.WriteFile(filepath.Join(f.fixtures, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			edit := expected
			if broken {
				edit = expected[:len(expected)-2]
			}
			if err := os.WriteFile(filepath.Join(f.fixtures, "edit"), edit, 0600); err != nil {
				t.Fatal(err)
			}
			f.commands[0].Argv = []string{"/bin/sh", "-c", "cmp -s " + path + " " + progressShellQuote(filepath.Join(f.fixtures, "before")) + " || cmp -s " + path + " " + progressShellQuote(filepath.Join(f.fixtures, "after"))}
			f.prepare(t)
			script, err := os.ReadFile(f.bin)
			if err != nil {
				t.Fatal(err)
			}
			hook := "if [ \"$phase\" = execute ]; then cp " + progressShellQuote(filepath.Join(f.fixtures, "edit")) + " " + path + "; fi\n"
			body := strings.Replace(string(script), "cp "+progressShellQuote(f.fixtures), hook+"cp "+progressShellQuote(f.fixtures), 1)
			if err := os.WriteFile(f.bin, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			_, report, _ := f.run(t, "")
			want := expected
			if broken {
				want = original
				skipped := mapsOf(report["skipped"])
				if report["branch"] != nil || len(skipped) != 1 || skipped[0]["reason"] != "REGRESSION" {
					t.Fatalf("unrepaired mismatch was accepted: %#v", report)
				}
			} else if report["status"] != "DONE" || obj(report["counters"])["commits"] != float64(1) {
				t.Fatalf("exact edit did not complete: %#v", report)
			}
			actual, err := os.ReadFile(filepath.Join(str(report["worktree"]), path))
			if err != nil || !bytes.Equal(actual, want) {
				t.Fatalf("unexpected final bytes: %q: %v", actual, err)
			}
		})
	}
}

func TestPhaseEvidencePreservesFailuresWithoutRawOutput(t *testing.T) {
	zero, one := 0, 1
	r := runner{state: &runState{BaseOID: "base"}, baseline: workspace.Baseline{
		Usable: true, Describe: "GREEN 1 / RED-differential 1 of 3",
		Results: []workspace.CommandResult{
			{Command: workspace.Command{ID: "lint", Name: "lint", Area: "web", Cwd: "web", Argv: []string{"PRIVATE_ARGV"}}, Status: workspace.StatusGreen, ExitCode: &zero},
			{Command: workspace.Command{ID: "test"}, Status: workspace.StatusRed, ExitCode: &one, StderrTail: "PRIVATE_OUTPUT"},
			{Command: workspace.Command{ID: "build"}, Status: workspace.StatusUnrunnable, SpawnError: "PRIVATE_ERROR"},
		},
	}}
	r.validation = &engine.ValidationEvidence{
		TreeBeforeChecks: "checked-tree", Scope: []string{"web"}, ValidationAccepted: true,
		Checks: checkEvidence(r.baseline.Results[:2]),
	}
	evidence := r.phaseEvidence()
	if !evidence.BaselineUsable || evidence.Characterization != nil || len(evidence.BaselineChecks) != 3 {
		t.Fatalf("lost measured evidence: %+v", evidence)
	}
	checks := evidence.BaselineChecks
	if checks[1].Status != "RED" || *checks[1].ExitCode != 1 || checks[2].Status != "UNRUNNABLE" || checks[2].ExitCode != nil {
		t.Fatalf("failed or unavailable checks were misrepresented: %+v", checks)
	}
	if evidence.Validation == nil || !evidence.Validation.ValidationAccepted || evidence.Validation.Checks[1].Status != "RED" || *evidence.Validation.Checks[1].ExitCode != 1 || checks[0].Name != "lint" || checks[0].Area != "web" || checks[0].Cwd != "web" {
		t.Fatalf("accepted validation lost failures or command scope: %+v", evidence)
	}
	data, err := json.Marshal(evidence)
	if err != nil || strings.Contains(string(data), "PRIVATE_") {
		t.Fatalf("raw diagnostic output entered a prompt: %s: %v", data, err)
	}
}

func TestRunCarriesMeasuredEvidenceAcrossCandidatePhases(t *testing.T) {
	f := newProgressRunFixture(t)
	if err := os.MkdirAll(filepath.Join(f.repo, "test"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.repo, "test", "existing.test.mjs"), []byte("// existing assertions\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.repo, "src", "other-parser.mjs"), []byte("export function parseOther() { return 2; }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, f.repo, "add", "src/other-parser.mjs", "test/existing.test.mjs")
	gitTest(t, f.repo, "commit", "-qm", "second candidate")
	f.base = gitTest(t, f.repo, "rev-parse", "HEAD")
	for _, phase := range []string{"audit", "deep_check", "preflight", "execute", "review"} {
		data, err := json.Marshal(f.responses[phase])
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.NewReplacer("parseLegacy", "parseOther", "dead-legacy-parser", "dead-other-parser", "legacy-parser.mjs", "other-parser.mjs").Replace(string(data)))
		var second map[string]any
		if err := json.Unmarshal(data, &second); err != nil {
			t.Fatal(err)
		}
		f.responses[phase+"-second"] = second
	}
	const testPath = "test/entrypoint-characterization.test.mjs"
	packet := f.responses["deep_check"]
	packet["characterization_needed"] = true
	packet["characterization_files"] = []string{testPath, "test/existing.test.mjs"}
	packet["allowlist"] = []string{"src/legacy-parser.mjs", testPath, "test/existing.test.mjs"}
	packet["stop_conditions"] = []string{"Stop if the characterization file already exists before creation.", "Stop if the baseline result is unavailable."}
	f.responses["characterization"] = map[string]any{
		"schema_version": "1", "candidate_id": "dead-legacy-parser", "changed_files": []string{testPath, "test/existing.test.mjs"},
		"characterized_contracts": []string{"C1"}, "production_source_changed": false, "assertions_weakened": false,
		"verdict": "PASS", "notes": "Prepared test file before production edit.",
	}
	// Validate the test file against the original source, then validate the
	// production deletion with the same command. No real provider is called.
	f.commands[0].Argv = []string{"/bin/sh", "-c", "test -f src/index.mjs && { test ! -f test/entrypoint-characterization.test.mjs || test -s test/entrypoint-characterization.test.mjs; }"}
	f.prepare(t)
	script, err := os.ReadFile(f.bin)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(script), "cat >/dev/null", "prompt=$(cat)", 1)
	body = strings.Replace(body, "  */deep_check-last.json)", "  */characterization-last.json) phase=characterization ;;\n  */deep_check-last.json)", 1)
	hook := `
case "$phase" in
  characterization)
    test -f src/legacy-parser.mjs || exit 24
    test ! -e test/entrypoint-characterization.test.mjs || exit 25
    mkdir -p test
    printf '%s\n' '// pinned entrypoint behavior' > test/entrypoint-characterization.test.mjs
    printf '%s\n' '// coverage 1' '// coverage 2' '// coverage 3' '// coverage 4' '// coverage 5' '// coverage 6' >> test/existing.test.mjs
    ;;
  execute|execute-second)
    printf '%s' "$prompt" | grep -Fq '"baseline_usable": true' || exit 26
    printf '%s' "$prompt" | grep -Fq '"status": "GREEN"' || exit 27
    if [ "$phase" = execute ]; then
      test -s test/entrypoint-characterization.test.mjs || exit 28
      printf '%s' "$prompt" | grep -Fq '"validation_accepted": true' || exit 29
      printf '%s' "$prompt" | grep -Fq '"commit": "' || exit 30
      printf '%s' "$prompt" | grep -Fq 'pre-creation filename collision condition does not apply' || exit 31
    else
      printf '%s' "$prompt" | grep -Fq '"characterization": null' || exit 32
    fi
    ;;
esac
`
	body = strings.Replace(body, "cp "+progressShellQuote(f.fixtures), hook+"\ncp "+progressShellQuote(f.fixtures), 1)
	if err := os.WriteFile(f.bin, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	_, report, runDir := f.run(t, "")
	if report["status"] != "DONE" || obj(report["counters"])["commits"] != float64(2) || obj(report["counters"])["consecutiveFailures"] != float64(0) || len(mapsOf(report["skipped"])) != 0 {
		t.Fatalf("handoff caused a spurious stop: %#v", report)
	}
	commits := mapsOf(report["commits"])
	if len(commits) != 3 || commits[0]["category"] != "CHARACTERIZATION" {
		t.Fatalf("characterization commit was not retained: %#v", commits)
	}
	paths, err := filepath.Glob(filepath.Join(runDir, "cycles", "01-*", "characterization-validation.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("missing characterization validation record: %v, %v", paths, err)
	}
	var ladder workspace.Ladder
	data, err := os.ReadFile(paths[0])
	if err != nil || json.Unmarshal(data, &ladder) != nil || !ladder.OK || len(ladder.Checks) != 1 {
		t.Fatalf("incomplete recorded evidence: %s: %v", data, err)
	}
	var gate workspace.GateResult
	data, err = os.ReadFile(filepath.Join(filepath.Dir(paths[0]), "characterization-gate.json"))
	if err != nil || json.Unmarshal(data, &gate) != nil || gate.Verdict != "PASS" {
		t.Fatalf("valid test additions failed measured safety checks: %s: %v", data, err)
	}
	wt := str(report["worktree"])
	if got := gitTest(t, wt, "show", "HEAD:"+testPath); got != "// pinned entrypoint behavior" {
		t.Fatalf("execution changed the prepared tests: %q", got)
	}
	for cycle, pattern := range []string{"01-*", "02-*"} {
		for _, phase := range []string{"preflight", "execute", "review"} {
			prompts, err := filepath.Glob(filepath.Join(runDir, "cycles", pattern, "provider", phase+"-codex-primary.prompt.md"))
			if err != nil || len(prompts) != 1 {
				t.Fatalf("missing %s prompt: %v, %v", phase, prompts, err)
			}
			data, err := os.ReadFile(prompts[0])
			if err != nil {
				t.Fatal(err)
			}
			_, record, _ := strings.Cut(string(data), "<<<ORCHESTRATOR_EVIDENCE id=")
			_, record, _ = strings.Cut(record, ">>>\n")
			record, _, _ = strings.Cut(record, "\n<<<END_ORCHESTRATOR_EVIDENCE")
			var evidence engine.PhaseEvidence
			if err := json.Unmarshal([]byte(record), &evidence); err != nil {
				t.Fatal(err)
			}
			if evidence.BaseCommit != f.base {
				t.Fatalf("%s lost baseline provenance: %+v", phase, evidence)
			}
			if cycle == 0 && (evidence.Characterization == nil || evidence.Characterization.Commit != commits[0]["oid"] || len(evidence.Characterization.Files) != 2 || len(evidence.Characterization.CreatedFiles) != 1 || evidence.Characterization.CreatedFiles[0] != testPath) {
				t.Fatalf("%s lost provenance or exempted an existing file: %+v", phase, evidence)
			}
			if cycle == 1 && evidence.Characterization != nil {
				t.Fatalf("%s reused previous candidate's characterization", phase)
			}
			if phase == "review" {
				data, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(prompts[0])), "validation.json"))
				var validation workspace.Ladder
				if err != nil || json.Unmarshal(data, &validation) != nil {
					t.Fatalf("cannot read validation: %s: %v", data, err)
				}
				want := engine.ValidationEvidence{
					TreeBeforeChecks: gitTest(t, wt, "rev-parse", str(commits[cycle+1]["oid"])+"^{tree}"),
					Scope:            validation.Scope, ValidationAccepted: validation.OK, Checks: checkEvidence(validation.Checks),
				}
				if evidence.Validation == nil || !reflect.DeepEqual(*evidence.Validation, want) {
					t.Fatalf("review lost current candidate's validation: got %+v, want %+v", evidence.Validation, want)
				}
			} else if evidence.Validation != nil {
				t.Fatalf("%s reused previous candidate's validation", phase)
			}
		}
	}
}

func TestRunCharacterizationRejectsDishonestProvider(t *testing.T) {
	for _, tc := range []struct {
		name, path, edit, code string
	}{
		{"assertion-removed", "test/existing.test.mjs", "printf '// removed assertion\\n' > test/existing.test.mjs", "TEST_WEAKENED"},
		{"skip-added", "test/existing.test.mjs", "printf 'test.skip(() => {});\\n' >> test/existing.test.mjs", "TEST_WEAKENED"},
		{"production", "src/index.mjs", "printf '// production changed\\n' >> src/index.mjs", "PRODUCTION_CHANGED"},
		{"forbidden", "test/fixtures/data.test.mjs", "printf '// fixture changed\\n' >> test/fixtures/data.test.mjs", "FORBIDDEN_PATH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProgressRunFixture(t)
			path := filepath.Join(f.repo, tc.path)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			const original = "assert.equal(1, 1);\n"
			if err := os.WriteFile(path, []byte(original), 0644); err != nil {
				t.Fatal(err)
			}
			gitTest(t, f.repo, "add", tc.path)
			gitTest(t, f.repo, "commit", "-qm", "characterization fixture")
			f.base = gitTest(t, f.repo, "rev-parse", "HEAD")
			f.responses["deep_check"]["characterization_needed"] = true
			f.responses["deep_check"]["characterization_files"] = []string{tc.path}
			f.responses["characterization"] = map[string]any{
				"schema_version": "1", "candidate_id": "dead-legacy-parser", "changed_files": []string{tc.path},
				"characterized_contracts": []string{"C1"}, "production_source_changed": false, "assertions_weakened": false,
				"verdict": "PASS", "notes": "Claims safe tests despite the actual diff.",
			}
			f.prepare(t)
			script, err := os.ReadFile(f.bin)
			if err != nil {
				t.Fatal(err)
			}
			body := strings.Replace(string(script), "  */deep_check-last.json)", "  */characterization-last.json) phase=characterization ;;\n  */deep_check-last.json)", 1)
			hook := "if [ \"$phase\" = characterization ]; then " + tc.edit + "; fi\n"
			body = strings.Replace(body, "cp "+progressShellQuote(f.fixtures), hook+"cp "+progressShellQuote(f.fixtures), 1)
			if err := os.WriteFile(f.bin, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			_, report, runDir := f.run(t, "")
			skipped := mapsOf(report["skipped"])
			if report["branch"] != nil || len(skipped) != 1 || skipped[0]["reason"] != "CHARACTERIZATION_REJECTED" {
				t.Fatalf("unsafe characterization was accepted: %#v", report)
			}
			paths, err := filepath.Glob(filepath.Join(runDir, "cycles", "01-*", "characterization-gate.json"))
			if err != nil || len(paths) != 1 {
				t.Fatalf("missing measured gate evidence: %v: %v", paths, err)
			}
			var gate workspace.GateResult
			data, err := os.ReadFile(paths[0])
			if err != nil || json.Unmarshal(data, &gate) != nil || gate.Violation == nil || gate.Violation.Code != tc.code {
				t.Fatalf("wrong rejection evidence: %s: %v", data, err)
			}
			patch, err := os.ReadFile(filepath.Join(filepath.Dir(paths[0]), "characterization.patch"))
			if err != nil || !bytes.Contains(patch, []byte(tc.path)) {
				t.Fatalf("missing rejected diff: %s: %v", patch, err)
			}
			wt := str(report["worktree"])
			data, err = os.ReadFile(filepath.Join(wt, tc.path))
			if err != nil || string(data) != original || gitTest(t, wt, "rev-parse", "HEAD") != f.base {
				t.Fatalf("rejected characterization was not rolled back: %s: %v", data, err)
			}
		})
	}
}
