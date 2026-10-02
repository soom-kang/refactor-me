# How a run works

[Project](../README.md) · [한국어](WORKFLOW.ko.md) · [Usage](TUTORIAL.md) · [Reference](README.md)

You set the limits and start `run`. The controller then checks each phase without asking for approval between steps. Changes happen in an isolated Git worktree; accepted commits appear on a local result branch.

In public Beta `0.10.0-beta.2`, select a model for every primary/fallback provider before live calls. Command model options override project configuration and remain local to the command. A command effort option applies across every phase for its selected provider; otherwise the configured phase policy remains. See [model and effort selection](README.md#model-and-effort-selection).

## Global installation, separate project state

![Homebrew and global Skills are shared; configuration and reports belong to the selected repository.](../docs/assets/workflow/installation.en.png)

`--repo` selects the repository independently of the executable's location. Each repository has its own `.refactor` configuration, lock and run records. Worktrees default to `~/.cache/refactor-me`.

Codex receives the absolute paths of the selected global Skills. Claude receives a per-run `.claude/skills` copy through `--add-dir`, with project-only settings and restricted tools/MCP. Source and snapshot hashes are checked around provider calls; unexpected changes stop result publication. See [Skill resolution](README.md#skill-availability).

## From checks to a local branch

![Run preparation leads to an isolated refactoring cycle, a saved outcome and manual review of published changes.](../docs/assets/workflow/execution.en.png)

| Stage | Required check | Saved evidence |
| --- | --- | --- |
| Prepare | Clean source, valid global Skills, selected models and available provider; at least one passing baseline command | Doctor and baseline results |
| Choose | Audit scope, risk and candidate history; deep check produces a ready task packet | `audits/`, `packet.json` |
| Refine | Optional characterization tests; preflight rejects unresolved failure hypotheses; execution stays within the packet | `preflight.json`, `execution.json` |
| Validate and review | Diff gate, no validation regression, separate review session | `gate.json`, `validation.json`, `review.json` |
| Publish and repeat | Commit tree matches the reviewed tree, source unchanged and result ref matches the expected OID | `state.json`, final reports |

Characterization records existing behavior in tests before production edits. When enabled, these tests must pass against unchanged production code and may form a separate commit. A later rejected refactor can leave that test commit on the result branch. `max_commits` counts refactor commits only.

Preflight must report `READY_TO_EXECUTE`, a `FALSIFIED` failure hypothesis and no blocking reasons. Execution cannot expand the packet's scope. The controller checks paths, file and line limits, test weakening, binaries and repeated trees against the actual diff.

Review uses a separate session, preferring the other available provider when configured. `FAIL`, `BLOCKER`, or an assessment other than `PRESERVED` rejects the change. A `HIGH` finding alone does not override `PASS`; read the findings before merging.

Commit hooks run. Hook changes to the reviewed tree or residual working changes stop publication with `HALTED_UNSAFE`. Updating the result branch checks its previous OID, so a conflicting update also stops publication.

## Validation scope

Baseline checks cover discovered areas, or the locked list in `.refactor/commands.json`. At least one command must pass before audit starts.

After an edit, checks select the deepest affected area for each path and the root area when present. Passing commands must keep passing. Readable baseline failures must gain no new error signatures; an unchanged failure still counts as a failure. Baseline `TIMEOUT`, `UNRUNNABLE` and `OPAQUE` commands are skipped.

`--target` limits discovery. Reachability checks cover the repository, and caller edits may extend beyond the target. Omitted browser, service and integration tests remain your responsibility before merging.

## Rejection, retries and stopping

| Event | Controller action |
| --- | --- |
| Unknown risk | Set aside by default. `unknown_risk: deep_check` admits only `UNKNOWN` + `NEEDS_EVIDENCE`; the resulting packet must still pass the risk policy |
| Gate, validation or review rejection | Restore candidate edits in the owned worktree; preserve any earlier accepted commits |
| Invalid JSON or response schema | Try one read-only repair; if it still fails with `SCHEMA`, try another available provider |
| Timeout or process failure | Retry after 5 and 20 seconds, then try another available provider; timeouts terminate the process group with SIGTERM then SIGKILL after a grace period |
| Quota or authentication failure | Mark provider availability and attempt a handoff; continue the phase with another available provider |

Fatal provider errors abort. Exhausted providers, cycle/commit/time limits, repeated failures or enough empty audits stop the loop. Safety violations preserve the worktree for investigation.

A provider switch starts a new session with phase inputs; it does not resume the old conversation. Write-phase retries do not each restore the worktree: candidate rejection or failure handles rollback. `handoff.md` is an intermediate record, not the next session's context or the final outcome.

## Read the final evidence

All paths below are relative to `.refactor/runs/<id>/`.

| File | Use |
| --- | --- |
| `report.md`, `report.json` | Outcome, stop reason, validation and usage |
| `changes.patch` | Net published text changes from the recorded base to final published commit |
| `state.json` | Counters, terminal status, worktree and published OID |
| `audits/<cycle>/`, `cycles/*/` | Prompts, responses, task packets and phase checks |
| `cycles/*/accepted.patch` | Diff sent to review; it may belong to a rejected candidate |

A missing comparison is recorded as `UNAVAILABLE` and does not change the run's exit code. Viewing a report does not regenerate a diff. Exit `0` includes partial completion; [inspect the result](TUTORIAL.md#read-the-result) before merging.

The worktree can contain copied gitignored build inputs, including local environment files. Apply the source repository's access controls to it and the run records.

## Provider response examples

The appendix uses the [optional JavaScript fixture](fixtures/README.md#optional-javascript-example) to illustrate deleting an unreachable module. It is current target-language support, not a Node implementation of the CLI. These are illustrative responses, not captured model results.

The Go tests validate all ten examples against [response schemas](go/internal/engine/schemas.json). Phase instructions live in [prompt templates](go/internal/engine/templates/). Both language editions retain identical JSON; `schema_version: "1"` here is independent of configuration schema 2 and report schema 3.

<details>
<summary>Expand schema-checked examples</summary>

### audit-success

<!-- example: audit-success schema: audit -->
```json
{
  "schema_version": "1",
  "scan_scope": "Tracked source, tests, configuration and package surface in the fixture.",
  "rejected_count": 1,
  "notes": "Illustrative response, not a captured run. The plugin-x registry entry rules out deleting that plugin.",
  "candidates": [
    {
      "candidate_id": "dead-legacy-parser",
      "category": "DEAD_CODE",
      "title": "Remove the unreferenced legacy parser",
      "related_files": [
        "src/legacy-parser.mjs"
      ],
      "problem": "No existing entrypoint reaches this module.",
      "minimal_change": "Delete src/legacy-parser.mjs.",
      "observable_contracts": [
        "Keep the exports and behavior of src/index.mjs unchanged."
      ],
      "static_reachability": "EXPORTED_UNUSED",
      "dynamic_reachability": "NONE",
      "external_consumer_risk": "NONE",
      "ui_impact": "NONE",
      "persistence_impact": "NONE",
      "async_side_effect_impact": "NONE",
      "test_protection": "PARTIAL",
      "characterization_needed": false,
      "estimated_file_count": 1,
      "primary_symbol": "parseLegacy",
      "risk_level": "L0_LOW",
      "readiness": "READY",
      "evidence": [
        {
          "file": "src/legacy-parser.mjs",
          "locator": "parseLegacy and legacyVersion exports",
          "kind": "DEFINITION",
          "supports": "The module contains only the legacy parser and version helper."
        },
        {
          "file": "src/index.mjs",
          "locator": "Repository-wide search of imports, strings, registry entries, tests and configuration",
          "kind": "GREP_ABSENCE",
          "supports": "No reference reaches legacy-parser, parseLegacy or legacyVersion from an existing entrypoint."
        },
        {
          "file": "package.json",
          "locator": "private and entrypoint fields",
          "kind": "CONFIG",
          "supports": "The fixture has no declared public package surface."
        }
      ]
    }
  ]
}
```

### deepcheck-success

<!-- example: deepcheck-success schema: deepcheck -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "category": "DEAD_CODE",
  "title": "Remove the unreferenced legacy parser",
  "minimal_change": "Delete src/legacy-parser.mjs.",
  "allowlist": [
    "src/legacy-parser.mjs"
  ],
  "forbidden_files": [
    "src/registry.mjs",
    "src/plugin-x.mjs"
  ],
  "contracts": [
    {
      "contract_id": "C1",
      "statement": "Existing src/index.mjs exports, return values and side effects stay unchanged.",
      "kind": "RETURN_SHAPE",
      "verified_by": "Inspect reachable imports and run the existing entrypoint tests and build."
    }
  ],
  "stop_conditions": [
    "Stop if an existing entrypoint or package consumer reaches the module.",
    "Stop if removal requires edits outside the allowlist."
  ],
  "rollback": {
    "strategy": "GIT_RESET_HARD",
    "detail": "The controller restores only the detached run worktree to its recorded pre-write commit."
  },
  "risk_level": "L0_LOW",
  "characterization_needed": false,
  "characterization_files": [],
  "readiness": "READY",
  "readiness_reason": "The fixture has no reachable consumer of this module; the existing entrypoint checks cover the retained behavior.",
  "notes": "Illustrative task packet. The controller adds fp and original_lines when saving packet.json."
}
```

### preflight-success

<!-- example: preflight-success schema: preflight -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "verdict": "READY_TO_EXECUTE",
  "failure_hypothesis": "A string-based loader still resolves the legacy parser.",
  "falsification_method": "Inspect the registry, imports, scripts and configuration for the module path and exported symbols.",
  "falsification_result": "FALSIFIED",
  "blocking_reasons": [],
  "notes": "Illustrative evidence: registry references plugin-x, not the legacy parser."
}
```

### execute-success

<!-- example: execute-success schema: execute -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "changed_files": [],
  "deleted_files": [
    "src/legacy-parser.mjs"
  ],
  "rationale": "Remove the module that no existing entrypoint reaches.",
  "hunks": [
    {
      "hunk_id": "H1",
      "file": "src/legacy-parser.mjs",
      "classification": "INTERNAL_ONLY",
      "justification": "The removed exports have no reachable consumer in the fixture.",
      "contract_ids": [
        "C1"
      ]
    }
  ],
  "verdict": "PASS",
  "scope_expansion_required": false,
  "scope_expansion_reason": null,
  "notes": "The controller applies the declared deletion and checks the resulting Git diff."
}
```

### review-success

<!-- example: review-success schema: review -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "verdict": "PASS",
  "behavior_preservation_assessment": "PRESERVED",
  "unreviewable_hunks": [],
  "findings": [],
  "notes": "Illustrative review: deletion is confined to the unreferenced module; existing entrypoints remain unchanged."
}
```

### deepcheck-evidence

<!-- example: deepcheck-evidence schema: deepcheck -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "category": "DEAD_CODE",
  "title": "Remove the unreferenced legacy parser",
  "minimal_change": "Delete src/legacy-parser.mjs.",
  "allowlist": [
    "src/legacy-parser.mjs"
  ],
  "forbidden_files": [
    "src/registry.mjs",
    "src/plugin-x.mjs"
  ],
  "contracts": [
    {
      "contract_id": "C1",
      "statement": "Existing src/index.mjs exports, return values and side effects stay unchanged.",
      "kind": "RETURN_SHAPE",
      "verified_by": "Inspect reachable imports and run the existing entrypoint tests and build."
    }
  ],
  "stop_conditions": [
    "Stop if an existing entrypoint or package consumer reaches the module.",
    "Stop if removal requires edits outside the allowlist."
  ],
  "rollback": {
    "strategy": "GIT_RESET_HARD",
    "detail": "The controller restores only the detached run worktree to its recorded pre-write commit."
  },
  "risk_level": "L0_LOW",
  "characterization_needed": false,
  "characterization_files": [],
  "readiness": "NEEDS_EVIDENCE",
  "readiness_reason": "The available configuration does not establish whether a loader reaches the module.",
  "notes": "Alternative branch, not part of the successful fixture example."
}
```

### preflight-blocked

<!-- example: preflight-blocked schema: preflight -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "verdict": "BLOCKED",
  "failure_hypothesis": "A string-based loader still resolves the legacy parser.",
  "falsification_method": "Inspect the registry, imports, scripts and configuration for the module path and exported symbols.",
  "falsification_result": "INCONCLUSIVE",
  "blocking_reasons": [
    {
      "code": "EVIDENCE_STALE",
      "detail": "The available registry evidence does not cover the current source snapshot.",
      "file": "src/registry.mjs"
    }
  ],
  "notes": "Alternative branch. No refactor should start with this result."
}
```

