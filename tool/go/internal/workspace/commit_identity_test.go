package workspace

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitApprovedUsesActiveGitIdentity(t *testing.T) {
	for _, source := range []string{"local", "global", "include", "worktree", "environment", "missing-name", "missing-email"} {
		t.Run(source, func(t *testing.T) {
			global := filepath.Join(t.TempDir(), "gitconfig")
			if err := os.WriteFile(global, []byte("[user]\n\tname = Global User\n\temail = global@example.invalid\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GIT_CONFIG_GLOBAL", global)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			t.Setenv("GIT_CONFIG_COUNT", "0")
			for _, key := range []string{"GIT_CONFIG_PARAMETERS", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
				t.Setenv(key, "")
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			root, wt, base, _ := fixtureRepo(t)
			config := func(cwd string, args ...string) {
				t.Helper()
				if _, err := Git(cwd, append([]string{"config"}, args...)...); err != nil {
					t.Fatal(err)
				}
			}
			wantName, wantEmail := "Test", "test@example.invalid"
			switch source {
			case "global":
				config(root, "--unset", "user.name")
				config(root, "--unset", "user.email")
				wantName, wantEmail = "Global User", "global@example.invalid"
			case "include":
				config(root, "--unset", "user.name")
				config(root, "--unset", "user.email")
				included := filepath.Join(t.TempDir(), "included.gitconfig")
				if err := os.WriteFile(included, []byte("[user]\n\tname = Included User\n\temail = included@example.invalid\n"), 0600); err != nil {
					t.Fatal(err)
				}
				common, err := gitCommonDir(root)
				if err != nil {
					t.Fatal(err)
				}
				config(root, "--file", global, "includeIf.gitdir:"+filepath.ToSlash(common)+"/.path", included)
				wantName, wantEmail = "Included User", "included@example.invalid"
			case "worktree":
				config(root, "extensions.worktreeConfig", "true")
				config(wt, "--worktree", "user.name", "Worktree User")
				config(wt, "--worktree", "user.email", "worktree@example.invalid")
				wantName, wantEmail = "Worktree User", "worktree@example.invalid"
			case "environment":
				wantName, wantEmail = "Explicit User", "explicit@example.invalid"
				t.Setenv("GIT_AUTHOR_NAME", wantName)
				t.Setenv("GIT_AUTHOR_EMAIL", wantEmail)
				t.Setenv("GIT_COMMITTER_NAME", wantName)
				t.Setenv("GIT_COMMITTER_EMAIL", wantEmail)
			case "missing-name", "missing-email":
				if err := os.WriteFile(global, nil, 0600); err != nil {
					t.Fatal(err)
				}
				key := "user.name"
				if source == "missing-email" {
					key = "user.email"
				}
				config(root, "--unset", key)
			}
			configPath := filepath.Join(root, ".git", "config")
			before, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(wt, "src", "a.go"), []byte("package a\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := StageExact(wt, []string{"src/a.go"}); err != nil {
				t.Fatal(err)
			}
			tree, err := WriteTree(wt)
			if err != nil {
				t.Fatal(err)
			}
			message := filepath.Join(t.TempDir(), "message")
			if err := os.WriteFile(message, []byte("refactor: remove unused function\n"), 0600); err != nil {
				t.Fatal(err)
			}
			oid, commitErr := CommitApproved(wt, message, tree)
			if strings.HasPrefix(source, "missing-") {
				if !errors.Is(commitErr, ErrUnsafe) || oid != "" {
					t.Fatalf("missing identity did not stop commit: oid=%q err=%v", oid, commitErr)
				}
				if head, err := HeadOID(wt); err != nil || head != base {
					t.Fatalf("failed commit changed HEAD: %s, %v", head, err)
				}
				if staged, err := WriteTree(wt); err != nil || staged != tree {
					t.Fatalf("failed commit lost approved tree: %s, %v", staged, err)
				}
			} else {
				if commitErr != nil {
					t.Fatal(commitErr)
				}
				got, err := Git(wt, "show", "-s", "--format=%an%n%ae%n%cn%n%ce", oid)
				want := strings.Join([]string{wantName, wantEmail, wantName, wantEmail}, "\n") + "\n"
				if err != nil || got != want {
					t.Fatalf("commit identity = %q, want %q: %v", got, want, err)
				}
			}
			after, err := os.ReadFile(configPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("commit changed repository configuration: %v", err)
			}
		})
	}
}
