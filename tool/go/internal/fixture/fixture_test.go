package fixture

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

func command(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	b, err := cmd.CombinedOutput()
	return string(b), err
}
func makeRepo(t *testing.T, o Options) string {
	t.Helper()
	if o.Destination == "" {
		o.Destination = filepath.Join(t.TempDir(), "repo")
	}
	dest, err := Create(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	return dest
}
func TestParse(t *testing.T) {
	o, err := Parse([]string{"--multi", "target", "--lang", "js", "--skills", "catalog"})
	if err != nil || !o.Multi || o.Destination != "target" || o.Language != "js" || o.Skills != "catalog" {
		t.Fatalf("%+v %v", o, err)
	}
	for _, args := range [][]string{{"--lang"}, {"--lang", "rust"}, {"--skills", "--multi"}, {"--unknown"}, {"one", "two"}} {
		if _, err := Parse(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	o, err = Parse(nil)
	if err != nil || o.Language != "go" || o.Destination != "" {
		t.Fatalf("defaults: %+v %v", o, err)
	}
}
func TestExistingDestinationAndInvalidOptionsPreserveFiles(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "keep")
	if err := os.WriteFile(marker, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, o := range []Options{{Destination: dir, Language: "go"}, {Destination: dir, Language: "rust"}, {Destination: dir, Language: "go", Skills: filepath.Join(dir, "missing")}} {
		if _, err := Create(context.Background(), o); err == nil {
			t.Fatal("existing or invalid accepted")
		}
	}
	b, err := os.ReadFile(marker)
	if err != nil || string(b) != "unchanged" {
		t.Fatal("existing file changed")
	}
}
func TestTemporaryDestination(t *testing.T) {
	dest, err := Create(context.Background(), Options{Language: "go"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dest) })
	if !strings.HasPrefix(filepath.Base(dest), "refactor-fixture-") {
		t.Fatal(dest)
	}
}
func TestGoFixtureContractsAndBaseline(t *testing.T) {
	root := makeRepo(t, Options{Language: "go", Multi: true})
	panel, err := os.ReadFile(filepath.Join(root, "src/big_panel.go"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(panel), "\n"); lines <= 500 {
		t.Fatalf("split candidate must exceed 500 lines, got %d", lines)
	}
	for _, area := range []string{".", "app/api", "app/worker"} {
		dir := filepath.Join(root, area)
		for _, args := range [][]string{{"go", "vet", "./..."}, {"go", "build", "./..."}, {"go", "test", "./...", "-run", "^TestContract$"}} {
			if out, err := command(t, dir, args...); err != nil {
				t.Fatalf("%s %v: %v\n%s", area, args, err, out)
			}
		}
		out, err := command(t, dir, "go", "test", "./...")
		if err == nil || !strings.Contains(out, "BASELINE_RED") {
			t.Fatalf("expected RED %s: %v %s", area, err, out)
		}
	}
	discovery, err := workspace.DiscoverCommands(root, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, area := range []string{".", "app/api", "app/worker"} {
		found := false
		for _, c := range discovery.Commands {
			if c.Area == area {
				found = true
			}
		}
		if !found {
			t.Fatalf("area %s missing: %+v", area, discovery)
		}
	}
	if out, err := command(t, root, "git", "status", "--porcelain"); err != nil || out != "" {
		t.Fatalf("validation dirtied fixture: %s %v", out, err)
	}
}
func TestSkillsAreInBaseCommit(t *testing.T) {
	skills := t.TempDir()
	if err := writeFile(skills, "review/SKILL.md", "# Review\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(skills, "review/check.sh", "#!/bin/sh\nexit 0\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(skills, "review/check.sh"), 0755); err != nil {
		t.Fatal(err)
	}
	root := makeRepo(t, Options{Language: "go", Skills: skills})
	info, err := os.Stat(filepath.Join(root, ".agents/skills/review/check.sh"))
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("skill executable mode: %v %v", info, err)
	}
	if out, err := command(t, root, "git", "show", "HEAD:.agents/skills/review/SKILL.md"); err != nil || out != "# Review\n" {
		t.Fatalf("skill not committed: %s %v", out, err)
	}
	link, err := os.Readlink(filepath.Join(root, ".claude/skills/review"))
	if err != nil || link != "../../.agents/skills/review" {
		t.Fatalf("link %s %v", link, err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(skills, "review", "outside")); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(context.Background(), Options{Destination: filepath.Join(t.TempDir(), "repo"), Language: "go", Skills: skills}); err == nil {
		t.Fatal("copied unsafe skill symlink")
	}
}
func TestJavaScriptFixtureDiscovery(t *testing.T) {
	root := makeRepo(t, Options{Language: "js", Multi: true})
	discovery, err := workspace.DiscoverCommands(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Commands) < 12 {
		t.Fatalf("expected commands for three JS areas: %+v", discovery)
	}
	for _, path := range []string{"src/plugin-x.mjs", "src/legacy-parser.mjs", "src/compat.mjs", "src/broken.mjs", "src/big-panel.mjs", "app/api/package.json", "app/worker/package.json"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestJavaScriptFixtureExecution(t *testing.T) {
	if os.Getenv("REFACTOR_TEST_JS") != "1" {
		t.Skip("set REFACTOR_TEST_JS=1 to execute optional Node target validation")
	}
	root := makeRepo(t, Options{Language: "js", Multi: true})
	for _, area := range []string{".", "app/api", "app/worker"} {
		dir := filepath.Join(root, area)
		for _, args := range [][]string{{"node", "tools/lint.mjs"}, {"node", "--test", "test/contract.test.mjs"}, {"node", "tools/build.mjs"}} {
			if out, err := command(t, dir, args...); err != nil {
				t.Fatalf("%s %v: %v %s", area, args, err, out)
			}
		}
		out, err := command(t, dir, "node", "tools/typecheck.mjs")
		if err == nil || !strings.Contains(out, "TS2345") {
			t.Fatalf("expected typecheck RED: %s %v", out, err)
		}
	}
}

func TestUnsafeSkillsFailBeforeDestinationCreation(t *testing.T) {
	for _, scenario := range []string{"source-symlink", "top-level-symlink", "nested-destination", "aliased-destination"} {
		t.Run(scenario, func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "skills")
			if err := writeFile(source, "review/SKILL.md", "# Review\n"); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(base, "fixture")
			switch scenario {
			case "source-symlink":
				alias := filepath.Join(base, "alias")
				if err := os.Symlink(source, alias); err != nil {
					t.Fatal(err)
				}
				source = alias
			case "top-level-symlink":
				if err := os.Symlink(filepath.Join(source, "review"), filepath.Join(source, "linked")); err != nil {
					t.Fatal(err)
				}
			case "nested-destination":
				dest = filepath.Join(source, "review", "fixture")
			case "aliased-destination":
				alias := filepath.Join(base, "alias")
				if err := os.Symlink(filepath.Join(source, "review"), alias); err != nil {
					t.Fatal(err)
				}
				dest = filepath.Join(alias, "fixture")
			}
			if _, err := Create(context.Background(), Options{Destination: dest, Language: "go", Skills: source}); err == nil {
				t.Fatal("unsafe source accepted")
			}
			if _, err := os.Lstat(dest); !os.IsNotExist(err) {
				t.Fatalf("destination created: %v", err)
			}
		})
	}
}