### characterization-packet

<!-- example: characterization-packet schema: deepcheck -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "category": "DEAD_CODE",
  "title": "Remove the unreferenced legacy parser",
  "minimal_change": "Delete src/legacy-parser.mjs.",
  "allowlist": [
    "src/legacy-parser.mjs"
  ],
  "forbidden_files": [
    "src/registry.mjs",
    "src/plugin-x.mjs"
  ],
  "contracts": [
    {
      "contract_id": "C1",
      "statement": "Existing src/index.mjs exports, return values and side effects stay unchanged.",
      "kind": "RETURN_SHAPE",
      "verified_by": "Inspect reachable imports and run the existing entrypoint tests and build."
    }
  ],
  "stop_conditions": [
    "Stop if an existing entrypoint or package consumer reaches the module.",
    "Stop if removal requires edits outside the allowlist."
  ],
  "rollback": {
    "strategy": "GIT_RESET_HARD",
    "detail": "The controller restores only the detached run worktree to its recorded pre-write commit."
  },
  "risk_level": "L0_LOW",
  "characterization_needed": true,
  "characterization_files": [
    "test/entrypoint-characterization.test.mjs"
  ],
  "readiness": "READY",
  "readiness_reason": "Reachability is established, but record the retained entrypoint return shape before deleting the module.",
  "notes": "Alternative test-protection scenario; the successful example skips characterization."
}
```

### characterization-success

<!-- example: characterization-success schema: characterization -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "changed_files": [
    "test/entrypoint-characterization.test.mjs"
  ],
  "characterized_contracts": [
    "C1"
  ],
  "production_source_changed": false,
  "assertions_weakened": false,
  "verdict": "PASS",
  "notes": "Illustrative response. The controller must still validate the actual test diff and run it against unchanged production code."
}
```

### review-rejected

<!-- example: review-rejected schema: review -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "verdict": "FAIL",
  "behavior_preservation_assessment": "CANNOT_DETERMINE",
  "unreviewable_hunks": [
    "H1"
  ],
  "findings": [
    {
      "finding_id": "R1",
      "severity": "BLOCKER",
      "file": "src/legacy-parser.mjs",
      "locator": "H1",
      "claim": "The reviewed evidence does not rule out a configured loader.",
      "why_it_matters": "Removing a reachable module would change an existing entrypoint.",
      "suggested_action": "Retain the module until the loader configuration is checked."
    }
  ],
  "notes": "Alternative branch illustrating review rejection, not a finding from the fixture."
}
```

</details>
