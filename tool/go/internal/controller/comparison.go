package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

var fullOID = regexp.MustCompile(`^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`)

// collectComparison reads only published Git objects. Reporting errors do not
// change the run result, and no external diff driver or textconv is executed.
func (r *runner) collectComparison() map[string]any {
	s := r.state
	base := map[string]any{"status": "NO_CHANGES", "baseCommit": s.BaseOID, "resultCommit": nil,
		"files": []any{}, "totals": map[string]int{"files": 0, "insertions": 0, "deletions": 0, "binary": 0},
		"patchFile": nil, "preview": "", "truncated": false, "error": nil}
	if s.PublishedOID == "" {
		return base
	}
	base["resultCommit"] = s.PublishedOID
	fail := func(err error) map[string]any {
		base["status"] = "UNAVAILABLE"
		base["error"] = err.Error()
		return base
	}
	for _, oid := range []string{s.BaseOID, s.PublishedOID} {
		if !fullOID.MatchString(oid) {
			return fail(fmt.Errorf("comparison requires full commit OIDs"))
		}
		if _, err := workspace.Git(s.RepoRoot, "rev-parse", "--verify", "--end-of-options", oid+"^{commit}"); err != nil {
			return fail(err)
		}
	}
	args := []string{"-c", "core.quotePath=true", "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames=50%", "--diff-algorithm=myers", "--no-relative", "--src-prefix=a/", "--dst-prefix=b/", "--submodule=short", "--ignore-submodules=none", s.BaseOID, s.PublishedOID}
	raw, err := workspace.Git(s.RepoRoot, append(append([]string{}, args...), "--raw", "-z", "--")...)
	if err != nil {
		return fail(err)
	}
	parts := strings.Split(raw, "\x00")
	files := []map[string]any{}
	byPath := map[string]map[string]any{}
	for i := 0; i < len(parts) && parts[i] != ""; {
		meta := strings.Fields(strings.TrimPrefix(parts[i], ":"))
		i++
		if len(meta) < 5 || i >= len(parts) {
			return fail(fmt.Errorf("invalid Git raw diff record"))
		}
		first := parts[i]
		i++
		path, oldPath := first, any(nil)
		status := meta[4]
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i >= len(parts) {
				return fail(fmt.Errorf("invalid Git rename record"))
			}
			oldPath, path = first, parts[i]
			i++
		}
		file := map[string]any{"path": path, "oldPath": oldPath, "status": status, "oldMode": meta[0], "newMode": meta[1], "insertions": 0, "deletions": 0, "binary": false}
		files = append(files, file)
		byPath[path] = file
	}
	stats, err := workspace.Git(s.RepoRoot, append(append([]string{}, args...), "--numstat", "-z", "--")...)
	if err != nil {
		return fail(err)
	}
	sp := strings.Split(stats, "\x00")
	totals := map[string]int{"files": len(files), "insertions": 0, "deletions": 0, "binary": 0}
	for i := 0; i < len(sp) && sp[i] != ""; {
		line := strings.SplitN(sp[i], "\t", 3)
		i++
		if len(line) != 3 {
			return fail(fmt.Errorf("invalid Git numstat record"))
		}
		path := line[2]
		if path == "" {
			if i+1 >= len(sp) {
				return fail(fmt.Errorf("invalid Git rename numstat record"))
			}
			i++
			path = sp[i]
			i++
		}
		file := byPath[path]
		if file == nil {
			return fail(fmt.Errorf("Git metadata and numstat paths differ"))
		}
		if line[0] == "-" || line[1] == "-" {
			file["binary"] = true
			file["insertions"] = nil
			file["deletions"] = nil
			totals["binary"]++
		} else {
			added, aerr := strconv.Atoi(line[0])
			deleted, derr := strconv.Atoi(line[1])
			if aerr != nil || derr != nil {
				return fail(fmt.Errorf("invalid Git line counts"))
			}
			file["insertions"] = added
			file["deletions"] = deleted
			totals["insertions"] += added
			totals["deletions"] += deleted
		}
	}
	patch, err := workspace.Git(s.RepoRoot, append(append([]string{}, args...), "--patch", "--unified=3", "--")...)
	if err != nil {
		return fail(err)
	}
	patchPath := filepath.Join(r.runDir, "changes.patch")
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		return fail(err)
	}
	preview := []byte(patch)
	if len(preview) > 32*1024 {
		preview = preview[:32*1024]
	}
	lines := 0
	end := 0
	for i, b := range preview {
		if b == '\n' {
			end = i + 1
			lines++
			if lines == 200 {
				break
			}
		}
	}
	if end == 0 && len(preview) == len(patch) {
		end = len(preview)
	}
	if end < len(preview) && lines < 200 && len(preview) == len(patch) {
		end = len(preview)
	}
	base["status"] = "AVAILABLE"
	if len(files) == 0 {
		base["status"] = "NO_CHANGES"
	}
	base["files"] = files
	base["totals"] = totals
	base["patchFile"] = "changes.patch"
	base["preview"] = string(preview[:end])
	base["truncated"] = end < len(patch)
	return base
}
