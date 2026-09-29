package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

func Clean(c surface.Context) (string, error) {
	runsDir := filepath.Join(c.Repo, ".refactor", "runs")
	entries, err := os.ReadDir(runsDir)
	if os.IsNotExist(err) {
		return "nothing to clean\n", nil
	}
	if err != nil {
		return "", err
	}
	var lines []string
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		state, err := readJSONObject(filepath.Join(runsDir, id, "state.json"))
		if err != nil {
			continue
		}
		if state["schema"] != float64(workspace.StateSchema) {
			return "", fmt.Errorf("run %s has unsupported state schema %v; expected %d", id, state["schema"], workspace.StateSchema)
		}
		wt := str(state["worktree"])
		if wt == "" {
			continue
		}
		terminal := obj(state["terminal"])
		status := str(terminal["status"])
		if status != "DONE" && status != "NO_CHANGES" {
			lines = append(lines, fmt.Sprintf("keep  %s  (%s — worktree preserved)", id, status))
			continue
		}
		if _, err := os.Stat(wt); os.IsNotExist(err) {
			continue
		}
		if err := workspace.WorktreeRemoveRecorded(c.Repo, wt, id); err != nil {
			lines = append(lines, fmt.Sprintf("keep  %s  (%s)", id, err))
			continue
		}
		removed++
		lines = append(lines, "removed  "+wt)
	}
	lines = append(lines, fmt.Sprintf("%d worktree(s) removed", removed))
	return strings.Join(lines, "\n") + "\n", nil
}
