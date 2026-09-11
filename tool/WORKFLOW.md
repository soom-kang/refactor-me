# Refactoring workflow

[Project](../README.md) · [한국어](WORKFLOW.ko.md) · [Reference](README.md) · [Guide](TUTORIAL.md)

Start with the flow, then expand the JSON examples for the phase you need. You choose the configuration and start the run; the controller checks provider JSON responses and decides whether to proceed. It does not ask you to approve each phase.

The example removes `src/legacy-parser.mjs` from the [local fixture](fixtures/make-fixture.mjs). It exports `parseLegacy` and `legacyVersion` without a reachable caller. The same fixture loads `plugin-x` by name through a registry, so static import searches alone would miss that reference.

**JSON examples are illustrative, not captured responses or proof of a successful run.** Both languages use the same examples and current [response schemas](src/schemas.mjs). Questions summarize the [phase prompts](src/prompts.mjs).

## Flow

![Workflow overview: initialization, baseline, audit, preparation, execution, checks, review, publication and final report.](../docs/assets/workflow/overview.en.png)

[HTML source](../docs/assets/workflow/overview.en.html) · [Source and fidelity record](../docs/assets/workflow/README.md)

The controller checks run limits and provider availability between steps. Ordinary rejection restores the pre-write state, then leads to another audit or a configured stop. A safety halt preserves the worktree as evidence.

## 1. INIT, doctor and baseline

**Input:** Check the repository, configuration, providers, and eight required Skills. Doctor calls models to check session Skill visibility, using a diagnostic response distinct from audit JSON. `doctor --no-live-probe` skips the session check.

**Check and next step:** Missing prerequisites abort the run. After initialization, run baseline commands in the detached worktree. Use discovered commands unless `.refactor/commands.json` locks a list. At least one selected command across discovered areas must be `GREEN` to reach audit.

`src/broken.mjs` illustrates a readable typecheck failure (`RED`); other commands can pass. This describes the fixture design, not a measured baseline for this document.

Later validation must preserve passing checks and add no error signatures to readable failures. Baseline `TIMEOUT`, `UNRUNNABLE`, and `OPAQUE` commands provide no comparison signal and are excluded.

## 2. Audit and candidate ranking

**Input:** “Find a bounded behavior-preserving refactor. Check static and dynamic reachability, external consumers, observable contracts and risk. Reject speculative candidates.” The session uses `sharpen-clarify`, `sharpen-review`, `sharpen-challenge` and `sharpen-assess`.

**Response:** Propose the legacy parser as `DEAD_CODE`, `L0_LOW`, and `READY`, with absent-reference evidence. `EXPORTED_UNUSED` alone does not rule out external consumers.

**Check and next step:** `rank` checks scope, `UNKNOWN` and disallowed risk, `REJECT`, candidate history, attempt limits, and file limits. Eligible candidates sort by risk → readiness → file count → candidate ID.

Audit `NEEDS_EVIDENCE` can reach deep check. Record the selected candidate’s attempt; empty audits stop at the configured count, which defaults to two.

<details>
<summary>Full JSON response</summary>

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

</details>

## 3. Deep check and the task packet

**Input:** “Verify the candidate against current repository evidence. Name the smallest allowlist, preserved contracts, stop conditions and any characterization tests needed.” The session uses `sharpen-review` and `sharpen-challenge`; deduplication candidates also use `sharpen-dedupe`.

**Response:** `READY`, one allowed deletion, contract `C1`, and `characterization_needed: false`.

