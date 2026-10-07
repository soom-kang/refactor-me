package controller

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
)

func TestDeclaredDeletionStaysWithinWorktree(t *testing.T) {
	for _, name := range []string{"file", "missing", "final-symlink", "intermediate-symlink", "parent", "absolute", "directory"} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			wt := filepath.Join(base, "worktree")
			if err := os.Mkdir(wt, 0755); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(base, "sentinel.txt")
			if err := os.WriteFile(sentinel, []byte("preserve\n"), 0644); err != nil {
				t.Fatal(err)
			}
			path, wantError := "file.txt", false
			switch name {
			case "file":
				if err := os.WriteFile(filepath.Join(wt, path), []byte("remove\n"), 0644); err != nil {
					t.Fatal(err)
				}
			case "final-symlink", "intermediate-symlink":
				if err := os.Symlink(base, filepath.Join(wt, "link")); err != nil {
					t.Fatal(err)
				}
				path = "link"
				if name == "intermediate-symlink" {
					path, wantError = "link/sentinel.txt", true
				}
			case "parent":
				path, wantError = "../sentinel.txt", true
			case "absolute":
				path, wantError = sentinel, true
			case "directory":
				path, wantError = ".", true
			}
			if err := removeDeclaredFiles(wt, []string{path}); (err != nil) != wantError {
				t.Fatalf("deletion error = %v, want error %v", err, wantError)
			}
			if data, err := os.ReadFile(sentinel); err != nil || string(data) != "preserve\n" {
				t.Fatalf("external file changed: %q: %v", data, err)
			}
			if !wantError {
				if _, err := os.Lstat(filepath.Join(wt, path)); !os.IsNotExist(err) {
					t.Fatalf("declared file remains: %v", err)
				}
			}
		})
	}
}

func comparisonFixture(t *testing.T) (*runner, func(string, string), func() string) {
	t.Helper()
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	gitTest(t, repo, "config", "commit.gpgsign", "false")
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func() string {
		t.Helper()
		gitTest(t, repo, "add", "-A")
		gitTest(t, repo, "commit", "-qm", "fixture", "--allow-empty")
		return gitTest(t, repo, "rev-parse", "HEAD")
	}
	return &runner{runDir: t.TempDir(), state: &runState{RepoRoot: repo}}, write, commit
}

