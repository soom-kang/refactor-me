package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

var requiredSkills = []string{
	"sharpen-clarify", "sharpen-review", "sharpen-challenge", "sharpen-assess",
	"sharpen-refine", "sharpen-cold-review", "sharpen-brief", "sharpen-dedupe",
}

type doctorCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Blocking bool   `json:"blocking"`
	Detail   string `json:"detail"`
	Fix      string `json:"fix,omitempty"`
}

type doctorReport struct {
	At       string        `json:"at"`
	Version  string        `json:"version"`
	OK       bool          `json:"ok"`
	Healthy  []string      `json:"healthy"`
	Excluded []string      `json:"excluded"`
	Checks   []doctorCheck `json:"checks"`
	Usage    []doctorUsage `json:"usage"`
}

type doctorUsage struct {
	Provider   string       `json:"provider"`
	Usage      engine.Usage `json:"usage"`
	DurationMS int64        `json:"durationMs"`
	Processes  int          `json:"processes"`
	OK         bool         `json:"ok"`
}

func check(id, status, detail string, blocking bool) doctorCheck {
	return doctorCheck{ID: id, Status: status, Detail: detail, Blocking: blocking}
}

func runDoctor(c surface.Context, runDir string) (doctorReport, error) {
	report := doctorReport{At: time.Now().UTC().Format(time.RFC3339Nano), Version: surface.Version,
		Healthy: []string{}, Excluded: []string{}, Checks: []doctorCheck{}}
	add := func(id, status, detail string, blocking bool) {
		report.Checks = append(report.Checks, check(id, status, detail, blocking))
	}
	if _, err := workspace.EnsureRefactorDir(c.Repo); err != nil {
		return report, err
	}
	add("version", "PASS", "refactor-me "+surface.Version, false)
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		add("platform", "PASS", "darwin/arm64 "+runtime.Version(), false)
	} else {
		add("platform", "WARN", runtime.GOOS+"/"+runtime.GOARCH+" is untested", false)
	}
	status, err := workspace.StatusPorcelain(c.Repo)
	if err != nil {
		return report, err
	}
	if strings.TrimSpace(status) == "" {
		add("source-clean", "PASS", "working tree and index are clean", true)
	} else {
		add("source-clean", "FAIL", "working tree and index are dirty", true)
	}
	if len(c.Targets) > 0 {
		tracked, err := workspace.Git(c.Repo, "ls-files")
		if err != nil {
			add("target", "WARN", "could not list tracked files", false)
		} else {
			n := 0
			for _, path := range strings.Split(tracked, "\n") {
				if slices.ContainsFunc(c.Targets, func(t string) bool { return path == t || strings.HasPrefix(path, t+"/") }) {
					n++
				}
			}
			if n == 0 {
				add("target", "FAIL", "no tracked file under target", true)
			} else {
				add("target", "PASS", fmt.Sprintf("%d tracked file(s) in scope", n), true)
			}
		}
	}
	probePath := filepath.Join(os.TempDir(), fmt.Sprintf("refactor-probe-%d-%d", os.Getpid(), time.Now().UnixNano()))
	base, err := workspace.HeadOID(c.Repo)
	if err != nil {
		return report, err
	}
	if err := workspace.WorktreeAdd(c.Repo, probePath, base); err != nil {
		add("git-worktree", "FAIL", err.Error(), true)
	} else {
		add("git-worktree", "PASS", "detached checkout opens", true)
		for _, provider := range c.Providers {
			missing := missingSkills(provider, probePath)
			if len(missing) > 0 {
				add(provider+"-skills", "FAIL", "base commit lacks: "+strings.Join(missing, ", "), true)
			} else {
				add(provider+"-skills", "PASS", "required skills resolve in base checkout", true)
			}
		}
		if err := workspace.WorktreeRemove(c.Repo, probePath); err != nil {
			add("git-worktree-cleanup", "WARN", err.Error(), false)
		}
	}
	config := engineConfig(c.Config)
	for _, provider := range c.Providers {
		agent := config.Agents[provider]
		bin := agent.Bin
		if bin == "" {
			bin = provider
		}
		where, err := exec.LookPath(bin)
		if err != nil {
			add(provider+"-cli", "WARN", bin+" is not on PATH", false)
			continue
		}
		add(provider+"-cli", "PASS", where, false)
		if c.Args.Live {
			probeFile := filepath.Join(c.Repo, ".refactor", fmt.Sprintf("probe-%d.txt", time.Now().UnixNano()))
			prompt := fmt.Sprintf("Capability probe. Return JSON with answer 7, visible_skills listing only skills available in this session from %s, and wrote_file indicating whether creating %s succeeded. Attempt the write once. Do not work around a refusal.", strings.Join(requiredSkills, ", "), probeFile)
			res, err := engine.CallProvider(context.Background(), provider, engine.Request{
				Phase: "doctor", Mode: "read", CWD: c.Repo, Body: prompt,
				Schema: map[string]any{"type": "object", "additionalProperties": false,
					"required":   []any{"answer", "visible_skills", "wrote_file"},
					"properties": map[string]any{"answer": map[string]any{"type": "integer"}, "visible_skills": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "wrote_file": map[string]any{"type": "boolean"}}},
				RunDir: runDir, Timeout: 180 * time.Second, Attempt: "primary", Effort: "low",
			}, config, nil)
			report.Usage = append(report.Usage, doctorUsage{Provider: provider, Usage: res.Usage, DurationMS: res.DurationMS, Processes: res.Processes, OK: res.OK})
			wroteFile := false
			if _, statErr := os.Lstat(probeFile); statErr == nil {
				wroteFile = true
				add(provider+"-read-only", "FAIL", "read session created probe file", false)
				_ = os.Remove(probeFile)
			}
			if wroteFile {
				report.Excluded = append(report.Excluded, provider)
				continue
			}
			if err != nil || !res.OK {
				add(provider+"-live", "FAIL", fmt.Sprint(err, " ", res.Failure, " ", res.Detail), false)
				report.Excluded = append(report.Excluded, provider)
				continue
			}
			data := obj(res.Data)
			if data["wrote_file"] == true {
				add(provider+"-read-only", "FAIL", "provider reported writing during read-only probe", false)
				report.Excluded = append(report.Excluded, provider)
				continue
			}
			if integer(data["answer"], 0) != 7 {
				add(provider+"-structured-output", "FAIL", "expected answer 7", false)
				report.Excluded = append(report.Excluded, provider)
				continue
			}
			if len(stringsOf(data["visible_skills"])) == 0 {
				add(provider+"-visible-skills", "FAIL", "provider did not report any visible skills", false)
				report.Excluded = append(report.Excluded, provider)
				continue
			}
			add(provider+"-visible-skills", "PASS", "provider reported visible skills", false)
			add(provider+"-structured-output", "PASS", "structured output received", true)
		}
		report.Healthy = append(report.Healthy, provider)
	}
	if len(report.Healthy) == 0 {
		add("providers", "FAIL", "no provider passed availability checks", true)
	}
	report.OK = true
	for _, row := range report.Checks {
		if row.Status == "FAIL" && row.Blocking {
			report.OK = false
		}
	}
	if runDir != "" {
		if err := writeJSONAtomic(filepath.Join(runDir, "doctor.json"), report); err != nil {
			return report, err
		}
	}
	return report, nil
}

