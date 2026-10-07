package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

func (r *runner) cycles() error {
	for {
		if err := r.budgetCheck(); err != nil {
			return err
		}
		r.state.Cycle++
		r.state.Counters.Cycles++
		if err := r.save(); err != nil {
			return err
		}
		proposed, eligible, err := r.audit()
		if err != nil {
			return err
		}
		// An audit may finish after the time limit. Do not start a new candidate,
		// but avoid rechecking the cycle budget after this cycle was incremented.
		if err := r.wallClockCheck(); err != nil {
			return err
		}
		if len(eligible) == 0 {
			kind := "ALL_FILTERED"
			if proposed == 0 {
				kind = "NO_PROPOSALS"
			}
			r.emptyKinds = append(r.emptyKinds, kind)
			r.state.Counters.EmptyAudits++
			if kind == "NO_PROPOSALS" {
				r.state.Counters.AuditsNoProposals++
			} else {
				r.state.Counters.AuditsAllFiltered++
			}
			if err := r.save(); err != nil {
				return err
			}
			if r.state.Counters.EmptyAudits >= r.policy.EmptyAuditsToStop {
				return nil
			}
			continue
		}
		r.state.Counters.EmptyAudits = 0
		r.emptyKinds = nil
		if err := r.runCandidate(eligible[0]); err != nil {
			return err
		}
	}
}

func (r *runner) budgetCheck() error {
	if err := r.contextErr(); err != nil {
		return err
	}
	c := r.state.Counters
	p := r.policy
	switch {
	case c.Cycles >= p.MaxCycles:
		return halt{"DONE_PARTIAL", fmt.Sprintf("stopped on cycle budget (%d)", p.MaxCycles)}
	case c.Commits >= p.MaxCommits:
		return halt{"DONE_PARTIAL", fmt.Sprintf("stopped on commit budget (%d)", p.MaxCommits)}
	case c.ConsecutiveFailures >= p.MaxConsecutiveFailures:
		return halt{"DONE_PARTIAL", fmt.Sprintf("stopped on %d consecutive failures", p.MaxConsecutiveFailures)}
	}
	if err := r.wallClockCheck(); err != nil {
		return err
	}
	if len(r.readyProviders()) == 0 {
		return halt{"DONE_PARTIAL", "stopped on all providers exhausted"}
	}
	return nil
}

func (r *runner) wallClockCheck() error {
	if time.Since(r.started) >= time.Duration(r.policy.MaxWallClockMin)*time.Minute {
		return halt{"DONE_PARTIAL", fmt.Sprintf("stopped on wall clock (%dm)", r.policy.MaxWallClockMin)}
	}
	return nil
}

func (r *runner) readyProviders() []string {
	var out []string
	for _, name := range r.state.ProviderOrder {
		p := r.state.Providers[name]
		if p == nil || p.Status == "DISABLED" || p.Status == "DEAD" {
			continue
		}
		if p.Status == "COOLDOWN" {
			until, _ := time.Parse(time.RFC3339Nano, p.CooldownUntil)
			if time.Now().Before(until) {
				continue
			}
			p.Status = "READY"
			p.CooldownUntil = ""
		}
		out = append(out, name)
	}
	return out
}

type phaseResult struct {
	Result   engine.Result
	Provider string
	Reason   string
	OK       bool
}

func (r *runner) noteUsage(provider, phase string, result engine.Result) {
	one := engine.UsageSummaryForResult(result)
	status := r.state.Providers[provider]
	status.Usage = engine.MergeUsageSummaries(status.Usage, one)
	if r.state.Usage.ByPhase == nil {
		r.state.Usage.ByPhase = map[string]engine.UsageSummary{}
	}
	r.state.Usage.ByPhase[phase] = engine.MergeUsageSummaries(r.state.Usage.ByPhase[phase], one)
}

