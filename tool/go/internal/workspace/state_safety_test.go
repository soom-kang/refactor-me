package workspace

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateDirectoriesRejectSymlinksWithoutWriting(t *testing.T) {
	for _, name := range []string{".refactor", ".refactor/runs", ".refactor/.gitignore"} {
		t.Run(name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			path := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
			if _, err := EnsureRefactorDir(root); err == nil {
				t.Fatal("accepted symlinked state")
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatal("wrote outside repository", err)
			}
			if _, err := RunDirFor(root, "new-run"); err == nil {
				t.Fatal("created run through symlink")
			}
		})
	}
}

func TestRunDirectoryRejectsCollisionAndInvalidID(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../escape", "child/run", "/tmp/run"} {
		root := t.TempDir()
		if _, err := RunDirFor(root, id); err == nil {
			t.Fatalf("accepted run ID %q", id)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("invalid ID %q created state: %v", id, err)
		}
	}
	root := t.TempDir()
	dir, err := RunDirFor(root, "existing")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "state.json")
	before := []byte("preserve evidence\n")
	if err := os.WriteFile(file, before, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunDirFor(root, "existing"); err == nil {
		t.Fatal("reused existing run")
	}
	if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, before) {
		t.Fatal("changed existing run", err)
	}
}

func TestStoreRejectsUnsupportedStateVersion(t *testing.T) {
	store := NewStore(t.TempDir())
	current := &RunState{Schema: StateSchema, RunID: "current"}
	if err := store.Init(current); err != nil {
		t.Fatal(err)
	}
	if err := store.Load(); err != nil || store.State.Schema != StateSchema {
		t.Fatal(store.State, err)
	}
	for _, raw := range []string{`{"schema":1}`, `{"schema":99}`, `{}`, `null`, `{broken`} {
		beforeState := store.State
		if err := os.WriteFile(store.File, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if err := store.Load(); err == nil {
			t.Fatalf("accepted state %s", raw)
		}
		if store.State != beforeState {
			t.Fatal("failed read replaced in-memory state")
		}
		if got, err := os.ReadFile(store.File); err != nil || string(got) != raw {
			t.Fatal("changed unsupported record", err)
		}
	}
}

func TestLastRunRejectsUnsupportedVersion(t *testing.T) {
	root := t.TempDir()
	dir, err := EnsureRefactorDir(root)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "last-run.json")
	for _, raw := range []string{`{"schemaVersion":1,"runId":"r"}`, `{"schemaVersion":2}`, `{}`, `null`, `{broken`} {
		if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := ReadLastRun(root)
		if strings.Contains(raw, `"schemaVersion":1`) {
			if err != nil || got["runId"] != "r" {
				t.Fatal(got, err)
			}
		} else if err == nil {
			t.Fatalf("accepted last-run %s", raw)
		}
		if got, err := os.ReadFile(file); err != nil || string(got) != raw {
			t.Fatal("changed last-run", err)
		}
	}
}

func TestRecordedRemovalRejectsUnsupportedVersion(t *testing.T) {
	root, wt, _, before := fixtureRepo(t)
	id := filepath.Base(wt)
	dir, err := RunDirFor(root, id)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "state.json")
	record := map[string]any{"runId": id, "repoRoot": root, "worktree": wt, "terminal": map[string]any{"status": "DONE"}}
	for _, version := range []int{0, 1, 99} {
		record["schema"] = version
		if err := WriteJSONAtomic(file, record); err != nil {
			t.Fatal(err)
		}
		if err := WorktreeRemoveRecorded(root, wt, id); err == nil || !strings.Contains(err.Error(), "unsupported state schema") {
			t.Fatalf("version %d: %v", version, err)
		}
		if _, err := os.Stat(wt); err != nil {
			t.Fatal("removed worktree with unsupported record", err)
		}
	}
	after, err := SourceFingerprint(root)
	if err != nil || after != before {
		t.Fatal("changed source checkout", err)
	}
	record["schema"] = StateSchema
	if err := WriteJSONAtomic(file, record); err != nil {
		t.Fatal(err)
	}
	if err := WorktreeRemoveRecorded(root, wt, id); err != nil {
		t.Fatal(err)
	}
}
