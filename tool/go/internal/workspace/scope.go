package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var sourceExt = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".go": true, ".py": true, ".rs": true, ".java": true, ".kt": true, ".rb": true, ".swift": true, ".vue": true, ".svelte": true}

func SourceFiles(tracked []string) []string {
	var out []string
	for _, p := range tracked {
		if !sourceExt[filepath.Ext(p)] {
			continue
		}
		parts := strings.Split(filepath.ToSlash(p), "/")
		skip := false
		for _, part := range parts[:len(parts)-1] {
			if part == "node_modules" || part == "vendor" || part == "dist" || part == "build" {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, p)
		}
	}
	return out
}

func IsUnderTarget(p string, targets []string) bool {
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		if p == t || strings.HasPrefix(p, t+"/") {
			return true
		}
	}
	return false
}
func TouchesTarget(paths, targets []string) bool {
	if len(targets) == 0 {
		return true
	}
	for _, p := range paths {
		if IsUnderTarget(p, targets) {
			return true
		}
	}
	return false
}

// NormalizeTargets interprets inputs relative to cwd and rejects paths outside root.
func NormalizeTargets(root string, inputs []string, cwd string) ([]string, []string) {
	var targets, problems []string
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, []string{err.Error()}
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return nil, []string{err.Error()}
	}
	for _, raw := range inputs {
		abs := raw
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, raw)
		}
		abs, err = filepath.Abs(abs)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		real, realErr := filepath.EvalSymlinks(abs)
		if realErr == nil {
			abs = real
		}
		rel, err := filepath.Rel(rootReal, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			problems = append(problems, fmt.Sprintf("--target %s: %s is outside the repository (%s)", raw, abs, root))
			continue
		}
		if rel == "." {
			problems = append(problems, fmt.Sprintf("--target %s: that is the repository root", raw))
			continue
		}
		st, err := os.Stat(abs)
		if err != nil {
			problems = append(problems, fmt.Sprintf("--target %s: %s does not exist", raw, abs))
			continue
		}
		if !st.IsDir() {
			problems = append(problems, fmt.Sprintf("--target %s: %s is not a directory", raw, abs))
			continue
		}
		targets = append(targets, filepath.ToSlash(rel))
	}
	slices.Sort(targets)
	targets = slices.Compact(targets)
	filtered := make([]string, 0, len(targets))
	for _, t := range targets {
		if len(filtered) == 0 || !IsUnderTarget(t, filtered) {
			filtered = append(filtered, t)
		}
	}
	return filtered, problems
}
