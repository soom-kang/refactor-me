package workspace

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func migrationFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestMigrationDiscovery(t *testing.T) {
	simple := migrationFiles(t, map[string]string{"package.json": `{"scripts":{"lint":"node lint.mjs","test":"node --test","typecheck":"node check.mjs","build":"node build.mjs"}}`})
	if commands := DiscoverArea(simple, "."); len(commands) != 4 {
		t.Fatal(commands)
	}

	for _, tc := range []struct {
		name  string
		files map[string]string
		want  [][]string
	}{
		{"go", map[string]string{"go.mod": "module example.invalid/fixture\n"}, [][]string{{"go", "vet", "./..."}, {"go", "test", "-count=1", "./..."}, {"go", "build", "./..."}}},
		{"gradle", map[string]string{"build.gradle": "plugins { id 'java' }"}, [][]string{{"gradle", "testClasses", "--console=plain"}, {"gradle", "test", "--console=plain"}}},
		{"kotlin", map[string]string{"build.gradle.kts": "plugins { kotlin(\"jvm\") }"}, [][]string{{"gradle", "testClasses", "--console=plain"}, {"gradle", "test", "--console=plain"}}},
		{"legacy-gradle", map[string]string{"build.gradle": "apply plugin: 'java-library'"}, [][]string{{"gradle", "testClasses", "--console=plain"}, {"gradle", "test", "--console=plain"}}},
		{"no-jvm", map[string]string{"build.gradle": "plugins { id 'base' }"}, nil},
		{"near-plugin-name", map[string]string{"build.gradle": "apply plugin: 'notjava'"}, nil},
		{"subproject", map[string]string{"settings.gradle": "include 'api'", "api/build.gradle": "plugins { id 'java' }"}, [][]string{{"gradle", "testClasses", "--console=plain"}, {"gradle", "test", "--console=plain"}}},
		{"maven", map[string]string{"pom.xml": "<project/>"}, [][]string{{"mvn", "-B", "test-compile"}, {"mvn", "-B", "test"}}},
		{"python-make", map[string]string{"pyproject.toml": "[tool.ruff]\n", "Makefile": "test:\n\tpython -m unittest\n"}, [][]string{{"ruff", "check", "."}, {"make", "test"}}},
		{"uv-declared", map[string]string{"pyproject.toml": "dependencies = ['ruff']\n", "uv.lock": ""}, [][]string{{"uv", "run", "--", "ruff", "check", "."}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := migrationFiles(t, tc.files)
			var got [][]string
			for _, c := range DiscoverArea(root, ".") {
				got = append(got, c.Argv)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv=%v want %v", got, tc.want)
			}
		})
	}
	for _, runner := range []struct{ lock, name string }{{"", "npm"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"}, {"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}} {
		t.Run(runner.name+runner.lock, func(t *testing.T) {
			files := map[string]string{"package.json": `{"scripts":{"lint":"eslint .","test":"playwright test","build":"build","typecheck":"tsc"}}`}
			if runner.lock != "" {
				files[runner.lock] = ""
			}
			root := migrationFiles(t, files)
			d, err := DiscoverCommands(root, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Commands) != 3 || len(d.Skipped) != 1 {
				t.Fatalf("discovery=%+v", d)
			}
			for _, c := range append(d.Commands, d.Skipped...) {
				if c.Argv[0] != runner.name {
					t.Fatal(c)
				}
			}
		})
	}
}

func TestMigrationJVMWrapperAndAreas(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, "build.gradle"), []byte("plugins { id 'java' }"), 0600); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "repo")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "build.gradle"), []byte("plugins { id 'java' }"), 0600); err != nil {
		t.Fatal(err)
	}
	if len(DiscoverArea(child, ".")) != 2 {
		t.Fatal("outside-root build overshadowed root")
	}

	root := migrationFiles(t, map[string]string{"build.gradle": "plugins { id 'java' }", "api/build.gradle": "plugins { id 'java' }", "web/package.json": `{"scripts":{"test":"unit"}}`, "gradlew": "#!/bin/sh\nprintf 'wrapper invoked'\n"})
	if got := DetectAreas(root, 3); !reflect.DeepEqual(got, []string{".", "web"}) {
		t.Fatal(got)
	}
	d, err := DiscoverCommands(root, "")
	if err != nil {
		t.Fatal(err)
	}
	families := map[string]bool{}
	for _, c := range d.Commands {
		families[c.Family] = true
	}
	if !families["gradle"] || !families["npm"] {
		t.Fatal(d)
	}
	if got := DiscoverArea(root, "api"); len(got) != 0 {
		t.Fatal(got)
	}
	if got := DiscoverArea(root, ".")[0].Argv[0]; got != "gradle" {
		t.Fatal(got)
	}
	if err := os.Chmod(filepath.Join(root, "gradlew"), 0700); err != nil {
		t.Fatal(err)
	}
	c := DiscoverArea(root, ".")[0]
	if c.Argv[0] != "./gradlew" {
		t.Fatal(c)
	}
	r := RunCommand(context.Background(), c, root, nil)
	if r.Status != StatusGreen || r.StdoutTail != "wrapper invoked" {
		t.Fatal(r)
	}
}

