package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

type runner struct {
	ctx                           context.Context
	surface                       surface.Context
	policy                        policy
	engineConfig                  engine.Config
	state                         *runState
	lock                          *workspace.Lock
	runDir, wt, sourceFingerprint string
	started                       time.Time
	doctor                        doctorReport
	discovery                     workspace.Discovery
	baseline                      workspace.Baseline
	commands                      []workspace.Command
	skippedCommands               []workspace.Command
	emptyKinds                    []string
	forcedQuota                   bool
	handoffWritten                bool
	gateFacts                     workspace.GateFacts
}

func (r *runner) Info(message string)  { fmt.Fprintln(r.surface.Stderr, message) }
func (r *runner) Warn(message string)  { fmt.Fprintln(r.surface.Stderr, "warning:", message) }
func (r *runner) Retry(message string) { fmt.Fprintln(r.surface.Stderr, "retry:", message) }

func (r *runner) phase(next string) error {
	r.state.State = next
	return r.save()
}

func (r *runner) save() error { return writeJSONAtomic(filepath.Join(r.runDir, "state.json"), r.state) }

func Run(c surface.Context) (surface.RunResult, error) {
	p, err := readPolicy(c.Config)
	if err != nil {
		return surface.RunResult{}, err
	}
	r := &runner{ctx: context.Background(), surface: c, policy: p,
		engineConfig: engineConfig(c.Config), started: time.Now()}
	if err := r.init(); err != nil {
		if r.lock != nil {
			_ = r.lock.Release()
		}
		return surface.RunResult{}, err
	}
	defer func() {
		if r.lock != nil {
			_ = r.lock.Release()
		}
		if r.state != nil && r.state.Terminal != nil && (r.state.Terminal.Status == "DONE" || r.state.Terminal.Status == "NO_CHANGES") &&
			!boolValue(obj(c.Config["workspace"])["keep_worktree"], true) {
			if err := workspace.WorktreeRemove(c.Repo, r.wt); err != nil {
				r.Warn("worktree retained: " + err.Error())
			}
		}
	}()
	if err := r.baselinePhase(); err != nil {
		r.stopFromError(err)
	} else if err := r.cycles(); err != nil {
		r.stopFromError(err)
	} else {
		status := "NO_CHANGES"
		if r.state.Counters.Commits > 0 {
			status = "DONE"
		}
		r.state.finish(status, noCandidatesReason(r.emptyKinds))
	}
	if err := r.save(); err != nil {
		return surface.RunResult{}, err
	}
	return r.result()
}

func (r *runner) init() error {
	id, err := workspace.RunID(time.Now())
	if err != nil {
		return err
	}
	r.runDir, err = workspace.RunDirFor(r.surface.Repo, id)
	if err != nil {
		return err
	}
	r.lock = workspace.NewLock(filepath.Join(r.surface.Repo, ".refactor", "lock.json"))
	holder, err := r.lock.Acquire()
	if err != nil {
		return err
	}
	if holder != nil {
		return fmt.Errorf("another run holds the lock (pid %d)", holder.PID)
	}
	r.doctor, err = runDoctor(r.surface, r.runDir)
	if err != nil {
		return err
	}
	r.Info(renderDoctor(r.doctor))
	r.engineConfig.Skills = r.doctor.Skills
	if !r.doctor.OK {
		return errors.New("doctor found a blocking problem")
	}
	base, err := workspace.HeadOID(r.surface.Repo)
	if err != nil {
		return err
	}
	branch, _ := workspace.CurrentBranch(r.surface.Repo)
	r.sourceFingerprint, err = workspace.SourceFingerprint(r.surface.Repo)
	if err != nil {
		return err
	}
	resultBranch := surface.ConfigString(r.surface.Config, "workspace", "branch_prefix") + id
	r.wt = workspace.WorktreePath(r.surface.Repo, id, surface.ConfigString(r.surface.Config, "workspace", "worktree_parent"))
	r.state = newState(id, r.surface.Repo, r.wt, base, branch, resultBranch, r.surface.Targets, r.surface.Providers)
	r.state.ToolVersion = surface.Version
	for _, usage := range r.doctor.Usage {
		r.noteUsage(usage.Provider, "doctor", engine.Result{OK: usage.OK, Usage: usage.Usage, DurationMS: usage.DurationMS, Processes: usage.Processes})
	}
	for name, status := range r.state.Providers {
		if !slices.Contains(r.doctor.Healthy, name) {
			status.Status = "DISABLED"
		}
	}
	if err := r.save(); err != nil {
		return err
	}
	if err := workspace.WorktreeAdd(r.surface.Repo, r.wt, base); err != nil {
		return err
	}
	r.Info("worktree " + r.wt)
	r.hydrate()
	return nil
}