func (r *runner) callPhase(phase, runDir, prefer string, args engine.PromptArgs) (phaseResult, error) {
	resourceOnly := true
	available := r.readyProviders()
	if prefer != "" && slices.Contains(available, prefer) {
		available = append([]string{prefer}, slices.DeleteFunc(available, func(x string) bool { return x == prefer })...)
	}
	for providerIndex, name := range available {
		if providerIndex > 0 {
			r.startPhase(r.state.State)
		}
		if r.state.ActiveProvider != "" && r.state.ActiveProvider != name {
			r.notice("Switching provider from %s to %s.", "provider를 %s에서 %s로 전환합니다.", surface.DisplayText(r.state.ActiveProvider), surface.DisplayText(name))
		}
		for attempt := 0; attempt < 3; attempt++ {
			if err := r.contextErr(); err != nil {
				return phaseResult{}, err
			}
			status := r.state.Providers[name]
			status.Calls++
			r.state.ActiveProvider = name
			if err := r.save(); err != nil {
				return phaseResult{}, err
			}
			r.notice("%s is working on this stage.", "이 단계는 %s가 진행합니다.", surface.DisplayText(name))
			var result engine.Result
			injected := r.surface.Args.ForceQuotaAt == phase && !r.forcedQuota
			if injected {
				r.forcedQuota = true
				result = engine.Result{Failure: "QUOTA", Hard: true, Detail: "injected for rehearsal", Provider: name}
			} else {
				args.Provider = name
				args.Nonce = engine.NewNonce()
				prompt, err := engine.BuildPrompt(phase, args)
				if err != nil {
					return phaseResult{}, err
				}
				schemaName := phase
				if phase == "deep_check" {
					schemaName = "deepcheck"
				}
				request := engine.Request{Phase: phase, Mode: engine.ModeFor(phase), CWD: r.wt,
					Body: prompt, Schema: engine.Schema(schemaName), RunDir: runDir, Attempt: "primary"}
				if phase == "audit" && r.state.Cycle > 1 {
					request.Effort = "medium"
				}
				result, err = engine.CallWithRepair(r.ctx, name, request, r.engineConfig, r)
				if err != nil {
					if errors.Is(err, workspace.ErrUnsafe) {
						if result.Processes > 0 {
							r.noteUsage(name, phase, result)
						}
						return phaseResult{}, err
					}
					result.OK = false
					result.Failure, result.Detail, result.Provider = "PROCESS", err.Error(), name
				}
			}
			if !injected {
				r.noteUsage(name, phase, result)
				if err := r.save(); err != nil {
					return phaseResult{}, err
				}
			}
			if err := r.contextErr(); err != nil {
				return phaseResult{}, err
			}
			if result.OK {
				return phaseResult{Result: result, Provider: name, OK: true}, nil
			}
			if result.Fatal {
				return phaseResult{}, halt{"ABORTED", name + ": " + result.Detail}
			}
			status.LastError = result.Failure + ": " + result.Detail
			if result.Failure == "QUOTA" || result.Failure == "AUTH" {
				if result.Failure == "QUOTA" {
					status.QuotaHits++
				}
				if result.Failure == "AUTH" {
					status.Status = "DEAD"
				} else if result.Hard {
					status.Status = "COOLDOWN"
					status.CooldownUntil = time.Now().Add(time.Duration(r.policy.CooldownMinutes) * time.Minute).UTC().Format(time.RFC3339Nano)
				}
				if err := r.save(); err != nil {
					return phaseResult{}, err
				}
				r.notice("%s is unavailable for this call. [%s]", "%s가 이번 호출을 진행할 수 없습니다. [%s]", surface.DisplayText(name), surface.DisplayText(result.Failure))
				if err := r.writeHandoff(name, result.Failure); errors.Is(err, workspace.ErrUnsafe) {
					return phaseResult{}, err
				}
				break
			}
			resourceOnly = false
			if result.Failure == "TIMEOUT" || result.Failure == "PROCESS" {
				if attempt < 2 {
					delay := 5 * time.Second
					if attempt == 1 {
						delay = 20 * time.Second
					}
					r.notice("%s call failed; retrying in %s (retry %d of 2). [%s]", "%s 호출에 실패했습니다. %s 후 재시도합니다. 재시도 %d/2회입니다. [%s]", surface.DisplayText(name), delay, attempt+1, surface.DisplayText(result.Failure))
					select {
					case <-time.After(delay):
					case <-r.ctx.Done():
						return phaseResult{}, r.ctx.Err()
					}
					continue
				}
			}
			break
		}
	}
	reason := "FAILED"
	if resourceOnly {
		reason = "RESOURCE"
	}
	return phaseResult{Reason: reason}, nil
}

