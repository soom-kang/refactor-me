// Package catalog resolves and fingerprints the global Skills used by a run.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Required names are the stable Skill dependencies of the controller.
var Required = []string{"sharpen-clarify", "sharpen-review", "sharpen-challenge", "sharpen-assess", "sharpen-refine", "sharpen-cold-review", "sharpen-brief", "sharpen-dedupe"}

const maxFileBytes = 4 << 20
const maxTreeBytes = 32 << 20
const maxFiles = 4096

// Entry identifies the exact global source selected at run start.
type Entry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Catalog pins source contents and a private Claude delivery snapshot.
type Catalog struct {
	Entries    []Entry `json:"entries"`
	ClaudeRoot string  `json:"claudeRoot,omitempty"`
	root       string
	repo       string
	home       string
}

type file struct {
	path string
	data []byte
	mode os.FileMode
}

// Load validates the canonical global catalog and rejects conflicting discovery paths.
func Load(home, repo string) (*Catalog, error) {
	c := &Catalog{Entries: []Entry{}, root: filepath.Join(home, ".agents", "skills"), home: home, repo: repo}
	for _, name := range Required {
		path, err := filepath.EvalSymlinks(filepath.Join(c.root, name))
		if err != nil {
			return nil, fmt.Errorf("global skill %s: %w", name, err)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		files, sum, err := readTree(path, name)
		if err != nil {
			return nil, err
		}
		_ = files
		c.Entries = append(c.Entries, Entry{Name: name, Path: path, SHA256: sum})
	}
	if err := c.checkConflicts(repo); err != nil {
		return nil, err
	}
	return c, nil
}

// Snapshot creates a new run-owned directory, never replacing existing files.
func (c *Catalog) Snapshot(parent string) error {
	if err := c.Verify(); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	root, err := os.MkdirTemp(parent, "skills-")
	if err != nil {
		return err
	}
	for _, entry := range c.Entries {
		files, sum, err := readTree(entry.Path, entry.Name)
		if err != nil {
			return err
		}
		if sum != entry.SHA256 {
			return fmt.Errorf("global skill changed: %s", entry.Name)
		}
		for _, f := range files {
			dst := filepath.Join(root, ".claude", "skills", entry.Name, f.path)
			if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(dst, f.data, 0600|f.mode.Perm()&0111); err != nil {
				return err
			}
		}
	}
	c.ClaudeRoot = root
	return c.Verify()
}

// Verify detects changed sources, snapshots, links and discovery conflicts.
func (c *Catalog) Verify() error {
	if c == nil {
		return nil
	}
	for _, entry := range c.Entries {
		current, err := filepath.EvalSymlinks(filepath.Join(c.root, entry.Name))
		if err != nil || current != entry.Path {
			return fmt.Errorf("global skill source changed: %s", entry.Name)
		}
		_, sum, err := readTree(entry.Path, entry.Name)
		if err != nil {
			return err
		}
		if sum != entry.SHA256 {
			return fmt.Errorf("global skill changed: %s", entry.Name)
		}
		if c.ClaudeRoot != "" {
			_, sum, err = readTree(filepath.Join(c.ClaudeRoot, ".claude", "skills", entry.Name), entry.Name)
			if err != nil {
				return err
			}
			if sum != entry.SHA256 {
				return fmt.Errorf("skill snapshot changed: %s", entry.Name)
			}
		}
	}
	return c.checkConflicts(c.repo)
}

// CheckWorkspace checks discovery roots in a newly opened isolated checkout.
func (c *Catalog) CheckWorkspace(repo string) error {
	if c == nil {
		return nil
	}
	return c.checkConflicts(repo)
}

func (c *Catalog) checkConflicts(repo string) error {
	roots := []string{c.root, filepath.Join(c.home, ".claude", "skills"), filepath.Join(c.home, ".codex", "skills"), "/etc/codex/skills"}
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		roots = append(roots, filepath.Join(dir, "skills"))
	}
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		roots = append(roots, filepath.Join(dir, "skills"))
	}
	// Providers discover nested Skills and parent-level project Skills. Walk the
	// project directories, excluding dependency and VCS internals.
	for dir := repo; dir != ""; dir = filepath.Dir(dir) {
		roots = append(roots, filepath.Join(dir, ".agents", "skills"), filepath.Join(dir, ".claude", "skills"))
		if filepath.Dir(dir) == dir {
			break
		}
	}
	err := filepath.WalkDir(repo, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".agents" || d.Name() == ".claude" {
			roots = append(roots, filepath.Join(path, "skills"))
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != repo && (d.Name() == ".git" || d.Name() == ".refactor" || d.Name() == "node_modules" || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("inspect project skill conflicts: %w", err)
	}
	seenRoots := map[string]bool{}
	for _, root := range roots {
		if seenRoots[root] {
			continue
		}
		seenRoots[root] = true
		children, err := os.ReadDir(root)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect skill root %s: %w", root, err)
		}
		for _, child := range children {
			candidate := filepath.Join(root, child.Name())
			name := discoveredName(candidate)
			for _, entry := range c.Entries {
				if name != entry.Name {
					continue
				}
				real, err := filepath.EvalSymlinks(candidate)
				if err != nil || real != entry.Path {
					return fmt.Errorf("conflicting skill %s at %s", entry.Name, candidate)
				}
			}
		}
		for _, entry := range c.Entries {
			candidate := filepath.Join(root, entry.Name)
			_, err := os.Lstat(candidate)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			real, err := filepath.EvalSymlinks(candidate)
			if err != nil || real != entry.Path {
				return fmt.Errorf("conflicting skill %s at %s", entry.Name, candidate)
			}
		}
	}
	return nil
}