func TestComparisonContracts(t *testing.T) {
	r, write, commit := comparisonFixture(t)
	if got := r.collectComparison()["status"]; got != "NO_CHANGES" {
		t.Fatal(got)
	}
	if md, err := os.ReadFile(filepath.Join(r.runDir, "changes.md")); err != nil || !strings.Contains(string(md), "No committed code changes") {
		t.Fatal("missing no-change Markdown", err)
	}
	write("modify.txt", "old\n")
	write("remove.txt", "remove me\n")
	write("rename.txt", "rename stays\n")
	write("executable", "#!/bin/sh\n")
	write("binary.dat", string([]byte{0, 1, 2}))
	r.state.BaseOID = commit()
	write("characterization.txt", "evidence\n")
	commit()
	write("modify.txt", "intermediate\n")
	commit()
	write("modify.txt", "final\n")
	if err := os.Remove(filepath.Join(r.state.RepoRoot, "remove.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(r.state.RepoRoot, "rename.txt"), filepath.Join(r.state.RepoRoot, "이름 | `x`\t새\n파일.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(r.state.RepoRoot, "executable"), 0755); err != nil {
		t.Fatal(err)
	}
	write("binary.dat", string([]byte{0, 3, 4}))
	r.state.PublishedOID = commit()
	c := r.collectComparison()
	if c["status"] != "AVAILABLE" {
		t.Fatal(c)
	}
	if !reflect.DeepEqual(c["totals"], map[string]int{"files": 6, "insertions": 2, "deletions": 2, "binary": 1}) {
		t.Fatal(c["totals"])
	}
	patch, err := os.ReadFile(filepath.Join(r.runDir, "changes.patch"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(patch), "intermediate") || strings.Contains(string(patch), "GIT binary patch") || !strings.Contains(string(patch), "+final") {
		t.Fatal(string(patch))
	}
	markdown, err := os.ReadFile(filepath.Join(r.runDir, "changes.md"))
	if err != nil || c["markdownFile"] != "changes.md" || strings.Count(string(markdown), "- [ ]") != 6 || !strings.Contains(string(markdown), string(patch)) || !strings.Contains(string(markdown), "binary") || !strings.Contains(string(markdown), "파일 6개") && !strings.Contains(string(markdown), "6 files") {
		t.Fatal("invalid complete changes Markdown", err)
	}
	write("rejected.txt", "uncommitted\n")
	if !reflect.DeepEqual(c, r.collectComparison()) {
		t.Fatal("dirty checkout changed comparison")
	}
	write("modify.txt", "later unrelated\n")
	commit()
	if !reflect.DeepEqual(c, r.collectComparison()) {
		t.Fatal("later HEAD changed comparison")
	}
	gitTest(t, r.state.RepoRoot, "config", "diff.external", "/does-not-exist")
	gitTest(t, r.state.RepoRoot, "config", "diff.custom.textconv", "/does-not-exist")
	if !reflect.DeepEqual(c, r.collectComparison()) {
		t.Fatal("external diff changed comparison")
	}
}

func TestComparisonLimitsAndFailures(t *testing.T) {
	for name, body := range map[string]string{"lines": strings.Repeat("line\n", 400), "bytes": strings.Repeat(strings.Repeat("한글", 600)+"\n", 40), "oversized": strings.Repeat("한", 40000) + "\n"} {
		t.Run(name, func(t *testing.T) {
			r, write, commit := comparisonFixture(t)
			r.state.BaseOID = commit()
			write("large.txt", body)
			r.state.PublishedOID = commit()
			c := r.collectComparison()
			p, _ := c["preview"].(string)
			if c["status"] != "AVAILABLE" || c["truncated"] != true || len(p) > 32768 || strings.Count(p, "\n") > 200 || !utf8.ValidString(p) || !strings.HasSuffix(p, "\n") {
				t.Fatalf("invalid preview: %+v", c)
			}
			patch, err := os.ReadFile(filepath.Join(r.runDir, "changes.patch"))
			if err != nil || !strings.HasPrefix(string(patch), p) {
				t.Fatal("patch mismatch", err)
			}
			markdown, err := os.ReadFile(filepath.Join(r.runDir, "changes.md"))
			if err != nil || !strings.Contains(string(markdown), string(patch)) {
				t.Fatal("Markdown truncated the full patch", err)
			}
		})
	}
	r, write, commit := comparisonFixture(t)
	write("x", "before\n")
	r.state.BaseOID = commit()
	write("x", "after\n")
	valid := commit()
	for _, oid := range []string{"HEAD", strings.Repeat("f", 40)} {
		r.state.PublishedOID = oid
		c := r.collectComparison()
		if c["status"] != "UNAVAILABLE" || c["patchFile"] != nil {
			t.Fatal(c)
		}
	}
	r.state.PublishedOID = valid
	if err := os.Remove(filepath.Join(r.runDir, "changes.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(r.runDir, "changes.md"), 0755); err != nil {
		t.Fatal(err)
	}
	if c := r.collectComparison(); c["status"] != "AVAILABLE" || c["markdownFile"] != nil || c["markdownError"] == nil || c["patchFile"] != "changes.patch" {
		t.Fatal("Markdown failure hid or altered the committed comparison", c)
	}
	if err := os.Remove(filepath.Join(r.runDir, "changes.patch")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(r.runDir, "changes.patch"), 0755); err != nil {
		t.Fatal(err)
	}
	if c := r.collectComparison(); c["status"] != "UNAVAILABLE" {
		t.Fatal(c)
	}
	r.runDir = t.TempDir()
	write("x", "before\n")
	r.state.PublishedOID = commit()
	if c := r.collectComparison(); c["status"] != "NO_CHANGES" || c["preview"] != "" {
		t.Fatal(c)
	}
}

func TestDoctorGlobalSkillsAndCleanup(t *testing.T) {
	for _, mode := range []string{"global", "missing", "project-conflict"} {
		t.Run(mode, func(t *testing.T) {
			home := globalSkillsFixture(t)
			r, write, commit := comparisonFixture(t)
			write("source.txt", "base\n")
			if mode == "missing" {
				if err := os.Remove(filepath.Join(home, ".agents", "skills", requiredSkills[0], "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "project-conflict" {
				write(".claude/skills/"+requiredSkills[0]+"/SKILL.md", "different")
			}
			commit()
			cfg := surface.DefaultConfig()
			cfg["agents"] = map[string]any{"claude": map[string]any{"bin": "/usr/bin/true"}}
			before := gitTest(t, r.state.RepoRoot, "worktree", "list", "--porcelain")
			report, err := runDoctor(surface.Context{Repo: r.state.RepoRoot, Config: cfg, Providers: []string{"claude"}}, "")
			if err != nil {
				t.Fatal(err)
			}
			if report.OK != (mode == "global") {
				b, _ := json.Marshal(report)
				t.Fatal(string(b))
			}
			if got := gitTest(t, r.state.RepoRoot, "worktree", "list", "--porcelain"); got != before {
				t.Fatal("probe worktree leaked")
			}
		})
	}
}

func TestDoctorRendering(t *testing.T) {
	cases := []struct {
		report doctorReport
		want   []string
	}{
		{doctorReport{OK: true, Healthy: []string{"codex"}, Excluded: []string{"claude"}, Checks: []doctorCheck{{ID: "claude-skills", Status: "FAIL", Detail: "missing", Fix: "check project skills"}}}, []string{"doctor: OK", "healthy providers: codex; excluded: claude", "[x]", "check project skills"}},
		{doctorReport{OK: false, Excluded: []string{"codex", "claude"}}, []string{"doctor: BLOCKED", "healthy providers: none; excluded: codex, claude"}},
		{doctorReport{OK: true, Healthy: []string{"codex", "claude"}}, []string{"healthy providers: codex, claude)"}},
	}
	for _, tc := range cases {
		out := renderDoctor(tc.report)
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Fatalf("missing %s in %s", want, out)
			}
		}
	}
}

func TestDoctorLiveFixtureProviderIsolation(t *testing.T) {
	visibleData, _ := json.Marshal(requiredSkills)
	for _, visible := range []string{string(visibleData), `[]`} {
		t.Run(visible, func(t *testing.T) {
			r, write, commit := comparisonFixture(t)
			globalSkillsFixture(t)
			write("source.txt", "source\n")
			commit()
			dir := t.TempDir()
			good := filepath.Join(dir, "codex")
			bad := filepath.Join(dir, "claude")
			body := "#!/bin/sh\nout=''\nwhile [ $# -gt 0 ]; do\nif [ \"$1\" = --output-last-message ]; then shift; out=$1; fi\nshift\ndone\nprintf '%s' '{\"answer\":7,\"visible_skills\":" + visible + ",\"wrote_file\":false}' > \"$out\"\n"
			if err := os.WriteFile(good, []byte(body), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bad, []byte("#!/bin/sh\necho 'authentication failed' >&2\nexit 1\n"), 0755); err != nil {
				t.Fatal(err)
			}
			cfg := surface.DefaultConfig()
			cfg["agents"] = map[string]any{"codex": map[string]any{"bin": good}, "claude": map[string]any{"bin": bad}}
			report, err := runDoctor(surface.Context{Repo: r.state.RepoRoot, Config: cfg, Providers: []string{"codex", "claude"}, Args: surface.Args{Live: true}}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if report.OK != (visible != "[]") {
				t.Fatalf("unexpected provider health %+v", report)
			}
			if visible != "[]" && !reflect.DeepEqual(report.Healthy, []string{"codex"}) {
				t.Fatal(report)
			}
		})
	}
}

func TestComparisonAfterWorktreeRemoval(t *testing.T) {
	r, write, commit := comparisonFixture(t)
	write("source.txt", "original\n")
	r.state.BaseOID = commit()
	wt := filepath.Join(t.TempDir(), "worktree")
	gitTest(t, r.state.RepoRoot, "worktree", "add", "--detach", wt, r.state.BaseOID)
	if err := os.WriteFile(filepath.Join(wt, "characterization.txt"), []byte("evidence\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, wt, "add", ".")
	gitTest(t, wt, "commit", "-qm", "characterization")
	r.state.PublishedOID = gitTest(t, wt, "rev-parse", "HEAD")
	gitTest(t, r.state.RepoRoot, "update-ref", "refs/heads/result", r.state.PublishedOID)
	gitTest(t, r.state.RepoRoot, "worktree", "remove", wt)
	c := r.collectComparison()
	if c["status"] != "AVAILABLE" {
		t.Fatal(c)
	}
	files := c["files"].([]map[string]any)
	if len(files) != 1 || files[0]["path"] != "characterization.txt" {
		t.Fatal(files)
	}
}

func TestDoctorProbeFailure(t *testing.T) {
	globalSkillsFixture(t)
	r, write, commit := comparisonFixture(t)
	write("source.txt", "base\n")
	commit()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \" $* \" in *' worktree add '*) echo probe-denied >&2; exit 1;; esac\nexec '" + strings.ReplaceAll(realGit, "'", "'\"'\"'") + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg := surface.DefaultConfig()
	cfg["agents"] = map[string]any{"claude": map[string]any{"bin": "/usr/bin/true"}}
	report, err := runDoctor(surface.Context{Repo: r.state.RepoRoot, Config: cfg, Providers: []string{"claude"}}, "")
	if err != nil || report.OK {
		t.Fatal(report, err)
	}
	failed := false
	referenceChecked := false
	for _, c := range report.Checks {
		if c.ID == "git-worktree" && c.Status == "FAIL" && c.Blocking {
			failed = true
		}
		if strings.HasSuffix(c.ID, "-skills") && c.ID != "global-skills" {
			t.Fatal("unmeasured skill check reported", c)
		}
		if c.ID == "skill-reference" {
			referenceChecked = c.Status == "WARN" && !c.Blocking && strings.Contains(c.Detail, "unverified/custom")
		}
	}
	if !failed || !referenceChecked {
		t.Fatal(report)
	}
}

func TestReportGenerationLanguageNeutral(t *testing.T) {
	r, _, _ := comparisonFixture(t)
	r.state = newState("r", r.state.RepoRoot, "/worktree", "", "main", "", nil, nil)
	r.state.finish("NO_CHANGES", "the audit proposed no candidates")
	r.started = time.Now()
	var first map[string]any
	for _, lang := range []string{"en", "ko"} {
		r.surface.Args.Language = lang
		result, err := r.result()
		if err != nil {
			t.Fatal(err)
		}
		var report map[string]any
		if err := json.Unmarshal(result.Report, &report); err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = report
		} else if !reflect.DeepEqual(first, report) {
			t.Fatal("language changed saved JSON")
		}
		md, err := os.ReadFile(filepath.Join(r.runDir, "report.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "## Committed changes"
		if lang == "ko" {
			want = "## 커밋된 변경"
		}
		if !strings.Contains(string(md), want) {
			t.Fatal(string(md))
		}
	}
}
