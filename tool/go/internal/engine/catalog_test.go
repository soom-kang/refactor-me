package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soom-kang/refactor-me/tool/go/internal/catalog"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

func catalogFixture(t *testing.T) (*catalog.Catalog, string) {
	t.Helper()
	home, repo := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	for _, name := range catalog.Required {
		path := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: "+name+"\ndescription: test\n---\nInstructions.\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c, err := catalog.Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Snapshot(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return c, repo
}
func TestProviderGlobalSkillDelivery(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			c, repo := catalogFixture(t)
			runDir := t.TempDir()
			bin := filepath.Join(t.TempDir(), "provider")
			script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + runDir + "/args'\ncat > '" + runDir + "/body'\nprintf '%s\\n' '{\"type\":\"result\",\"is_error\":false,\"result\":\"ok\"}'\n"
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			result, err := CallProvider(context.Background(), provider, Request{Phase: "doctor", CWD: repo, RunDir: runDir, Body: "probe"}, Config{Skills: c, Agents: map[string]AgentConfig{provider: {Bin: bin}}}, nil)
			if err != nil || !result.OK {
				t.Fatal(result, err)
			}
			args, _ := os.ReadFile(filepath.Join(runDir, "args"))
			body, _ := os.ReadFile(filepath.Join(runDir, "body"))
			if provider == "claude" {
				if !strings.Contains(string(args), "--setting-sources\nproject\n") || !strings.Contains(string(args), "--add-dir\n"+c.ClaudeRoot+"\n") {
					t.Fatal(string(args))
				}
				if !strings.Contains(string(body), filepath.Join(c.ClaudeRoot, ".claude", "skills", catalog.Required[0], "SKILL.md")) {
					t.Fatal(string(body))
				}
			} else if !strings.Contains(string(body), filepath.Join(c.Entries[0].Path, "SKILL.md")) {
				t.Fatal(string(body))
			}
		})
	}
}
func TestProviderDetectsSkillDrift(t *testing.T) {
	for _, which := range []string{"source-before", "source-after", "snapshot-after", "repair"} {
		t.Run(which, func(t *testing.T) {
			c, repo := catalogFixture(t)
			runDir := t.TempDir()
			bin := filepath.Join(t.TempDir(), "provider")
			path := filepath.Join(c.Entries[0].Path, "SKILL.md")
			if which == "snapshot-after" {
				path = filepath.Join(c.ClaudeRoot, ".claude", "skills", catalog.Required[0], "SKILL.md")
			}
			count := filepath.Join(runDir, "count")
			body := "#!/bin/sh\ncat >/dev/null\necho call >> '" + count + "'\n"
			if which == "repair" {
				body += "if [ -f '" + runDir + "/called' ]; then echo changed >> '" + path + "'; else touch '" + runDir + "/called'; fi\n"
			} else {
				body += "echo changed >> '" + path + "'\n"
			}
			body += "printf '%s\\n' '{\"type\":\"result\",\"is_error\":false,\"result\":\"invalid\"}'\n"
			if err := os.WriteFile(bin, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			if which == "source-before" {
				if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := CallWithRepair(context.Background(), "claude", Request{Phase: "audit", CWD: repo, RunDir: runDir, Schema: Schema("audit")}, Config{Skills: c, Agents: map[string]AgentConfig{"claude": {Bin: bin}}}, nil)
			if !errors.Is(err, workspace.ErrUnsafe) {
				t.Fatalf("expected unsafe: %v", err)
			}
			calls, readErr := os.ReadFile(count)
			if which == "source-before" {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatal("provider ran after drift")
				}
			} else {
				want := 1
				if which == "repair" {
					want = 2
				}
				if strings.Count(string(calls), "call") != want {
					t.Fatal(string(calls))
				}
			}
		})
	}
}