**Check and next step:** Add `fp` and `original_lines` and save `packet.json`. Require `READY` and a nonempty allowlist. This example proceeds to preflight; see the [characterization branch](#characterization-can-be-the-only-published-change) when tests are needed.

<details>
<summary>Full JSON response</summary>

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

</details>

## 4. Preflight

![Execution preparation: deep check, optional characterization tests, separate test commit and preflight decisions.](../docs/assets/workflow/preparation.en.png)

[HTML source](../docs/assets/workflow/preparation.en.html) · [Source and fidelity record](../docs/assets/workflow/README.md)

**Input:** “State the strongest concrete failure hypothesis for this packet and try to falsify it.” The session uses `sharpen-challenge` and reads the packet, repository facts and baseline summary without editing code.

**Response:** The string-loader hypothesis is `FALSIFIED`, the verdict is `READY_TO_EXECUTE`, and there are no blocking reasons.

**Check and next step:** Save `preflight.json` and require all three conditions before execution. `SURVIVED` or `INCONCLUSIVE` blocks execution even with a `READY_TO_EXECUTE` verdict.

<details>
<summary>Full JSON response</summary>

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

</details>

## 5. Execute

**Input:** “Apply only the frozen packet. Classify each hunk and declare any required scope expansion.” The session uses `sharpen-refine`; deduplication also allows `sharpen-dedupe`.

**Response:** Declare `deleted_files` with contract `C1` and return `PASS` without scope expansion. The controller handles file deletion, Git operations, and validation commands.

**Check and next step:** Save `execution.json`. `UNEXPLAINED` or `CONTRACT_CHANGING` hunks, scope expansion, and out-of-scope path claims can override `PASS`. The controller checks the allowlist, deletes the file, and inspects the actual diff at the gate.

<details>
<summary>Full JSON response</summary>

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

</details>

## 6. Diff gate and validation

**Check:** Without calling a model, compare the diff with the packet. Check allowed and forbidden paths, binaries, change size, category rules, test weakening, seen trees, and source-checkout integrity. Save `gate.json`.

**Next step:** Skip `NO_OP`; roll back gate violations. An unsafe invariant stops the run and preserves evidence. A passing gate leads to validation and `validation.json`. Reject regressions or unusable new validation results before review.

Validation selects the deepest affected area for each path and the root area when present, rather than every area. The baseline runs discovered commands; reachability checks cover the repository. Caller changes may extend beyond `--target`, and root commands may check more than the selected directories.

Save `accepted.patch` after validation and **before review**. It is review input and can remain after rejection. Check `review.json`, published commits, and final `changes.patch` for retained changes.


## 7. Independent review

**Input:** “Judge behavior preservation from the task packet and diff.” The session uses `sharpen-cold-review`. It receives the implementation rationale, but not the implementer's verdict or hunk classifications. It runs in a separate session; cross-provider review prefers another available provider when enabled.

**Response:** `PASS`, `PRESERVED`, no findings and no unreviewable hunks.

**Check and next step:** Save `review.json`. A `BLOCKER`, an assessment other than `PRESERVED`, or the model’s `FAIL` rejects the change and restores the pre-execution commit.

**A `HIGH` finding alone does not override `PASS`.** Read findings as well as the verdict.

<details>
<summary>Full JSON response</summary>

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

</details>

## 8. Commit, re-audit and report

**Publish:** Require the commit tree to match the reviewed tree. Create a local commit and update the result branch only if its previous OID matches the expected value. Record `state.publishedOid` and the commit, then re-audit within limits. Unexpected source or approved-tree changes can stop publication.

**Result:** At termination, save `report.json` and the selected-language `report.md`. Compare the source repository’s `state.baseOid` and `state.publishedOid`, including characterization commits. Worktree retention and the branch’s current position do not affect this comparison.

The report provides file statistics, a diff preview, and the full text `changes.patch`. Net changes may differ from individual commit contents when multiple commits edit a file. See [code comparison](README.md#code-comparison) for preview limits and binary handling.

`codeComparison.status` is `AVAILABLE`, `NO_CHANGES`, or `UNAVAILABLE`. No publication means no committed changes. Collection or patch-storage failures record a reason while preserving the run outcome and exit code.

Older JSON without `codeComparison` shows a missing-comparison notice. `report --lang ko` renders saved JSON without new diffs, file changes, or model calls.

## Failure and recovery branches

![Rejection restores candidate edits before re-audit; a safety halt retains the worktree and produces a report.](../docs/assets/workflow/outcomes.en.png)

[HTML source](../docs/assets/workflow/outcomes.en.html) · [Source and fidelity record](../docs/assets/workflow/README.md)

### Insufficient evidence and unknown risk

Audit `risk_level: "UNKNOWN"` is excluded as `RISK_UNKNOWN` even with `READY`. `NEEDS_EVIDENCE` can reach deep check. Deep check responses other than `READY` record `NOT_READY` and stop the candidate before edits.

<details>
<summary>Full JSON response</summary>

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

</details>

### Preflight blocks execution

A surviving or inconclusive hypothesis blocks implementation. The example uses stale evidence to return `INCONCLUSIVE` for the loader hypothesis and records `PREFLIGHT_BLOCKED`. Earlier characterization commits may remain.

<details>
<summary>Full JSON response</summary>

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

</details>

### Characterization can be the only published change

When the packet requests characterization and `policy.auto_characterization` is enabled, add tests for existing contracts. The model uses `sharpen-clarify` and `sharpen-challenge` and may edit only `characterization_files`.

The scenario below requests an entrypoint test. Do not create a caller just to make a dead module observable.

<details>
<summary>Full task packet JSON</summary>

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

</details>

The response reports test paths and preserved contracts. The controller rejects production edits, weakened assertions, and out-of-scope changes even with `PASS`.

Run tests against unchanged production code before a separate characterization commit. An empty diff makes no commit; missing paths or failing tests stop the candidate. Disabling automatic characterization skips test writing and proceeds to preflight.

<details>
<summary>Full JSON response</summary>

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

</details>

Later preflight blocks, validation regressions, or review rejections restore the commit **after characterization**. Test commits remain on the result branch, so the final comparison can include tests with zero refactor commits. `policy.max_commits` counts refactor commits only.

### Validation regression and review rejection

Previously passing commands must keep passing. New signatures in readable baseline failures, timeouts, or unrunnable commands reject the candidate. Matching failures still count as failures. Save results and the failure, then restore the pre-write commit.

The review below cannot establish behavior preservation, so the controller rolls back the refactor. Saved `accepted.patch` remains an input snapshot, not approval.

<details>
<summary>Full JSON response</summary>

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

### Schema repair, timeout and provider switching

| Event | Controller action | Next step |
| --- | --- | --- |
| Invalid response JSON or schema | One read-only repair request includes the validation errors and prior response | Revalidate; if it still fails with `SCHEMA`, try the other available provider |
| `TIMEOUT` or `PROCESS` failure | Retry the same provider after 5 seconds, then 20 seconds | After those retries, try the other available provider |
| `QUOTA` or `AUTH` | Update provider availability and attempt a handoff snapshot | Try the other provider within the same phase |
| Fatal provider error | Abort without repair or switching | Preserve diagnostics and report the stop reason |
| No provider completes the phase | Apply the phase's failure or provider-exhaustion handling | Record failure or terminate according to the controller state and limits |

Repair uses the original response schema, with no separate JSON contract. The repair session cannot continue editing. Process success still requires a valid response schema. Failed and repair processes count toward usage.

Switching starts a new session with the phase’s inputs; it does not resume the old one. In write phases, `callPhase` does not roll back before every retry or switch. Candidate handling performs rollback after execution failure or rejection.

`handoff.md` is a mid-run record for the reader, not the next provider’s conversation context or the final report.

## Evidence to inspect

| File | Meaning |
| --- | --- |
| `audits/*.json` | Candidate lists returned by audit |
| `cycles/*/packet.json` | Task packet plus controller metadata |
| `cycles/*/preflight.json` | Failure hypothesis and falsification result |
| `cycles/*/execution.json` | Implementation response and provider |
| `cycles/*/gate.json`, `validation.json`, `review.json` | Scope checks, executed validation and review evidence in that cycle |
| `cycles/*/accepted.patch` | Diff submitted for review, including candidates later rejected |
| `state.json` | Terminal state, counters and published commit OID |
| `report.json`, `report.md`, `changes.patch` | Final report and committed text comparison; unavailable comparisons record their reason |

Paths are relative to `.refactor/runs/<id>/`; cycle directories include category and fingerprint. Check [status, cost, and exit codes](README.md#usage-and-exit-codes) and [review before merging](TUTORIAL.md#read-the-result). Local tests check schemas and controller behavior, not live model decisions.
