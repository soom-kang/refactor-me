package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

func (r *runner) runCandidate(candidate map[string]any) error {
	r.characterization = nil
	r.candidateLabel = candidateDescription(candidate, r.surface.Args.Language)
	defer r.endProgress()
	fp := str(candidate["fp"])
	cycleDir, err := workspace.CycleDirFor(r.runDir, r.state.Cycle, str(candidate["category"]), fp)
	if err != nil {
		return err
	}
	r.state.Seen.Attempts[fp]++
	packet, ok, err := r.selectPacket(candidate, cycleDir)
	if err != nil || !ok {
		return err
	}
	r.state.ActivePacket = packet
	if err := r.save(); err != nil {
		return err
	}
	preOID, err := workspace.HeadOID(r.wt)
	if err != nil {
		return err
	}
	if packet["characterization_needed"] == true && r.policy.AutoCharacterization {
		ok, err = r.characterize(packet, cycleDir, preOID)
		if err != nil || !ok {
			return err
		}
	}
	ok, err = r.preflight(packet, cycleDir)
	if err != nil || !ok {
		return err
	}
	writeOID, err := workspace.HeadOID(r.wt)
	if err != nil {
		return err
	}
	execResult, ok, err := r.execute(packet, cycleDir, writeOID)
	if err != nil || !ok {
		return err
	}
	ok, err = r.gate(packet, cycleDir, writeOID)
	if err != nil || !ok {
		return err
	}
	ok, err = r.review(packet, execResult, cycleDir)
	if err != nil || !ok {
		return err
	}
	return r.commit(packet, candidate, cycleDir)
}

func (r *runner) selectPacket(candidate map[string]any, cycleDir string) (map[string]any, bool, error) {
	if err := r.phase("SELECT"); err != nil {
		return nil, false, err
	}
	if err := r.phase("DEEP_CHECK"); err != nil {
		return nil, false, err
	}
	facts, err := engine.CollectRepoFacts(r.wt, engine.FactsOptions{Areas: r.discovery.Areas, Commands: r.commandFacts(), Skipped: r.skippedFacts(), Targets: r.surface.Targets, MaxFiles: 200})
	if err != nil {
		return nil, false, err
	}
	args := engine.PromptArgs{Candidate: candidate, RepoFacts: facts, Targets: r.surface.Targets,
		AreaSkills: engine.CollectAreaSkills(r.wt, stringsOf(candidate["related_files"])), Commands: strings.Join(r.commandDescriptions(), "\n"), PhaseEvidence: r.phaseEvidence()}
	call, err := r.callPhase("deep_check", cycleDir, "", args)
	if err != nil {
		return nil, false, err
	}
	if !call.OK {
		if call.Reason == "RESOURCE" {
			return nil, false, nil
		}
		return nil, false, r.fail(str(candidate["fp"]), "DEEP_CHECK_FAILED", "no provider produced a task packet", nil, false)
	}
	packet := obj(call.Result.Data)
	packet["fp"] = candidate["fp"]
	allowlist := stringsOf(packet["allowlist"])
	if len(allowlist) > 0 {
		if lines := workspace.FileLineCount(r.wt, allowlist[0]); lines != nil {
			packet["original_lines"] = *lines
		}
	}
	if err := writeJSONAtomic(filepath.Join(cycleDir, "packet.json"), packet); err != nil {
		return nil, false, err
	}
	if packet["readiness"] != "READY" {
		r.state.markSkipped(str(candidate["fp"]), "NOT_READY", str(packet["readiness_reason"]), stringsOf(candidate["related_files"]), false)
		return nil, false, r.saveSkipped("NOT_READY")
	}
	if packet["risk_level"] == "UNKNOWN" || !slices.Contains(r.policy.AllowedRisks, str(packet["risk_level"])) {
		reason := "RISK_EXCLUDED"
		if packet["risk_level"] == "UNKNOWN" {
			reason = "RISK_UNKNOWN"
		}
		r.state.markSkipped(str(candidate["fp"]), reason, "task packet risk is outside policy", allowlist, false)
		return nil, false, r.saveSkipped(reason)
	}
	if len(allowlist) == 0 {
		return nil, false, r.fail(str(candidate["fp"]), "EMPTY_ALLOWLIST", "READY packet with no allowlist", nil, false)
	}
	return packet, true, nil
}

