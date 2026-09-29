package controller

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soom-kang/refactor-me/tool/go/internal/catalog"
	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

func TestPublicationRejectsSkillChangeByCommitHook(t *testing.T) {
	home := globalSkillsFixture(t)
	r, write, commit := comparisonFixture(t)
	write("source.txt", "before\n")
	base := commit()
	repo := r.state.RepoRoot
	c, err := catalog.Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "worktree")
	if err := workspace.WorktreeAdd(repo, wt, base); err != nil {
		t.Fatal(err)
	}
	defer workspace.WorktreeRemove(repo, wt)
	if err := os.WriteFile(filepath.Join(wt, "source.txt"), []byte("after\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, wt, "add", "source.txt")
	tree := gitTest(t, wt, "write-tree")
	skill := filepath.Join(c.Entries[0].Path, "SKILL.md")
	hook := filepath.Join(repo, ".git", "hooks", "pre-commit")
	script := "#!/bin/sh\nprintf '\\nHook changed skill.\\n' >> '" + strings.ReplaceAll(skill, "'", "'\"'\"'") + "'\n"
	if err := os.WriteFile(hook, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	message := filepath.Join(t.TempDir(), "message")
	if err := os.WriteFile(message, []byte("fixture commit"), 0600); err != nil {
		t.Fatal(err)
	}
	oid, err := workspace.CommitApproved(wt, message, tree)
	if err != nil {
		t.Fatal(err)
	}
	r.surface = surface.Context{Repo: repo}
	r.wt = wt
	r.engineConfig = engine.Config{Skills: c}
	r.state.BranchName = "refactor/skill-drift"
	if err := r.publish(oid); !errors.Is(err, workspace.ErrUnsafe) {
		t.Fatalf("expected unsafe: %v", err)
	}
	if refs := gitTest(t, repo, "for-each-ref", "--format=%(refname)", "refs/heads/refactor/skill-drift"); refs != "" {
		t.Fatal("published unsafe ref", refs)
	}
	if current := gitTest(t, repo, "rev-parse", "HEAD"); current != base {
		t.Fatal("source HEAD changed")
	}
	if current := gitTest(t, repo, "status", "--porcelain"); current != "" {
		t.Fatal("source checkout changed", current)
	}
}
func TestCleanRejectsUnsupportedStateSchema(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".refactor", "runs", "old")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"schema":1,"worktree":"/not-used"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Clean(surface.Context{Repo: repo}); err == nil || !strings.Contains(err.Error(), "unsupported state schema") {
		t.Fatal(err)
	}
}
