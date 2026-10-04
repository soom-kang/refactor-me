package controller

import (
	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

func checkEvidence(results []workspace.CommandResult) []engine.CheckEvidence {
	checks := make([]engine.CheckEvidence, 0, len(results))
	for _, result := range results {
		checks = append(checks, engine.CheckEvidence{ID: result.ID, Status: result.Status, ExitCode: result.ExitCode})
	}
	return checks
}

func (r *runner) phaseEvidence() *engine.PhaseEvidence {
	return &engine.PhaseEvidence{
		BaseCommit: r.state.BaseOID, BaselineSummary: r.baseline.Describe,
		BaselineUsable: r.baseline.Usable, BaselineChecks: checkEvidence(r.baseline.Results),
		Characterization: r.characterization,
	}
}
