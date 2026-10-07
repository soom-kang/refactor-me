package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func fixture(t *testing.T) (string, string) {
	t.Helper()
	home, repo := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	for _, name := range Required {
		put(t, filepath.Join(home, ".agents", "skills", name, "SKILL.md"), "---\nname: "+name+"\ndescription: test skill\n---\nRead references/guide.md.\n")
		put(t, filepath.Join(home, ".agents", "skills", name, "references", "guide.md"), "Reference instructions.\n")
	}
	return home, repo
}
func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func TestLoadSnapshotAndDrift(t *testing.T) {
	home, repo := fixture(t)
	c, err := Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 8 {
		t.Fatal(c)
	}
	if status, detail := c.ReferenceCheck(); status != "WARN" || !strings.Contains(detail, "unverified/custom") {
		t.Fatal(status, detail)
	}
	if err := c.Snapshot(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Prompt("codex"), c.Entries[0].Path) || !strings.Contains(c.Prompt("claude"), c.ClaudeRoot) {
		t.Fatal("missing explicit paths")
	}
	snapshot := filepath.Join(c.ClaudeRoot, ".claude", "skills", Required[0], "references", "guide.md")
	put(t, snapshot, "mutated")
	if err := c.Verify(); err == nil || !strings.Contains(err.Error(), "snapshot changed") {
		t.Fatal(err)
	}
	put(t, snapshot, "Reference instructions.\n")
	put(t, filepath.Join(c.Entries[0].Path, "references", "guide.md"), "global changed")
	if err := c.Verify(); err == nil || !strings.Contains(err.Error(), "global skill changed") {
		t.Fatal(err)
	}
}

func TestReferenceCheck(t *testing.T) {
	var ref referenceManifest
	if err := json.Unmarshal(referenceJSON, &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Skills) != len(Required) || ref.HashAlgorithm != "catalog-tree-sha256-v1" || ref.LiveCompatibility != "NOT_RUN" {
		t.Fatal(ref)
	}
	c := &Catalog{}
	for _, name := range Required {
		if len(ref.Skills[name]) != 64 {
			t.Fatal("missing reference hash", name)
		}
		c.Entries = append(c.Entries, Entry{Name: name, SHA256: ref.Skills[name]})
	}
	if status, detail := c.ReferenceCheck(); status != "PASS" || !strings.Contains(detail, ref.Revision) || !strings.Contains(detail, "NOT_RUN") {
		t.Fatal(status, detail)
	}
	c.Entries[0].SHA256 = "changed"
	if status, detail := c.ReferenceCheck(); status != "WARN" || !strings.Contains(detail, Required[0]) {
		t.Fatal(status, detail)
	}
	c = nil
	if status, _ := c.ReferenceCheck(); status != "FAIL" {
		t.Fatal(status)
	}
}
func TestInvalidCatalog(t *testing.T) {
	for _, mode := range []string{"missing", "invalid-name", "empty-description", "empty-body", "escaping-file", "escaping-dir", "cycle", "project-conflict", "nested-conflict", "global-conflict", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			home, repo := fixture(t)
			skill := filepath.Join(home, ".agents", "skills", Required[0])
			switch mode {
			case "missing":
				if err := os.Remove(filepath.Join(skill, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			case "invalid-name":
				put(t, filepath.Join(skill, "SKILL.md"), "---\nname: wrong\ndescription: test\n---\nbody\n")
			case "empty-description":
				put(t, filepath.Join(skill, "SKILL.md"), "---\nname: "+Required[0]+"\ndescription:\n---\nbody\n")
			case "empty-body":
				put(t, filepath.Join(skill, "SKILL.md"), "---\nname: "+Required[0]+"\ndescription: test\n---\n")
			case "escaping-file", "escaping-dir":
				outside := t.TempDir()
				if mode == "escaping-file" {
					outside = filepath.Join(outside, "outside.md")
					put(t, outside, "outside")
				}
				if err := os.Symlink(outside, filepath.Join(skill, "escape")); err != nil {
					t.Fatal(err)
				}
			case "cycle":
				if err := os.Symlink(skill, filepath.Join(skill, "cycle")); err != nil {
					t.Fatal(err)
				}
			case "project-conflict":
				put(t, filepath.Join(repo, ".agents", "skills", Required[0], "SKILL.md"), "duplicate")
			case "nested-conflict":
				put(t, filepath.Join(repo, "app", ".claude", "skills", Required[0], "SKILL.md"), "duplicate")
			case "global-conflict":
				put(t, filepath.Join(home, ".claude", "skills", Required[0], "SKILL.md"), "duplicate")
			case "oversize":
				put(t, filepath.Join(skill, "large.txt"), strings.Repeat("x", maxFileBytes+1))
			}
			if _, err := Load(home, repo); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}
func TestCanonicalRootAndAliasSymlinks(t *testing.T) {
	home, repo := fixture(t)
	canonical := filepath.Join(home, ".agents", "skills", Required[0])
	moved := filepath.Join(home, "actual-skill")
	if err := os.Rename(canonical, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, canonical); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, ".claude", "skills", Required[0])
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(canonical, alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("references/guide.md", filepath.Join(moved, "guide.md")); err != nil {
		t.Fatal(err)
	}
	c, err := Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Snapshot(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(canonical); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), canonical); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(); err == nil {
		t.Fatal("changed source alias accepted")
	}
}

func TestCanonicalCatalogRejectsRenamedDuplicate(t *testing.T) {
	home, repo := fixture(t)
	put(t, filepath.Join(home, ".agents", "skills", "different-folder", "SKILL.md"), "---\nname: "+Required[0]+"\ndescription: duplicate\n---\nbody\n")
	if _, err := Load(home, repo); err == nil || !strings.Contains(err.Error(), "conflicting skill") {
		t.Fatalf("duplicate accepted: %v", err)
	}
}

func TestDiscoverySkipsSpecialSkillFiles(t *testing.T) {
	home, repo := fixture(t)
	path := filepath.Join(home, ".agents", "skills", "unrelated", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(home, repo); err != nil {
		t.Fatal(err)
	}
}
