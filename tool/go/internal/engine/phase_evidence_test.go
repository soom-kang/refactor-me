package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCandidatePromptsCarryMeasuredEvidence(t *testing.T) {
	zero := 0
	evidence := &PhaseEvidence{BaseCommit: "base-oid", BaselineSummary: "GREEN 1 / RED-differential 0 of 1", BaselineUsable: true,
		BaselineChecks:   []CheckEvidence{{ID: "web:lint", Status: "GREEN", ExitCode: &zero}},
		Characterization: &CharacterizationEvidence{Commit: "test-oid", Files: []string{"test/behavior.test.ts"}, ValidationAccepted: true},
	}
	for _, provider := range []string{"codex", "claude"} {
		for _, category := range []string{"DEAD_CODE", "DEDUPLICATION"} {
			for _, targets := range [][]string{nil, {"src"}} {
				for _, phase := range []string{"deep_check", "characterization", "preflight", "execute"} {
					prompt, err := BuildPrompt(phase, PromptArgs{Provider: provider, Nonce: "fixture", Targets: targets,
						Candidate: map[string]any{"category": category}, Packet: map[string]any{"category": category}, PhaseEvidence: evidence})
					if err != nil {
						t.Fatal(err)
					}
					start, end := "<<<ORCHESTRATOR_EVIDENCE id=fixture>>>\n", "\n<<<END_ORCHESTRATOR_EVIDENCE id=fixture>>>"
					_, block, ok := strings.Cut(prompt, start)
					block, _, closed := strings.Cut(block, end)
					var decoded PhaseEvidence
					if !ok || !closed || json.Unmarshal([]byte(block), &decoded) != nil || decoded.BaseCommit != "base-oid" || !decoded.BaselineUsable || decoded.Characterization == nil || decoded.Characterization.Commit != "test-oid" {
						t.Fatalf("%s/%s/%s lost phase evidence: %s", provider, category, phase, block)
					}
					if phase == "execute" {
						flat := strings.Join(strings.Fields(prompt), " ")
						for _, rule := range []string{"including CRLF/LF, trailing blank lines and the final newline", "correct your own artifact once", "Do not blindly repeat the same patch", "scope expansion and behavior changes still require FAIL", "Do NOT modify tests", "Other existing files retain their collision checks", "A null record grants no exception"} {
							if !strings.Contains(flat, rule) {
								t.Fatalf("%s/%s recovery lost boundary %q", provider, category, rule)
							}
						}
					}
				}
			}
		}
	}
}

func TestMissingPhaseEvidenceDoesNotInventSuccess(t *testing.T) {
	prompt, err := BuildPrompt("execute", PromptArgs{Provider: "codex", Nonce: "fixture"})
	if err != nil || !strings.Contains(prompt, "<<<ORCHESTRATOR_EVIDENCE id=fixture>>>\nnull\n") || !strings.Contains(prompt, "Missing evidence remains unknown") {
		t.Fatalf("missing evidence was not explicit: %v\n%s", err, prompt)
	}
}
