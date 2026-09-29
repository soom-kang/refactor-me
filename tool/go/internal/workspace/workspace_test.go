package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureRepo(t *testing.T) (string, string, string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.invalid"}} {
		if _, err := Git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.go"), []byte("package a\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Git(root, "add", "src/a.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := Git(root, "commit", "-qm", "initial"); err != nil {
		t.Fatal(err)
	}
	base, err := HeadOID(root)
	if err != nil {
		t.Fatal(err)
	}
	fp, err := SourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "test-run")
	if err := WorktreeAdd(root, wt, base); err != nil {
		t.Fatal(err)
	}
	return root, wt, base, fp
}

func TestGateRollbackPreservesSource(t *testing.T) {
	root, wt, base, fp := fixtureRepo(t)
	if err := os.WriteFile(filepath.Join(wt, "src", "a.go"), []byte("package a\nfunc A() { }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	packet := Packet{Category: "DEAD_CODE", Allowlist: []string{"src/a.go"}}
	facts, err := CollectFacts(wt, root, base, base, fp, packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Changed) != 1 {
		t.Fatalf("changed = %v", facts.Changed)
	}
	got := RunChecks(facts, packet, Policy{}, nil, nil)
	if got.Verdict != "PASS" {
		t.Fatalf("gate = %+v", got.Violation)
	}
	back, err := Rollback(root, wt, base)
	if err != nil {
		t.Fatal(err)
	}
	if !back.Clean {
		t.Fatalf("rollback residue: %s", back.Residue)
	}
	after, err := SourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if after != fp {
		t.Fatalf("source changed: %s != %s", after, fp)
	}
	if err := WorktreeRemove(root, wt); err != nil {
		t.Fatal(err)
	}
}

func TestPublishCASRejectsExistingRef(t *testing.T) {
	root, _, base, _ := fixtureRepo(t)
	ref := "refs/heads/refactor/auto-collision"
	if _, err := Git(root, "update-ref", ref, base); err != nil {
		t.Fatal(err)
	}
	if err := PublishCAS(root, ref, base, strings.Repeat("0", len(base))); err == nil {
		t.Fatal("existing result ref was overwritten")
	}
	got, err := Git(root, "rev-parse", "--verify", ref)
	if err != nil || strings.TrimSpace(got) != base {
		t.Fatalf("result ref changed: %s, %v", got, err)
	}
}

func TestRollbackRefusesUnownedWorktree(t *testing.T) {
	root, wt, base, _ := fixtureRepo(t)
	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Rollback(root, other, base); err == nil {
		t.Fatal("rollback accepted unowned path")
	}
	if err := WorktreeRemove(root, wt); err != nil {
		t.Fatal(err)
	}
}

func TestCommitApprovedRejectsHookTreeMutation(t *testing.T) {
	root, wt, _, _ := fixtureRepo(t)
	if err := os.WriteFile(filepath.Join(wt, "src", "a.go"), []byte("package a\nfunc A() { println(1) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StageExact(wt, []string{"src/a.go"}); err != nil {
		t.Fatal(err)
	}
	tree, err := WriteTree(wt)
	if err != nil {
		t.Fatal(err)
	}
	hooks, err := Git(wt, "rev-parse", "--git-path", "hooks")
	if err != nil {
		t.Fatal(err)
	}
	hookDir := strings.TrimSpace(hooks)
	if !filepath.IsAbs(hookDir) {
		hookDir = filepath.Join(wt, hookDir)
	}
	if err := os.WriteFile(filepath.Join(hookDir, "pre-commit"), []byte("#!/bin/sh\nprintf '// hook\\n' >> src/a.go\ngit add src/a.go\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	msg := filepath.Join(t.TempDir(), "message")
	if err := os.WriteFile(msg, []byte("refactor: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitApproved(wt, msg, tree); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("expected unsafe hook error, got %v", err)
	}
	if RefExists(root, "refs/heads/refactor/test") {
		t.Fatal("unexpected published ref")
	}
}

func TestCommitApprovedRejectsFailingHook(t *testing.T) {
	_, wt, _, _ := fixtureRepo(t)
	if err := os.WriteFile(filepath.Join(wt, "src", "a.go"), []byte("package a\nfunc A() { println(2) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StageExact(wt, []string{"src/a.go"}); err != nil {
		t.Fatal(err)
	}
	tree, err := WriteTree(wt)
	if err != nil {
		t.Fatal(err)
	}
	hooks, err := Git(wt, "rev-parse", "--git-path", "hooks")
	if err != nil {
		t.Fatal(err)
	}
	hookDir := strings.TrimSpace(hooks)
	if !filepath.IsAbs(hookDir) {
		hookDir = filepath.Join(wt, hookDir)
	}
	if err := os.WriteFile(filepath.Join(hookDir, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	msg := filepath.Join(t.TempDir(), "message")
	if err := os.WriteFile(msg, []byte("refactor: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitApproved(wt, msg, tree); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("failing hook must halt unsafe: %v", err)
	}
}

func TestRecordedRemovalRequiresMatchingCleanCompletedRun(t *testing.T) {
	root, wt, _, _ := fixtureRepo(t)
	id := filepath.Base(wt)
	dir, err := RunDirFor(root, id)
	if err != nil {
		t.Fatal(err)
	}
	st := map[string]any{"schema": StateSchema, "runId": id, "repoRoot": root, "worktree": wt, "terminal": map[string]any{"status": "HALTED_UNSAFE"}}
	if err := WriteJSONAtomic(filepath.Join(dir, "state.json"), st); err != nil {
		t.Fatal(err)
	}
	if err := WorktreeRemoveRecorded(root, wt, id); err == nil {
		t.Fatal("removed unsafe run")
	}
	st["terminal"] = map[string]any{"status": "DONE"}
	if err := WriteJSONAtomic(filepath.Join(dir, "state.json"), st); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "untracked"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WorktreeRemoveRecorded(root, wt, id); err == nil {
		t.Fatal("removed dirty worktree")
	}
	if err := os.Remove(filepath.Join(wt, "untracked")); err != nil {
		t.Fatal(err)
	}
	if err := WorktreeRemoveRecorded(root, wt, id); err != nil {
		t.Fatal(err)
	}
}

func TestRunCommandKillsProcessGroupOnTimeout(t *testing.T) {
	root := t.TempDir()
	cmd := Command{ID: "x", Name: "test", Area: ".", Cwd: ".", Tier: TierTest, Argv: []string{"sh", "-c", "sleep 30 & wait"}, TimeoutMS: 100}
	started := time.Now()
	r := RunCommand(context.Background(), cmd, root, nil)
	if !r.TimedOut || r.Status != StatusTimeout {
		t.Fatalf("result = %+v", r)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("group termination took too long: %s", time.Since(started))
	}
}

func TestScopeAndSignatures(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app", "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	targets, problems := NormalizeTargets(root, []string{"web", "web"}, filepath.Join(root, "app"))
	if len(problems) != 0 || len(targets) != 1 || targets[0] != "app/web" {
		t.Fatalf("targets=%v problems=%v", targets, problems)
	}
	if IsUnderTarget("app/website/index.ts", targets) {
		t.Fatal("prefix sibling crossed target boundary")
	}
	if !IsUnderTarget("app/web/index.ts", targets) {
		t.Fatal("target child rejected")
	}
	before := ExtractSignature("app/a.go:12: error\n--- FAIL: TestThing")
	after := ExtractSignature("app/a.go:20: error\n--- FAIL: TestThing")
	delta := CompareSignatures(before, after)
	if !delta.OK || len(delta.Drifted) != 1 {
		t.Fatalf("line drift was not tolerated: %+v", delta)
	}
}