func skillRoots(provider, root, home string) []string {
	if provider == "claude" {
		return []string{filepath.Join(root, ".claude", "skills")}
	}
	roots := []string{filepath.Join(root, ".agents", "skills")}
	if home != "" {
		roots = append(roots, filepath.Join(home, ".agents", "skills"))
	}
	return roots
}

func missingSkills(provider, root string) []string {
	home, _ := os.UserHomeDir()
	roots := skillRoots(provider, root, home)
	var missing []string
	for _, name := range requiredSkills {
		found := false
		for _, base := range roots {
			if _, err := os.Stat(filepath.Join(base, name, "SKILL.md")); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, name)
		}
	}
	return missing
}

func engineConfig(cfg surface.Config) engine.Config {
	out := engine.Config{Agents: map[string]engine.AgentConfig{}}
	agents := obj(cfg["agents"])
	for _, name := range []string{"codex", "claude"} {
		data, _ := json.Marshal(agents[name])
		var agent engine.AgentConfig
		_ = json.Unmarshal(data, &agent)
		out.Agents[name] = agent
	}
	return out
}

func renderDoctor(report doctorReport) string {
	state := "OK"
	if !report.OK {
		state = "BLOCKED"
	}
	var lines []string
	for _, row := range report.Checks {
		icon := "[+]"
		if row.Status == "WARN" {
			icon = "[!]"
		}
		if row.Status == "FAIL" {
			icon = "[x]"
		}
		lines = append(lines, fmt.Sprintf("  %s %-26s %s", icon, row.ID, row.Detail))
		if row.Fix != "" {
			lines = append(lines, "    → "+row.Fix)
		}
	}
	healthy := strings.Join(report.Healthy, ", ")
	if healthy == "" {
		healthy = "none"
	}
	if len(report.Excluded) > 0 {
		healthy += "; excluded: " + strings.Join(report.Excluded, ", ")
	}
	return fmt.Sprintf("doctor: %s  (healthy providers: %s)\n%s", state, healthy, strings.Join(lines, "\n"))
}

func Doctor(c surface.Context) (surface.DoctorResult, error) {
	id, err := workspace.RunID(time.Now())
	if err != nil {
		return surface.DoctorResult{}, err
	}
	runDir, err := workspace.RunDirFor(c.Repo, "doctor-"+id)
	if err != nil {
		return surface.DoctorResult{}, err
	}
	report, err := runDoctor(c, runDir)
	if err != nil {
		return surface.DoctorResult{}, err
	}
	data, _ := json.MarshalIndent(report, "", "  ")
	return surface.DoctorResult{OK: report.OK, Summary: renderDoctor(report), JSON: append(data, '\n')}, nil
}
