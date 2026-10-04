package controller

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

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
			{Command: workspace.Command{ID: "lint"}, Status: workspace.StatusGreen, ExitCode: &zero},
			{Command: workspace.Command{ID: "test"}, Status: workspace.StatusRed, ExitCode: &one, StderrTail: "PRIVATE_OUTPUT"},
			{Command: workspace.Command{ID: "build"}, Status: workspace.StatusUnrunnable, SpawnError: "PRIVATE_ERROR"},
		},
	}}
	evidence := r.phaseEvidence()
	if !evidence.BaselineUsable || evidence.Characterization != nil || len(evidence.BaselineChecks) != 3 {
		t.Fatalf("lost measured evidence: %+v", evidence)
	}
	checks := evidence.BaselineChecks
	if checks[1].Status != "RED" || *checks[1].ExitCode != 1 || checks[2].Status != "UNRUNNABLE" || checks[2].ExitCode != nil {
		t.Fatalf("failed or unavailable checks were misrepresented: %+v", checks)
	}
	data, err := json.Marshal(evidence)
	if err != nil || strings.Contains(string(data), "PRIVATE_") {
		t.Fatalf("raw diagnostic output entered a prompt: %s: %v", data, err)
	}
}

func TestRunCarriesCharacterizationAndBaselineEvidenceToExecution(t *testing.T) {
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
    printf '%s\n' '// additional coverage' >> test/existing.test.mjs
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
	wt := str(report["worktree"])
	if got := gitTest(t, wt, "show", "HEAD:"+testPath); got != "// pinned entrypoint behavior" {
		t.Fatalf("execution changed the prepared tests: %q", got)
	}
	for _, phase := range []string{"preflight", "execute"} {
		prompts, err := filepath.Glob(filepath.Join(runDir, "cycles", "01-*", "provider", phase+"-codex-primary.prompt.md"))
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
		if evidence.BaseCommit != f.base || evidence.Characterization == nil || evidence.Characterization.Commit != commits[0]["oid"] || len(evidence.Characterization.Files) != 2 || len(evidence.Characterization.CreatedFiles) != 1 || evidence.Characterization.CreatedFiles[0] != testPath {
			t.Fatalf("%s lost provenance or exempted an existing file: %+v", phase, evidence)
		}
	}
}