func (r *runner) characterize(packet map[string]any, dir, preOID string) (bool, error) {
	if err := r.phase("CHARACTERIZE"); err != nil {
		return false, err
	}
	files := stringsOf(packet["characterization_files"])
	if len(files) == 0 {
		return false, r.fail(str(packet["fp"]), "NO_CHARACTERIZATION_FILES", "characterization required but no test files named", nil, false)
	}
	var absent []string
	for _, path := range files {
		if _, err := os.Lstat(filepath.Join(r.wt, path)); os.IsNotExist(err) {
			absent = append(absent, path)
		} else if err != nil {
			return false, err
		}
	}
	call, err := r.callPhase("characterization", dir, "", engine.PromptArgs{Packet: packet, AreaSkills: engine.CollectAreaSkills(r.wt, stringsOf(packet["allowlist"])), PhaseEvidence: r.phaseEvidence()})
	if err != nil {
		return false, err
	}
	if !call.OK {
		if err := r.rollback(preOID); err != nil {
			return false, err
		}
		if call.Reason == "RESOURCE" {
			return false, nil
		}
		return false, r.fail(str(packet["fp"]), "CHARACTERIZATION_FAILED", "no provider completed the test slice", nil, false)
	}
	data := obj(call.Result.Data)
	verdict, reasons := engine.EnforceCharacterizationVerdict(data)
	if err := r.sweep(); err != nil {
		return false, err
	}
	facts, err := workspace.CollectFacts(r.wt, r.surface.Repo, preOID, r.state.BaseOID, r.sourceFingerprint, workspace.Packet{Allowlist: files})
	if err != nil {
		return false, err
	}
	cfgPolicy := obj(r.surface.Config["policy"])
	policy := workspace.Policy{MaxFilesPerCandidate: r.policy.MaxFilesPerCandidate,
		MaxChangedLines: integer(cfgPolicy["max_changed_lines"], 600), ExtraForbiddenGlobs: stringsOf(cfgPolicy["extra_forbidden_globs"])}
	result := workspace.RunCharacterizationChecks(facts, files, policy, r.state.TreeHashes)
	if err := writeJSONAtomic(filepath.Join(dir, "characterization-gate.json"), result); err != nil {
		return false, err
	}
	patch, err := workspace.Git(r.wt, "diff", "--binary", "HEAD")
	if err != nil {
		return false, err
	}
	if err := writeText(filepath.Join(dir, "characterization.patch"), patch); err != nil {
		return false, err
	}
	if result.Verdict == "HALT" {
		return false, halt{"HALTED_UNSAFE", result.Violation.Code + ": " + result.Violation.Detail}
	}
	if verdict != "PASS" || result.Verdict == "VIOLATION" {
		if err := r.rollback(preOID); err != nil {
			return false, err
		}
		why := strings.Join(reasons, "; ")
		var paths []string
		if result.Violation != nil {
			why = result.Violation.Code + ": " + result.Violation.Detail
			paths = result.Violation.Paths
			r.state.Counters.Violations++
		}
		return false, r.fail(str(packet["fp"]), "CHARACTERIZATION_REJECTED", why, paths, result.Violation != nil)
	}
	changed := facts.Changed
	if len(changed) == 0 {
		return true, nil
	}
	ladder := workspace.RunLadder(r.ctx, r.baseline, r.commands, r.wt, changed, r.discovery.Areas, r.observeCommand)
	if err := writeJSONAtomic(filepath.Join(dir, "characterization-validation.json"), ladder); err != nil {
		return false, err
	}
	if !ladder.OK {
		if err := r.rollback(preOID); err != nil {
			return false, err
		}
		return false, r.fail(str(packet["fp"]), "CHARACTERIZATION_RED", "characterization tests do not pass", nil, false)
	}
	if err := workspace.StageExact(r.wt, changed); err != nil {
		return false, err
	}
	tree, err := workspace.WriteTree(r.wt)
	if err != nil {
		return false, err
	}
	if tree != facts.ProspectiveTree {
		return false, halt{"HALTED_UNSAFE", "characterization changed between safety checks and commit"}
	}
	message := filepath.Join(dir, "characterization-message.txt")
	if err := writeText(message, fmt.Sprintf("test(%s): characterize current behavior before refactor\n\nRefactor-Fingerprint: %s-char\n", str(packet["candidate_id"]), str(packet["fp"]))); err != nil {
		return false, err
	}
	oid, err := workspace.CommitApproved(r.wt, message, tree)
	if err != nil {
		return false, halt{"HALTED_UNSAFE", "characterization commit was not identical to the validated tree: " + err.Error()}
	}
	if err := r.publish(oid); err != nil {
		return false, err
	}
	r.state.TreeHashes = append(r.state.TreeHashes, tree)
	r.state.Commits = append(r.state.Commits, commitRecord{OID: oid, FP: str(packet["fp"]) + "-char", Category: "CHARACTERIZATION", Paths: changed, Subject: "characterize current behavior"})
	if err := r.save(); err != nil {
		return false, err
	}
	r.characterization = &engine.CharacterizationEvidence{
		Commit: oid, Files: changed, CreatedFiles: []string{}, ValidationAccepted: ladder.OK, Checks: checkEvidence(ladder.Checks),
	}
	for _, path := range changed {
		if slices.Contains(absent, path) {
			r.characterization.CreatedFiles = append(r.characterization.CreatedFiles, path)
		}
	}
	r.endProgress()
	r.notice("Saved characterization tests on the local result branch.", "현재 동작을 확인하는 테스트를 로컬 결과 branch에 저장했습니다.")
	return true, nil
}