// A clonefile copy makes ignored build inputs available in the detached checkout.
// Failure to hydrate a cache is reported; it does not modify the source checkout.
func (r *runner) hydrate() {
	wanted := map[string]bool{"node_modules": true, ".env.local": true, ".env": true, "vendor": true}
	var walk func(string, int)
	walk = func(rel string, depth int) {
		if depth > 3 {
			return
		}
		entries, err := os.ReadDir(filepath.Join(r.surface.Repo, rel))
		if err != nil {
			return
		}
		for _, entry := range entries {
			child := filepath.Join(rel, entry.Name())
			if wanted[entry.Name()] {
				src, dst := filepath.Join(r.surface.Repo, child), filepath.Join(r.wt, child)
				if _, err := os.Stat(dst); err == nil {
					continue
				}
				_ = os.MkdirAll(filepath.Dir(dst), 0o700)
				cmd := exec.Command("cp", "-c", "-R", src, dst)
				if err := cmd.Run(); err != nil {
					if err := exec.Command("cp", "-R", src, dst).Run(); err != nil {
						r.Warn("could not hydrate " + child)
					}
				}
				continue
			}
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && entry.Name() != "node_modules" {
				walk(child, depth+1)
			}
		}
	}
	walk("", 0)
}

func (r *runner) baselinePhase() error {
	if err := r.phase("BASELINE"); err != nil {
		return err
	}
	disc, err := workspace.DiscoverCommands(r.wt, filepath.Join(r.surface.Repo, ".refactor", "commands.json"))
	if err != nil {
		return err
	}
	r.discovery = disc
	r.commands, r.skippedCommands = disc.Commands, disc.Skipped
	if len(r.commands) == 0 {
		return halt{"ABORTED", "no deterministic validation command could be discovered"}
	}
	r.baseline = workspace.RunBaseline(r.ctx, r.commands, r.wt)
	noEvidence := map[string]bool{}
	for _, item := range r.baseline.Unrunnable {
		noEvidence[item.ID] = true
	}
	for _, item := range r.baseline.Opaque {
		noEvidence[item.ID] = true
	}
	filtered := make([]workspace.Command, 0, len(r.commands))
	for _, command := range r.commands {
		if noEvidence[command.ID] {
			command.Tier = "SKIPPED"
			r.skippedCommands = append(r.skippedCommands, command)
		} else {
			filtered = append(filtered, command)
		}
	}
	r.commands = filtered
	if err := writeJSONAtomic(filepath.Join(r.runDir, "baseline.json"), map[string]any{
		"areas": disc.Areas, "commands": r.commands, "skipped": r.skippedCommands, "hints": disc.Hints, "results": r.baseline.Results,
	}); err != nil {
		return err
	}
	if !r.baseline.Usable {
		return halt{"ABORTED", "no usable GREEN validation command at baseline"}
	}
	r.Info(r.baseline.Describe + " → proceeding")
	return nil
}

func (r *runner) stopFromError(err error) {
	var h halt
	if errors.As(err, &h) {
		r.state.finish(h.Status, h.Reason)
	} else {
		r.state.finish("HALTED_UNSAFE", "internal error: "+err.Error())
	}
	r.Warn(r.state.Terminal.Status + ": " + r.state.Terminal.Reason)
}

func noCandidatesReason(kinds []string) string {
	if len(kinds) > 0 {
		allFiltered, allNone := true, true
		for _, kind := range kinds {
			allFiltered = allFiltered && kind == "ALL_FILTERED"
			allNone = allNone && kind == "NO_PROPOSALS"
		}
		if allFiltered {
			return "every proposed candidate was filtered by policy"
		}
		if allNone {
			return "the audit proposed no candidates"
		}
	}
	return "no eligible candidates remain"
}