func TestMigrationScopeBoundaries(t *testing.T) {
	root := migrationFiles(t, map[string]string{"app/web/src/a.go": "package a", "README.md": "x"})
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"..", ".", "missing", "README.md", "escape"} {
		t.Run(raw, func(t *testing.T) {
			targets, problems := NormalizeTargets(root, []string{raw}, root)
			if len(targets) != 0 || len(problems) != 1 {
				t.Fatalf("%v %v", targets, problems)
			}
		})
	}
	got, problems := NormalizeTargets(root, []string{"app/web/", "app/web/src", "app/web"}, root)
	if len(problems) != 0 || !reflect.DeepEqual(got, []string{"app/web"}) {
		t.Fatalf("%v %v", got, problems)
	}
	if !TouchesTarget(nil, nil) || !TouchesTarget([]string{"app/web/a.go", "other.go"}, got) || TouchesTarget([]string{"app/website/a.go"}, got) {
		t.Fatal("target containment")
	}
	want := []string{"app/a.ts", "app/a.mjs", "api/a.go", "src/A.java"}
	all := append(slices.Clone(want), "vendor/b.go", "app/node_modules/b.js", "app/dist/b.js", "README.md")
	if files := SourceFiles(all); !reflect.DeepEqual(files, want) {
		t.Fatal(files)
	}
}

func TestMigrationValidationSignatures(t *testing.T) {
	if !reflect.DeepEqual(ExtractSignature("testThing(pkg.Test) -- Time elapsed: 0.04 s <<< FAILURE!"), ExtractSignature("testThing(pkg.Test) -- Time elapsed: 99.99 s <<< FAILURE!")) {
		t.Fatal("elapsed duration changed signature")
	}
	if d := CompareSignatures(ExtractSignature("[ERROR] src/Main.java:[12,9] error"), ExtractSignature("[ERROR] src/Main.java:[20,70] error")); !d.OK {
		t.Fatal(d)
	}

	for _, tc := range []struct{ input, want string }{
		{"[ERROR] src/Main.java:[12,9] error", "src/Main.java:12"}, {"e: src/App.kt: (20, 4): error", "src/App.kt:20"}, {"pkg.Test > method FAILED", "JUNIT:pkg.Test > method"}, {"testThing(pkg.Test) -- Time elapsed: 0.04 s <<< FAILURE!", "SUREFIRE:testThing(pkg.Test)"}, {"not ok 1 - old behavior", "TAP:old behavior"}, {"--- FAIL: TestThing/sub", "GOFAIL:TestThing/sub"}, {"src/a.ts:2 error TS1234", "TS1234"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := ExtractSignature(tc.input); !slices.Contains(got, tc.want) {
				t.Fatal(got)
			}
		})
	}
	for _, line := range []string{"BUILD FAILED", "Tests run: 5, Failures: 1", "Task :test FAILED"} {
		if got := ExtractSignature(line); len(got) != 0 {
			t.Fatalf("status count became signature: %v", got)
		}
	}
	for _, tc := range []struct {
		after []string
		ok    bool
	}{{[]string{"src/a.go:9", "E1234"}, true}, {nil, true}, {[]string{"src/b.go:1"}, false}, {[]string{"src/a.go:1", "E9999"}, false}} {
		if got := CompareSignatures([]string{"src/a.go:1", "E1234"}, tc.after); got.OK != tc.ok {
			t.Fatal(got)
		}
	}
}

