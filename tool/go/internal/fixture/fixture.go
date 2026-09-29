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
	Multi       bool
}

// Parse accepts flags before or after the optional destination.
func Parse(args []string) (Options, error) {
	o := Options{Language: "go"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--multi":
			o.Multi = true
		case "--lang":
			flag := args[i]
			i++
			if i == len(args) || strings.HasPrefix(args[i], "--") {
				return o, fmt.Errorf("%s requires a value", flag)
			}
			o.Language = args[i]
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