func (r *runner) preflight(packet map[string]any, dir string) (bool, error) {
	if err := r.phase("PREFLIGHT"); err != nil {
		return false, err
	}
	facts, err := engine.CollectRepoFacts(r.wt, engine.FactsOptions{Areas: r.discovery.Areas, Commands: r.commandFacts(), Skipped: r.skippedFacts(), Targets: r.surface.Targets, MaxFiles: 120})
	if err != nil {
		return false, err
	}
	call, err := r.callPhase("preflight", dir, "", engine.PromptArgs{Packet: packet, RepoFacts: facts, BaselineSummary: r.baseline.Describe, PhaseEvidence: r.phaseEvidence()})
	if err != nil {
		return false, err
	}
	if !call.OK {
		if call.Reason == "RESOURCE" {
			return false, nil
		}
		return false, r.fail(str(packet["fp"]), "PREFLIGHT_FAILED", "no provider completed preflight", nil, false)
	}
	data := obj(call.Result.Data)
	if err := writeJSONAtomic(filepath.Join(dir, "preflight.json"), data); err != nil {
		return false, err
	}
	verdict, reasons := engine.EnforcePreflightVerdict(data)
	if verdict != "READY_TO_EXECUTE" {
		r.state.markSkipped(str(packet["fp"]), "PREFLIGHT_BLOCKED", strings.Join(reasons, "; "), stringsOf(packet["allowlist"]), false)
		return false, r.saveSkipped("PREFLIGHT_BLOCKED")
	}
	return true, nil
}