func TestMigrationDifferentialLadder(t *testing.T) {
	root := migrationFiles(t, map[string]string{"check": "#!/bin/sh\necho src/a.go:1\nexit 1\n"})
	if err := os.Chmod(filepath.Join(root, "check"), 0700); err != nil {
		t.Fatal(err)
	}
	c := Command{ID: "a", Area: ".", Cwd: ".", Tier: TierFast, Argv: []string{"./check"}}
	zero := 0
	one := 1
	for _, tc := range []struct {
		name, body, status string
		signature          []string
		want               bool
	}{
		{"same-red", "echo src/a.go:1; exit 1", StatusRed, []string{"src/a.go:1"}, true},
		{"new-file", "echo src/b.go:1; exit 1", StatusRed, []string{"src/a.go:1"}, false},
		{"opaque-red", "echo unreadable; exit 1", StatusRed, []string{"src/a.go:1"}, false},
		{"green-regression", "echo src/a.go:1; exit 1", StatusGreen, nil, false},
		{"unrunnable-skip", "exit 99", StatusUnrunnable, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, "check"), []byte("#!/bin/sh\n"+tc.body+"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			base := CommandResult{Command: c, Status: tc.status, Signature: tc.signature, ExitCode: &one}
			if tc.status == StatusGreen {
				base.ExitCode = &zero
			}
			got := RunLadder(context.Background(), Baseline{Results: []CommandResult{base}}, []Command{c}, root, []string{"src/a.go"}, []string{"."})
			if got.OK != tc.want {
				t.Fatalf("ladder=%+v", got)
			}
			if tc.status == StatusUnrunnable && len(got.Checks) != 0 {
				t.Fatal("unrunnable baseline executed")
			}
		})
	}
	r := RunCommand(context.Background(), Command{Argv: []string{"missing-refactor-fixture-command"}}, root, nil)
	if r.Status != StatusUnrunnable {
		t.Fatal(r)
	}
	if got := AreasForPaths([]string{".", "app", "app/web"}, []string{"app/web/a.ts"}); !reflect.DeepEqual(got, []string{".", "app/web"}) {
		t.Fatal(got)
	}
}

func TestMigrationGateContracts(t *testing.T) {
	clean := func() GateFacts {
		return GateFacts{Changed: []string{"src/a.go"}, PerFile: map[string]FileStat{}, AddedByFile: map[string][]string{}, ProspectiveTree: "new"}
	}
	packet := Packet{Category: "DEAD_CODE", Allowlist: []string{"src/a.go"}}
	for _, tc := range []struct {
		name, code string
		mutate     func(*GateFacts)
	}{
		{"head", "WORKTREE_HEAD_MOVED", func(f *GateFacts) { f.WorktreeHead = "changed" }},
		{"source", "SOURCE_MUTATED", func(f *GateFacts) { f.SourceHead = "changed" }},
		{"forbidden", "FORBIDDEN_PATH", func(f *GateFacts) { f.Changed = []string{"go.mod"} }},
		{"binary", "FORBIDDEN_BINARY", func(f *GateFacts) { f.Binary = []string{"src/a.go"} }},
		{"allowlist", "OUT_OF_SCOPE", func(f *GateFacts) { f.Changed = []string{"src/b.go"} }},
		{"size", "TOO_LARGE", func(f *GateFacts) { f.Insertions = 601 }},
		{"net", "NET_SIZE_RULE", func(f *GateFacts) { f.Insertions = 6 }},
		{"thrash", "THRASH_REVERT", func(f *GateFacts) { f.ProspectiveTree = "old" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := clean()
			tc.mutate(&f)
			got := RunChecks(f, packet, Policy{}, []string{"old"}, nil)
			if got.Violation == nil || got.Violation.Code != tc.code {
				t.Fatalf("%+v", got)
			}
		})
	}
	f := clean()
	if got := RunChecks(f, packet, Policy{}, nil, nil); got.Verdict != "PASS" || len(got.Checks) != 11 {
		t.Fatalf("%+v", got)
	}
	f.Changed = nil
	if got := RunChecks(f, packet, Policy{}, nil, nil); got.Verdict != "NO_OP" {
		t.Fatal(got)
	}
	for _, p := range []string{"pom.xml", "app/build.gradle.kts", "app/src/test/resources/a.txt", "testdata/a.txt", "a.lock", ".env", "a/package.json"} {
		if !matchesGlobs(p, ForbiddenGlobs) {
			t.Fatal(p)
		}
	}
	for _, p := range []string{"src/A.java", "src/A.kt"} {
		if matchesGlobs(p, ForbiddenGlobs) {
			t.Fatal(p)
		}
	}
	for _, p := range []string{"src/test/java/A.java", "src/AIT.java", "src/ATest.kt", "src/a_test.go", "test_a.py", "a.test.ts"} {
		if !IsTestPath(p, nil) {
			t.Fatal(p)
		}
	}
	for _, token := range []string{"t.Skip()", "test.skip()", "@Disabled", "@Ignore", "assumeTrue(false)", "enabled = false"} {
		f := clean()
		f.Changed = []string{"src/a_test.go"}
		f.AddedByFile = map[string][]string{"src/a_test.go": {token}}
		p := Packet{Allowlist: f.Changed}
		if got := RunChecks(f, p, Policy{}, nil, nil); got.Violation == nil || got.Violation.Code != "TEST_WEAKENED" {
			t.Fatal(token, got)
		}
	}
	for _, tc := range []struct {
		glob, path string
		want       bool
	}{{"**/x", "x", true}, {"**/x", "a/x", true}, {"*.go", "a/b.go", true}, {"a/*", "a/b/c", false}} {
		re, err := GlobRegexp(tc.glob)
		if err != nil || re.MatchString(tc.path) != tc.want {
			t.Fatal(tc, err)
		}
	}
	f = clean()
	if got := RunChecks(f, packet, Policy{}, nil, []string{"elsewhere"}); got.Violation == nil || got.Violation.Code != "TARGET_SCOPE_EMPTY" {
		t.Fatal(got)
	}
	if !strings.Contains(strings.Join(ForbiddenGlobs, " "), "gradle/wrapper") {
		t.Fatal("wrapper protection absent")
	}
}

