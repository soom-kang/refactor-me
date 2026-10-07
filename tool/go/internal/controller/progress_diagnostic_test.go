package controller

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
)

func TestProgressInitFailurePointsToSavedFailure(t *testing.T) {
	f := newProgressRunFixture(t)
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	obj(f.config["workspace"])["worktree_parent"] = blocked
	f.prepare(t)
	var stdout, stderr bytes.Buffer
	code := surface.Execute([]string{"run", "--provider", "codex", "--fallback", "none", "--model", "fixture-model", "--no-live-probe", "--json"}, f.repo, &stdout, &stderr, surface.Callbacks{Run: Run, Doctor: Doctor})
	if code != surface.ExitAborted || stdout.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	entries, err := os.ReadDir(filepath.Join(f.repo, ".refactor", "runs"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("runs=%v err=%v", entries, err)
	}
	runDir := filepath.Join(f.repo, ".refactor", "runs", entries[0].Name())
	var doctor doctorReport
	data, err := os.ReadFile(filepath.Join(runDir, "doctor.json"))
	if err != nil || json.Unmarshal(data, &doctor) != nil || !doctor.OK {
		t.Fatalf("doctor should pass before worktree failure: %s, %v", data, err)
	}
	var state runState
	data, err = os.ReadFile(filepath.Join(runDir, "state.json"))
	if err != nil || json.Unmarshal(data, &state) != nil {
		t.Fatalf("missing failure state: %s, %v", data, err)
	}
	if state.Terminal == nil || state.Terminal.Status != "ABORTED" || !strings.Contains(state.Terminal.Reason, "not-a-directory") {
		t.Fatalf("saved state does not explain initialization failure: %+v", state.Terminal)
	}
	if !strings.Contains(stderr.String(), "state.json") || strings.Contains(stderr.String(), "doctor.json") || strings.Contains(stderr.String(), "Worktree ready") {
		t.Fatalf("wrong failure diagnostic or premature success:\n%s", &stderr)
	}
	if _, err := os.Stat(filepath.Join(f.repo, ".refactor", "lock.json")); !os.IsNotExist(err) {
		t.Fatalf("initialization failure retained lock: %v", err)
	}
}

func TestProgressDiagnosticPreservesRepeatedSpacePaths(t *testing.T) {
	for _, language := range []string{"en", "ko"} {
		t.Run(language, func(t *testing.T) {
			f := newProgressRunFixture(t)
			parent, err := os.MkdirTemp("/private/tmp", "progress-path-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(parent) })
			repo := filepath.Join(parent, "project  fixture")
			if err := os.Rename(f.repo, repo); err != nil {
				t.Fatal(err)
			}
			f.repo = repo
			f.responses["deep_check"]["risk_level"] = "UNKNOWN"
			f.prepare(t)
			output, _, runDir := f.run(t, language)
			actual := filepath.Join(runDir, "state.json")
			if utf8.RuneCountInString(actual) > 160 {
				t.Fatal("fixture must exercise whitespace fallback without length truncation")
			}
			prefix := "Details (relative to the selected repository): "
			if language == "ko" {
				prefix = "상세 기록 (선택한 저장소 기준 상대 경로): "
			}
			found := false
			for _, line := range strings.Split(output, "\n") {
				line = progressBody(line)
				if !strings.HasPrefix(line, prefix) {
					continue
				}
				found = true
				printed := filepath.Join(repo, strings.TrimPrefix(line, prefix))
				got, gotErr := os.Stat(printed)
				want, wantErr := os.Stat(actual)
				if gotErr != nil || wantErr != nil || !os.SameFile(got, want) {
					t.Fatalf("diagnostic path differs from actual saved file: %s, %v, %v", printed, gotErr, wantErr)
				}
			}
			if !found {
				t.Fatalf("missing accurate relative diagnostic path:\n%s", output)
			}
		})
	}
}