func (r *runner) result() (surface.RunResult, error) {
	state := r.state
	status := state.Terminal.Status
	var branch any
	if state.PublishedOID != "" {
		branch = state.BranchName
	}
	usageTotals := engine.UsageSummary{}
	for _, u := range state.Usage.ByPhase {
		usageTotals.Processes += u.Processes
		usageTotals.MS += u.MS
		usageTotals.InputTokens += u.InputTokens
		usageTotals.OutputTokens += u.OutputTokens
		usageTotals.CostMissing += u.CostMissing
		usageTotals.FailedCalls += u.FailedCalls
		usageTotals.Calls += u.Calls
		if u.CostUSD != nil {
			v := *u.CostUSD
			if usageTotals.CostUSD != nil {
				v += *usageTotals.CostUSD
			}
			usageTotals.CostUSD = &v
		}
	}
	providerMinutes := int(float64(usageTotals.MS)/60000 + 0.5)
	report := map[string]any{
		"schemaVersion": surface.ReportSchemaVersion, "runId": state.RunID, "toolVersion": state.ToolVersion,
		"status": status, "reason": state.Terminal.Reason,
		"durationMinutes": int(time.Since(r.started).Minutes() + 0.5), "providerMinutes": providerMinutes,
		"repoRoot": state.RepoRoot, "targets": state.Targets, "baseCommit": state.BaseOID,
		"baseBranch": state.BaseBranch, "branch": branch, "worktree": state.Worktree,
		"skills": r.doctor.Skills, "providerVersions": r.doctor.ProviderVersions, "skillLiveProbe": r.doctor.LiveProbe,
		"providers": state.Providers, "providerOrder": state.ProviderOrder,
		"usage":    map[string]any{"totals": usageTotals, "byPhase": state.Usage.ByPhase},
		"counters": state.Counters, "commits": state.Commits, "skipped": state.Seen.Skipped,
		"codeComparison": r.collectComparison(),
		"validation":     map[string]any{"describe": r.baseline.Describe, "ran": r.commandDescriptions(), "notRun": r.skippedDescriptions()},
	}
	if err := writeJSONAtomic(filepath.Join(r.runDir, "report.json"), report); err != nil {
		return surface.RunResult{}, err
	}
	data, err := os.ReadFile(filepath.Join(r.runDir, "report.json"))
	if err != nil {
		return surface.RunResult{}, err
	}
	md, err := surface.RenderReport(data, r.surface.Args.Language)
	if err != nil {
		return surface.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(r.runDir, "report.md"), []byte(md), 0o600); err != nil {
		return surface.RunResult{}, err
	}
	summary := fmt.Sprintf("refactor-me %s · %s\n  cycles      %d\n  commits     %d\n  branch      %v\n  report      %s\n  worktree    %s", surface.Version, status, state.Counters.Cycles, state.Counters.Commits, branch, filepath.Join(r.runDir, "report.md"), r.wt)
	branchName := ""
	if state.PublishedOID != "" {
		branchName = state.BranchName
	}
	return surface.RunResult{Status: status, RunID: state.RunID, RunDir: r.runDir, Branch: branchName, Report: data, Summary: summary}, nil
}

func (r *runner) commandDescriptions() []string {
	out := make([]string, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, fmt.Sprintf("%s %s %s: %s", c.Area, c.Tier, c.Name, strings.Join(c.Argv, " ")))
	}
	return out
}
func (r *runner) commandFacts() []engine.CommandFact {
	out := make([]engine.CommandFact, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, engine.CommandFact{Area: c.Area, Tier: c.Tier, Name: c.Name, Argv: c.Argv, CWD: c.Cwd, Source: c.Source})
	}
	return out
}
func (r *runner) skippedFacts() []engine.CommandFact {
	out := make([]engine.CommandFact, 0, len(r.skippedCommands))
	for _, c := range r.skippedCommands {
		out = append(out, engine.CommandFact{Area: c.Area, Tier: c.Tier, Name: c.Name, Argv: c.Argv, CWD: c.Cwd, Source: c.Source, Reason: c.Reason})
	}
	return out
}
func (r *runner) skippedDescriptions() []string {
	out := make([]string, 0, len(r.skippedCommands))
	for _, c := range r.skippedCommands {
		out = append(out, fmt.Sprintf("%s %s: %s", c.Area, c.Name, c.Reason))
	}
	return out
}

func writeText(path, value string) error { return os.WriteFile(path, []byte(value), 0o600) }