func (r *runner) writeHandoff(dead, failure string) error {
	if r.handoffWritten {
		return nil
	}
	live := ""
	for _, name := range r.readyProviders() {
		if name != dead {
			live = name
			break
		}
	}
	if live == "" {
		return nil
	}
	r.handoffWritten = true
	data, _ := json.MarshalIndent(r.state, "", "  ")
	end := r.state.PublishedOID
	if end == "" {
		end = r.state.BaseOID
	}
	log, _ := workspace.LogOneline(r.surface.Repo, r.state.BaseOID+".."+end, 20)
	args := engine.PromptArgs{Provider: live, DeadProvider: dead, LiveProvider: live, FailureClass: failure, StateJSON: string(data), GitLog: log}
	prompt, err := engine.BuildPrompt("handoff", args)
	if err != nil {
		return err
	}
	r.begin("Preparing context for %s after %s became unavailable.", "%s에 전달할 작업 기록을 준비합니다. %s가 현재 호출을 진행할 수 없습니다.", surface.DisplayText(live), surface.DisplayText(dead))
	defer r.endProgress()
	res, err := engine.CallProvider(r.ctx, live, engine.Request{Phase: "handoff", Mode: "read", CWD: r.wt, Body: prompt, RunDir: r.runDir, Effort: "low", Attempt: "primary"}, r.engineConfig, nil)
	if res.Processes > 0 {
		r.noteUsage(live, "handoff", res)
	}
	if err != nil {
		return err
	}
	if err := r.save(); err != nil {
		return err
	}
	if res.OK && strings.TrimSpace(res.Text) != "" {
		header := fmt.Sprintf("<!-- generated mid-run at the %s handoff; not the final outcome -->\n> Snapshot at the %s → %s handoff. Read report.md for the final result.\n\n", failure, dead, live)
		if r.surface.Args.Language == "ko" {
			header = fmt.Sprintf("<!-- generated mid-run at the %s handoff; not the final outcome -->\n> %s → %s 전환 시점의 기록입니다. 최종 결과는 report.md에서 확인하세요.\n\n", failure, dead, live)
		}
		return writeText(filepath.Join(r.runDir, "handoff.md"), header+res.Text)
	}
	return nil
}