func TestMigrationWorktreeArtifacts(t *testing.T) {
	root, wt, base, fp := fixtureRepo(t)
	if head, err := HeadOID(wt); err != nil || head != base {
		t.Fatal(head, err)
	}
	if branch, err := CurrentBranch(wt); err == nil || branch != "" {
		t.Fatal(branch, err)
	}
	if paths, err := ChangedPaths(wt); err != nil || len(paths) != 0 {
		t.Fatal(paths, err)
	}
	for name, body := range map[string]string{"compiled": "\x7fELF\x00\x01", "src/new.go": "package a\n", "build/report.xml": "<testsuite/>\n", "buildsystem/Keep.java": "class Keep {}\n", "empty": ""} {
		p := filepath.Join(wt, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	diff, err := DiffForReview(wt, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "package a") || !strings.Contains(diff, "compiled") || strings.Contains(diff, "\x00") {
		t.Fatal(diff)
	}
	swept, err := SweepBuildArtifacts(root, wt, nil)
	if err != nil || len(swept) != 1 {
		t.Fatal(swept, err)
	}
	if _, err := os.Stat(filepath.Join(wt, "build/report.xml")); err != nil {
		t.Fatal("text swept without output directory")
	}
	swept, err = SweepBuildArtifacts(root, wt, []string{"build"})
	if err != nil || len(swept) != 1 {
		t.Fatal(swept, err)
	}
	for _, name := range []string{"src/new.go", "buildsystem/Keep.java", "empty"} {
		if _, err := os.Stat(filepath.Join(wt, name)); err != nil {
			t.Fatal(name, err)
		}
	}
	if swept, err = SweepBuildArtifacts(root, wt, []string{"build"}); err != nil || len(swept) != 0 {
		t.Fatal(swept, err)
	}
	back, err := Rollback(root, wt, base)
	if err != nil || !back.Clean {
		t.Fatal(back, err)
	}
	if after, err := SourceFingerprint(root); err != nil || after != fp {
		t.Fatal("source changed", after, err)
	}
	if err := WorktreeRemove(root, wt); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationWorktreeSplitAndRollback(t *testing.T) {
	root, wt, base, fp := fixtureRepo(t)
	// This source-only split catches numstat implementations that omit untracked files.
	if err := os.WriteFile(filepath.Join(wt, "src/a.go"), []byte("package a\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "src/b.go"), []byte("package a\nfunc A() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	packet := Packet{Category: "LARGE_COMPONENT_SPLIT", Allowlist: []string{"src/a.go"}, OriginalLines: 2}
	facts, err := CollectFacts(wt, root, base, base, fp, packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Changed) != 2 || facts.Insertions < 2 {
		t.Fatal(facts)
	}
	if result := RunChecks(facts, packet, Policy{}, nil, nil); result.Verdict != "PASS" {
		t.Fatal(result)
	}
	back, err := Rollback(root, wt, base)
	if err != nil || !back.Clean {
		t.Fatal(back, err)
	}
	if err := os.Remove(filepath.Join(wt, "src/a.go")); err != nil {
		t.Fatal(err)
	}
	packet = Packet{Category: "DEAD_CODE", Allowlist: []string{"src/a.go"}}
	facts, err = CollectFacts(wt, root, base, base, fp, packet)
	if err != nil {
		t.Fatal(err)
	}
	if result := RunChecks(facts, packet, Policy{}, nil, nil); result.Verdict != "PASS" {
		t.Fatal(result)
	}
	if !slices.Contains(facts.Deleted, "src/a.go") {
		t.Fatal(facts)
	}
	if _, err := Rollback(root, wt, base); err != nil {
		t.Fatal(err)
	}
	if err := WorktreeRemove(root, wt); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationAuditDirectoriesAndSelfIgnore(t *testing.T) {
	root, wt, _, _ := fixtureRepo(t)
	dir, err := EnsureRefactorDir(root)
	if err != nil {
		t.Fatal(err)
	}
	one, err := AuditDirFor(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	two, err := AuditDirFor(dir, 2)
	if err != nil || one == two || filepath.Base(one) != "01" || filepath.Base(two) != "02" {
		t.Fatal(one, two, err)
	}
	if status, err := StatusPorcelain(root); err != nil || strings.TrimSpace(status) != "" {
		t.Fatal(status, err)
	}
	if err := WorktreeRemove(root, wt); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationNetSizeRules(t *testing.T) {
	for _, tc := range []struct {
		category        string
		ins, del, lines int
		want            bool
	}{{"DEAD_CODE", 6, 0, 0, false}, {"COMPATIBILITY_REMOVAL", 0, 5, 0, true}, {"DEDUPLICATION", 1, 0, 0, false}, {"DEDUPLICATION", 4, 4, 0, true}, {"LARGE_COMPONENT_SPLIT", 300, 300, 180, true}, {"LARGE_COMPONENT_SPLIT", 0, 600, 20, false}, {"LARGE_COMPONENT_SPLIT", 0, 0, 500, false}} {
		p := Packet{Category: tc.category, OriginalLines: 620}
		got, _ := NetSizeRule(p, GateFacts{Insertions: tc.ins, Deletions: tc.del, PrimaryFileLines: &tc.lines})
		if got != tc.want {
			t.Fatal(tc)
		}
	}
	if ChangedLineBudget(Packet{Category: "LARGE_COMPONENT_SPLIT", OriginalLines: 620}, Policy{MaxChangedLines: 600}) != 1550 {
		t.Fatal("split budget")
	}
	f := GateFacts{Changed: []string{"a_test.go"}, Deleted: []string{"a_test.go"}}
	p := Packet{Category: "DEAD_CODE", Allowlist: f.Changed}
	if got := RunChecks(f, p, Policy{}, nil, nil); got.Violation == nil || got.Violation.Code != "TEST_WEAKENED" {
		t.Fatal(got)
	}
	f = GateFacts{Changed: []string{"app/web/a.go", "src/caller.go"}}
	p.Allowlist = f.Changed
	if got := RunChecks(f, p, Policy{}, nil, []string{"app/web"}); got.Verdict != "PASS" {
		t.Fatal(got)
	}
	if !strings.Contains(WorktreePath("/repo", "run", ""), "/.cache/refactor-me/") || !strings.HasPrefix(WorktreePath("/repo", "run", "/custom"), "/custom/") {
		t.Fatal("worktree parent")
	}
}

func TestMigrationIgnoredHydrationAndBaseline(t *testing.T) {
	root, wt, base, _ := fixtureRepo(t)
	// Repository-local excludes apply to both worktrees without changing a tracked fixture file.
	exclude, err := Git(root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		t.Fatal(err)
	}
	exclude = strings.TrimSpace(exclude)
	if !filepath.IsAbs(exclude) {
		exclude = filepath.Join(root, exclude)
	}
	f, err := os.OpenFile(exclude, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("\nnode_modules/\n"); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	ignored := filepath.Join(wt, "node_modules/dep/binary")
	if err := os.MkdirAll(filepath.Dir(ignored), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignored, []byte{0, 1}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := SweepBuildArtifacts(root, wt, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ignored); err != nil {
		t.Fatal("ignored artifact swept")
	}
	if err := os.WriteFile(filepath.Join(wt, "src/a.go"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if back, err := Rollback(root, wt, base); err != nil || !back.Clean {
		t.Fatal(back, err)
	}
	if _, err := os.Stat(ignored); err != nil {
		t.Fatal("hydration removed")
	}
	commands := []Command{{ID: "green", Area: ".", Cwd: ".", Tier: TierFast, Argv: []string{"/usr/bin/true"}}, {ID: "red", Area: ".", Cwd: ".", Tier: TierFast, Argv: []string{"/bin/sh", "-c", "echo src/a.go:1; exit 1"}}}
	baseline := RunBaseline(context.Background(), commands, wt)
	if !baseline.Usable || baseline.Green != 1 || baseline.Red != 1 {
		t.Fatal(baseline)
	}
	if err := WorktreeRemove(root, wt); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRealGateRefusals(t *testing.T) {
	root, wt, base, fp := fixtureRepo(t)
	for _, tc := range []struct {
		name, path, body, code string
		packet                 Packet
	}{
		{"outside", "src/b.go", "package b\n", "OUT_OF_SCOPE", Packet{Category: "DEAD_CODE", Allowlist: []string{"src/a.go"}}},
		{"lock", "package-lock.json", "{}\n", "FORBIDDEN_PATH", Packet{Category: "DEAD_CODE", Allowlist: []string{"package-lock.json"}}},
		{"test-skip", "src/new_test.go", "package a\nfunc TestX(t *testing.T){t.Skip()}\n", "TEST_WEAKENED", Packet{Allowlist: []string{"src/new_test.go"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(wt, tc.path), []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			f, err := CollectFacts(wt, root, base, base, fp, tc.packet)
			if err != nil {
				t.Fatal(err)
			}
			got := RunChecks(f, tc.packet, Policy{}, nil, nil)
			if got.Violation == nil || got.Violation.Code != tc.code {
				t.Fatal(got)
			}
			if back, err := Rollback(root, wt, base); err != nil || !back.Clean {
				t.Fatal(back, err)
			}
		})
	}
	if err := os.WriteFile(filepath.Join(wt, "src/a.go"), []byte("package a\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := CollectFacts(wt, root, base, base, "stale", Packet{Allowlist: []string{"src/a.go"}})
	if err != nil {
		t.Fatal(err)
	}
	got := RunChecks(f, Packet{Allowlist: f.Changed}, Policy{}, nil, nil)
	if got.Verdict != "HALT" || got.Violation.Code != "SOURCE_MUTATED" {
		t.Fatal(got)
	}
	if _, err := Rollback(root, wt, base); err != nil {
		t.Fatal(err)
	}
	if err := WorktreeRemove(root, wt); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationAffectedAreaLadder(t *testing.T) {
	root := t.TempDir()
	for _, area := range []string{"app", "other"} {
		if err := os.Mkdir(filepath.Join(root, area), 0700); err != nil {
			t.Fatal(err)
		}
	}
	commands := []Command{{ID: "root", Area: ".", Cwd: ".", Tier: TierFast, Argv: []string{"/usr/bin/true"}}, {ID: "app", Area: "app", Cwd: "app", Tier: TierTest, Argv: []string{"/usr/bin/true"}}, {ID: "other", Area: "other", Cwd: "other", Tier: TierTest, Argv: []string{"/usr/bin/true"}}}
	baseline := RunBaseline(context.Background(), commands, root)
	if !baseline.Usable || len(baseline.Results) != 3 {
		t.Fatal(baseline)
	}
	commands[2].Argv = []string{"/usr/bin/false"}
	got := RunLadder(context.Background(), baseline, commands, root, []string{"app/a.go"}, []string{".", "app", "other"})
	if !got.OK || len(got.Checks) != 2 {
		t.Fatal(got)
	}
	commands[1].Argv = []string{"/usr/bin/false"}
	got = RunLadder(context.Background(), baseline, commands, root, []string{"app/a.go"}, []string{".", "app", "other"})
	if got.OK || got.Failed == nil || got.Failed.FailureKind != "REGRESSION" {
		t.Fatal(got)
	}
}
