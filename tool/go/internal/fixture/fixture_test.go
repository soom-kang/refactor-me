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
	o, err := Parse([]string{"--multi", "target", "--lang", "js"})
	if err != nil || !o.Multi || o.Destination != "target" || o.Language != "js" {
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
	for _, o := range []Options{{Destination: dir, Language: "go"}, {Destination: dir, Language: "rust"}} {
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
func TestFixtureDoesNotInstallSkills(t *testing.T) {
	root := makeRepo(t, Options{Language: "go"})
	for _, name := range []string{".agents", ".claude"} {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected skill installation: %s %v", name, err)
		}
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