func (r *runner) audit() (int, []map[string]any, error) {
	if err := r.phase("AUDIT"); err != nil {
		return 0, nil, err
	}
	cycleDir, err := workspace.AuditDirFor(r.runDir, r.state.Cycle)
	if err != nil {
		return 0, nil, err
	}
	facts, err := engine.CollectRepoFacts(r.wt, engine.FactsOptions{Areas: r.discovery.Areas, Commands: r.commandFacts(), Skipped: r.skippedFacts(), Targets: r.surface.Targets, MaxFiles: 400})
	if err != nil {
		return 0, nil, err
	}
	args := engine.PromptArgs{Incremental: r.state.Cycle > 1, Committed: len(r.state.Commits), Cycle: r.state.Cycle,
		RepoFacts: facts, Targets: r.surface.Targets, SeenDone: r.state.Seen.Done, Violated: r.violatedPaths()}
	for _, x := range r.state.Seen.Skipped {
		args.SeenSkipped = append(args.SeenSkipped, engine.SkippedCandidate{Fingerprint: x.FP, Reason: x.Reason})
	}
	call, err := r.callPhase("audit", cycleDir, "", args)
	if err != nil {
		return 0, nil, err
	}
	if !call.OK {
		if call.Reason == "RESOURCE" {
			return 0, nil, halt{"DONE_PARTIAL", "stopped on all providers exhausted during audit"}
		}
		r.state.Counters.ConsecutiveFailures++
		return 0, nil, halt{"DONE_PARTIAL", "audit failed without a usable provider response"}
	}
	data := obj(call.Result.Data)
	all := mapsOf(data["candidates"])
	proposed := len(all)
	if len(all) > r.policy.MaxAuditCandidates {
		all = all[:r.policy.MaxAuditCandidates]
	}
	if err := writeJSONAtomic(filepath.Join(r.runDir, "audits", fmt.Sprintf("%02d.json", r.state.Cycle)), map[string]any{"provider": call.Provider, "candidates": all}); err != nil {
		return 0, nil, err
	}
	seen := map[string]bool{}
	for _, x := range r.state.Seen.Done {
		seen[x] = true
	}
	for _, x := range r.state.Seen.Skipped {
		seen[x.FP] = true
	}
	eligible, rejected := rank(all, r.surface.Targets, r.policy, seen, r.state.Seen.Attempts)
	for _, x := range rejected {
		if x.Reason == "ALREADY_SEEN" {
			continue
		}
		r.state.markSkipped(str(x.Candidate["fp"]), x.Reason, x.Detail, stringsOf(x.Candidate["related_files"]), false)
	}
	if err := r.save(); err != nil {
		return 0, nil, err
	}
	r.endProgress()
	r.notice("This audit found %d candidates; %d are eligible.", "이번 조사에서 후보 %d개를 찾았습니다. 이 중 %d개를 진행할 수 있습니다.", proposed, len(eligible))
	if len(all) < proposed {
		r.notice("Reviewed %d candidates within the configured limit.", "설정한 한도에 따라 후보 %d개를 검토했습니다.", len(all))
	}
	for _, x := range rejected {
		if x.Reason != "ALREADY_SEEN" {
			r.notice("Excluded %s: %s. [%s]", "%s 항목을 제외했습니다. %s [%s]", candidateDescription(x.Candidate, r.surface.Args.Language), surface.ReasonLabel(x.Reason, r.surface.Args.Language), surface.DisplayText(x.Reason))
		}
	}
	return len(all), eligible, nil
}

func (r *runner) violatedPaths() []string {
	set := map[string]bool{}
	for _, x := range r.state.Seen.Skipped {
		if x.Detail["violated"] == true {
			for _, p := range stringsOf(x.Detail["paths"]) {
				set[p] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

func (r *runner) fail(fp, reason, detail string, paths []string, violated bool) error {
	r.state.markSkipped(fp, reason, detail, paths, violated)
	r.state.Counters.ConsecutiveFailures++
	r.state.ActivePacket = nil
	return r.saveSkipped(reason)
}

func (r *runner) rollback(preOID string) error {
	result, err := workspace.Rollback(r.surface.Repo, r.wt, preOID)
	if err != nil {
		return halt{"HALTED_UNSAFE", "rollback failed: " + err.Error()}
	}
	if !result.Clean {
		return halt{"HALTED_UNSAFE", "rollback left worktree dirty: " + result.Residue}
	}
	r.endProgress()
	r.notice("Rolled back this candidate's unaccepted changes.", "이번 항목의 반영되지 않은 변경을 되돌렸습니다.")
	return nil
}

func (r *runner) publish(oid string) error {
	if r.engineConfig.Skills != nil {
		if err := r.engineConfig.Skills.Verify(); err != nil {
			return fmt.Errorf("%w: %v", workspace.ErrUnsafe, err)
		}
		if err := r.engineConfig.Skills.CheckWorkspace(r.wt); err != nil {
			return fmt.Errorf("%w: %v", workspace.ErrUnsafe, err)
		}
	}
	ref := "refs/heads/" + r.state.BranchName
	if err := workspace.PublishCAS(r.surface.Repo, ref, oid, r.state.PublishedOID); err != nil {
		return halt{"HALTED_UNSAFE", "could not publish " + ref + ": " + err.Error()}
	}
	r.state.PublishedOID = oid
	return r.save()
}

func (r *runner) savePatch(dir, name string) {
	patch, err := workspace.Git(r.wt, "diff", "--binary", "HEAD")
	if err == nil {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(patch), 0o600)
	}
}
