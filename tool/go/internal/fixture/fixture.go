// Package fixture creates isolated synthetic repositories for development.
package fixture

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed templates/js
var templates embed.FS

type Options struct {
	Destination string
	Language    string
	Skills      string
	Multi       bool
}

// Parse accepts flags before or after the optional destination.
func Parse(args []string) (Options, error) {
	o := Options{Language: "go"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--multi":
			o.Multi = true
		case "--lang", "--skills":
			flag := args[i]
			i++
			if i == len(args) || strings.HasPrefix(args[i], "--") {
				return o, fmt.Errorf("%s requires a value", flag)
			}
			if flag == "--lang" {
				o.Language = args[i]
			} else {
				o.Skills = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return o, fmt.Errorf("unknown option: %s", args[i])
			}
			if o.Destination != "" {
				return o, errors.New("only one destination is allowed")
			}
			o.Destination = args[i]
		}
	}
	if o.Language != "go" && o.Language != "js" {
		return o, errors.New("--lang must be go or js")
	}
	return o, nil
}

// Create never overwrites an existing destination. On failure it leaves the new
// directory for diagnosis and returns its path; callers must not delete it blindly.
func Create(ctx context.Context, o Options) (string, error) {
	if o.Language != "go" && o.Language != "js" {
		return "", errors.New("language must be go or js")
	}
	if o.Skills != "" {
		if err := validateSkills(o.Skills, o.Destination); err != nil {
			return "", err
		}
	}
	var dest string
	var err error
	if o.Destination == "" {
		dest, err = os.MkdirTemp("", "refactor-fixture-")
	} else {
		dest, err = filepath.Abs(o.Destination)
		if err == nil {
			err = os.Mkdir(dest, 0755)
		}
	}
	if err != nil {
		return "", fmt.Errorf("create destination (must not exist): %w", err)
	}
	write := func(root string) error {
		if o.Language == "go" {
			return writeGo(root)
		}
		return fs.WalkDir(templates, "templates/js", func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			b, e := templates.ReadFile(path)
			if e != nil {
				return e
			}
			rel := strings.TrimSuffix(strings.TrimPrefix(path, "templates/js/"), ".txt")
			rel = strings.ReplaceAll(rel, "DOT.", ".")
			return writeFile(root, rel, string(b))
		})
	}
	if err = write(dest); err != nil {
		return dest, err
	}
	if o.Multi {
		for _, area := range []string{"app/api", "app/worker"} {
			if err = write(filepath.Join(dest, area)); err != nil {
				return dest, err
			}
		}
	}
	if o.Skills != "" {
		if err = copySkills(o.Skills, dest); err != nil {
			return dest, err
		}
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "fixture"}, {"config", "user.email", "fixture@local"}, {"add", "-A"}, {"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "chore: fixture baseline"}} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dest
		// Do not inherit a caller's repository/index override into the fixture.
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, "GIT_") {
				cmd.Env = append(cmd.Env, env)
			}
		}
		output, e := cmd.CombinedOutput()
		if e != nil {
			return dest, fmt.Errorf("git %s: %w: %s", args[0], e, output)
		}
	}
	return dest, nil
}

func writeFile(root, rel, body string) error {
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0644)
}

// validateSkills rejects symlinks before any fixture files are written. Resolve
// the destination's existing parent to catch aliases into the source tree too.
func validateSkills(source, dest string) error {
	source = filepath.Clean(source)
	st, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("skill symlinks are not copied: %s", source)
	}
	if !st.IsDir() {
		return errors.New("skills source is not a directory")
	}
	canonicalSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	canonicalSource, err = filepath.Abs(canonicalSource)
	if err != nil {
		return err
	}
	parent := os.TempDir()
	if dest != "" {
		parent = filepath.Dir(dest)
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return err
	}
	candidate := parent
	if dest != "" {
		candidate = filepath.Join(parent, filepath.Base(dest))
	}
	rel, err := filepath.Rel(canonicalSource, candidate)
	if err != nil {
		return err
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("fixture destination must be outside the skills source")
	}
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill symlinks are not copied: %s", path)
		}
		return nil
	})
}

func copySkills(source, dest string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill symlinks are not copied: %s", from)
		}
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(from, "SKILL.md")); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		target := filepath.Join(dest, ".agents", "skills", entry.Name())
		err = filepath.WalkDir(from, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("skill symlinks are not copied: %s", path)
			}
			rel, e := filepath.Rel(from, path)
			if e != nil {
				return e
			}
			if d.IsDir() {
				return os.MkdirAll(filepath.Join(target, rel), 0755)
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("unsupported skill file: %s", path)
			}
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if e = writeFile(target, rel, string(b)); e != nil {
				return e
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			return os.Chmod(filepath.Join(target, rel), info.Mode().Perm())
		})
		if err != nil {
			return err
		}
		link := filepath.Join(dest, ".claude", "skills", entry.Name())
		if err = os.MkdirAll(filepath.Dir(link), 0755); err != nil {
			return err
		}
		if err = os.Symlink(filepath.Join("..", "..", ".agents", "skills", entry.Name()), link); err != nil {
			return err
		}
	}
	return nil
}