func (r *runner) execute(packet map[string]any, dir, preOID string) (map[string]any, bool, error) {
	if err := r.phase("EXECUTE"); err != nil {
		return nil, false, err
	}
	call, err := r.callPhase("execute", dir, "", engine.PromptArgs{Packet: packet, AreaSkills: engine.CollectAreaSkills(r.wt, stringsOf(packet["allowlist"])), PhaseEvidence: r.phaseEvidence()})
	if err != nil {
		return nil, false, err
	}
	if !call.OK {
		if err := r.rollback(preOID); err != nil {
			return nil, false, err
		}
		if call.Reason == "RESOURCE" {
			return nil, false, nil
		}
		return nil, false, r.fail(str(packet["fp"]), "EXECUTE_FAILED", "no provider completed the write", nil, false)
	}
	data := obj(call.Result.Data)
	data["provider"] = call.Provider
	if err := writeJSONAtomic(filepath.Join(dir, "execution.json"), data); err != nil {
		return nil, false, err
	}
	verdict, reasons := engine.EnforceExecutionVerdict(data, packet)
	if verdict == "FAIL" {
		if err := r.rollback(preOID); err != nil {
			return nil, false, err
		}
		why := strings.Join(reasons, "; ")
		if why == "" {
			why = str(data["rationale"])
		}
		if why == "" {
			why = str(data["notes"])
		}
		if why == "" {
			why = "implementer reported FAIL"
		}
		return nil, false, r.fail(str(packet["fp"]), "EXECUTE_REJECTED", why, nil, false)
	}
	allow := stringsOf(packet["allowlist"])
	var outside []string
	for _, path := range stringsOf(data["deleted_files"]) {
		if !slices.Contains(allow, path) {
			outside = append(outside, path)
		}
	}
	if len(outside) > 0 {
		if err := r.rollback(preOID); err != nil {
			return nil, false, err
		}
		return nil, false, r.fail(str(packet["fp"]), "OUT_OF_SCOPE", "declared deletion outside allowlist", outside, true)
	}
	if err := removeDeclaredFiles(r.wt, stringsOf(data["deleted_files"])); err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func removeDeclaredFiles(wt string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	root, err := os.OpenRoot(wt)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, path := range paths {
		if !filepath.IsLocal(path) {
			return halt{"HALTED_UNSAFE", "invalid declared deletion path"}
		}
		info, err := root.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return halt{"HALTED_UNSAFE", "declared deletion is a directory"}
		}
		// Root also enforces containment during removal if an intermediate
		// symlink changes after Lstat. A final symlink is removed, not followed.
		if err := root.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) gate(packet map[string]any, dir, preOID string) (bool, error) {
	if err := r.phase("GATE"); err != nil {
		return false, err
	}
	if err := r.sweep(); err != nil {
		return false, err
	}
	allow := stringsOf(packet["allowlist"])
	p := workspace.Packet{Category: str(packet["category"]), Allowlist: allow, OriginalLines: integer(packet["original_lines"], 0)}
	facts, err := workspace.CollectFacts(r.wt, r.surface.Repo, preOID, r.state.BaseOID, r.sourceFingerprint, p)
	if err != nil {
		return false, err
	}
	if len(facts.Changed) > 0 {
		facts.ProspectiveTree, err = workspace.WriteTree(r.wt)
		if err != nil {
			return false, err
		}
	}
	cfgPolicy := obj(r.surface.Config["policy"])
	policy := workspace.Policy{MaxFilesPerCandidate: r.policy.MaxFilesPerCandidate,
		MaxChangedLines: integer(cfgPolicy["max_changed_lines"], 600), ExtraForbiddenGlobs: stringsOf(cfgPolicy["extra_forbidden_globs"])}
	result := workspace.RunChecks(facts, p, policy, r.state.TreeHashes, r.surface.Targets)
	if err := writeJSONAtomic(filepath.Join(dir, "gate.json"), result); err != nil {
		return false, err
	}
	switch result.Verdict {
	case "HALT":
		return false, halt{"HALTED_UNSAFE", result.Violation.Code + ": " + result.Violation.Detail}
	case "NO_OP":
		if err := r.rollback(preOID); err != nil {
			return false, err
		}
		r.state.markSkipped(str(packet["fp"]), "NO_OP", "implementer changed nothing", allow, false)
		return false, r.saveSkipped("NO_OP")
	case "VIOLATION":
		if err := r.rollback(preOID); err != nil {
			return false, err
		}
		r.state.Counters.Violations++
		return false, r.fail(str(packet["fp"]), result.Violation.Code, result.Violation.Detail, result.Violation.Paths, true)
	}
	ladder := workspace.RunLadder(r.ctx, r.baseline, r.commands, r.wt, facts.Changed, r.discovery.Areas, r.observeCommand)
	if err := writeJSONAtomic(filepath.Join(dir, "validation.json"), ladder); err != nil {
		return false, err
	}
	if !ladder.OK {
		r.savePatch(dir, "rejected.patch")
		if err := r.rollback(preOID); err != nil {
			return false, err
		}
		kind, why := "VALIDATION_FAIL", "validation failed"
		if ladder.Failed != nil {
			kind = ladder.Failed.FailureKind
			why = ladder.Failed.Why
		}
		return false, r.fail(str(packet["fp"]), kind, why, nil, false)
	}
	r.gateFacts = facts
	return true, nil
}

func (r *runner) review(packet, execution map[string]any, dir string) (bool, error) {
	if err := r.phase("REVIEW"); err != nil {
		return false, err
	}
	if err := r.sweep(); err != nil {
		return false, err
	}
	diff, err := workspace.DiffForReview(r.wt, "HEAD")
	if err != nil {
		return false, err
	}
	if err := writeText(filepath.Join(dir, "accepted.patch"), diff); err != nil {
		return false, err
	}
	writer := str(execution["provider"])
	reviewer := writer
	if r.policy.CrossProviderReview {
		for _, p := range r.readyProviders() {
			if p != writer {
				reviewer = p
				break
			}
		}
	}
	call, err := r.callPhase("review", dir, reviewer, engine.PromptArgs{Packet: packet, Diff: diff, Rationale: str(execution["rationale"])})
	if err != nil {
		return false, err
	}
	if !call.OK {
		if err := r.rollback(r.gateFacts.PreOID); err != nil {
			return false, err
		}
		if call.Reason == "RESOURCE" {
			return false, nil
		}
		return false, r.fail(str(packet["fp"]), "REVIEW_FAILED", "no provider completed the review", nil, false)
	}
	data := obj(call.Result.Data)
	if err := writeJSONAtomic(filepath.Join(dir, "review.json"), data); err != nil {
		return false, err
	}
	verdict, reasons := engine.EnforceReviewVerdict(data)
	if verdict != "PASS" {
		r.savePatch(dir, "rejected.patch")
		if err := r.rollback(r.gateFacts.PreOID); err != nil {
			return false, err
		}
		return false, r.fail(str(packet["fp"]), "REVIEW_REJECT", strings.Join(reasons, "; "), nil, false)
	}
	return true, nil
}

func (r *runner) sweep() error {
	dirs := []string{}
	for _, command := range r.commands {
		base := ""
		if command.Area != "." {
			base = command.Area + "/"
		}
		switch command.Family {
		case "gradle":
			dirs = append(dirs, base+"build", base+".gradle")
		case "maven":
			dirs = append(dirs, base+"target")
		}
	}
	swept, err := workspace.SweepBuildArtifacts(r.surface.Repo, r.wt, dirs)
	if err != nil {
		return err
	}
	if len(swept) > 0 {
		r.notice("Removed %d validation artifact(s); add them to .gitignore.", "검증 산출물 %d개를 정리했습니다. .gitignore에 추가하세요.", len(swept))
	}
	return nil
}

func (r *runner) commit(packet, candidate map[string]any, dir string) error {
	if err := r.phase("COMMIT"); err != nil {
		return err
	}
	if err := workspace.StageExact(r.wt, r.gateFacts.Changed); err != nil {
		return err
	}
	tree, err := workspace.WriteTree(r.wt)
	if err != nil {
		return err
	}
	if tree != r.gateFacts.ProspectiveTree {
		return halt{"HALTED_UNSAFE", "worktree changed between validation and commit"}
	}
	subject := str(packet["title"])
	if subject == "" {
		subject = str(candidate["candidate_id"])
	}
	message := fmt.Sprintf("refactor: %s\n\nRefactor-Fingerprint: %s\n", subject, str(packet["fp"]))
	messageFile := filepath.Join(dir, "message.txt")
	if err := writeText(messageFile, message); err != nil {
		return err
	}
	oid, err := workspace.CommitApproved(r.wt, messageFile, tree)
	if err != nil {
		return halt{"HALTED_UNSAFE", "commit was not identical to reviewed tree: " + err.Error()}
	}
	if err := r.publish(oid); err != nil {
		return err
	}
	r.state.TreeHashes = append(r.state.TreeHashes, tree)
	r.state.Counters.Commits++
	r.state.Counters.ConsecutiveFailures = 0
	r.state.Seen.Done = append(r.state.Seen.Done, str(packet["fp"]))
	r.state.Commits = append(r.state.Commits, commitRecord{OID: oid, FP: str(packet["fp"]), Category: str(packet["category"]), Paths: stringsOf(packet["allowlist"]), Subject: subject})
	r.state.ActivePacket = nil
	if err := r.save(); err != nil {
		return err
	}
	r.endProgress()
	r.notice("Saved %s on the local result branch. Commit: %s", "%s 항목을 로컬 결과 branch에 저장했습니다. 커밋: %s", r.candidateLabel, oid)
	return nil
}