// discoveredName also catches an alias stored under a different directory name.
func discoveredName(path string) string {
	root, err := os.OpenRoot(path)
	if err != nil {
		return ""
	}
	defer root.Close()
	info, err := root.Stat("SKILL.md")
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	f, err := root.Open("SKILL.md")
	if err != nil {
		return ""
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64<<10))
	if err != nil {
		return ""
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return ""
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return ""
	}
	for _, line := range strings.Split(text[4:4+end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && key == "name" {
			return strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	return ""
}

// Prompt lists exact source paths rather than allowing name-only substitution.
func (c *Catalog) Prompt(provider string) string {
	if c == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nRequired Skills for this execution (read these exact files and their referenced resources):\n")
	for _, entry := range c.Entries {
		path := filepath.Join(entry.Path, "SKILL.md")
		if provider == "claude" {
			path = filepath.Join(c.ClaudeRoot, ".claude", "skills", entry.Name, "SKILL.md")
		}
		fmt.Fprintf(&b, "- %s: %s\n", entry.Name, path)
	}
	return b.String()
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func readTree(root, name string) ([]file, string, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("skill directory is not a directory: %s", root)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, "", err
	}
	confined, err := os.OpenRoot(root)
	if err != nil {
		return nil, "", err
	}
	defer confined.Close()
	files := []file{}
	total := int64(0)
	entriesSeen := 0
	var walk func(string, string, map[string]bool) error
	walk = func(actual, rel string, ancestors map[string]bool) error {
		entriesSeen++
		if entriesSeen > maxFiles*2 || len(ancestors) > 64 {
			return fmt.Errorf("skill tree exceeds entry or depth limit: %s", name)
		}
		resolved, err := filepath.EvalSymlinks(actual)
		if err != nil {
			return err
		}
		if !within(root, resolved) {
			return fmt.Errorf("skill link escapes directory: %s", actual)
		}
		st, err := os.Stat(resolved)
		if err != nil {
			return err
		}
		if st.IsDir() {
			if ancestors[resolved] {
				return fmt.Errorf("skill symlink cycle: %s", actual)
			}
			next := map[string]bool{}
			for k, v := range ancestors {
				next[k] = v
			}
			next[resolved] = true
			entries, err := os.ReadDir(resolved)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if err := walk(filepath.Join(resolved, e.Name()), filepath.Join(rel, e.Name()), next); err != nil {
					return err
				}
			}
			return nil
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("unsupported skill file: %s", actual)
		}
		if st.Size() > maxFileBytes || total+st.Size() > maxTreeBytes || len(files) >= maxFiles {
			return fmt.Errorf("skill exceeds size limit: %s", name)
		}
		resolvedRel, err := filepath.Rel(root, resolved)
		if err != nil {
			return err
		}
		f, err := confined.Open(resolvedRel)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
		closeErr := f.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}
		total += int64(len(data))
		if len(data) > maxFileBytes || total > maxTreeBytes {
			return fmt.Errorf("skill exceeds size limit: %s", name)
		}
		files = append(files, file{path: rel, data: data, mode: st.Mode()})
		return nil
	}
	if err := walk(root, "", map[string]bool{}); err != nil {
		return nil, "", fmt.Errorf("skill %s: %w", name, err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	found := false
	h := sha256.New()
	for _, f := range files {
		if f.path == "SKILL.md" {
			if err := frontmatter(f.data, name); err != nil {
				return nil, "", err
			}
			found = true
		}
		fmt.Fprintf(h, "%d:%s:%d:%o:", len(filepath.ToSlash(f.path)), filepath.ToSlash(f.path), len(f.data), f.mode.Perm()&0111)
		h.Write(f.data)
	}
	if !found {
		return nil, "", fmt.Errorf("skill %s lacks SKILL.md", name)
	}
	return files, hex.EncodeToString(h.Sum(nil)), nil
}

func frontmatter(data []byte, name string) error {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return fmt.Errorf("skill %s lacks frontmatter", name)
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return fmt.Errorf("skill %s has unterminated frontmatter", name)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(text[4:4+end], "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if ok {
			fields[key] = strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	if fields["name"] != name || fields["description"] == "" {
		return fmt.Errorf("skill %s needs matching name and nonempty description", name)
	}
	if strings.TrimSpace(text[4+end+5:]) == "" {
		return fmt.Errorf("skill %s has no instructions", name)
	}
	return nil
}
