# refactor-me reference

[Project](../README.md) · [한국어](README.ko.md) · [Guide](TUTORIAL.md) · [Workflow](WORKFLOW.md) · [Changelog](CHANGELOG.md)

[Configuration](#configuration) · [Validation](#validation-commands) · [Reports](#reports-and-language) · [Skills](#skill-availability)

Follow the [guide](TUTORIAL.md) for installation.

## Configuration

The installer creates `.refactor/config.json` if absent and preserves it on reinstall. Runtime defaults are in `src/config.mjs` and `src/loop.mjs`; the installation template is [config.default.json](config.default.json).

| Setting | Default | Effect |
| --- | --- | --- |
| `workspace.branch_prefix` | `refactor/auto-` | Prefix for result branches |
| `workspace.worktree_parent` | Empty | Use `~/.cache/refactor-me`; a nonempty path overrides it |
| `workspace.keep_worktree` | `true` | Keep worktrees; `false` removes them after `DONE` or `NO_CHANGES` |
| `agents.primary` / `agents.fallback` | `codex` / `claude` | Initial provider order |
| `agents.<name>.enabled` | `true` | Enable a provider |
| `agents.<name>.bin` | Provider name | CLI executable |
| `agents.<name>.model` | Empty | Use the provider CLI's model default |
| `agents.<name>.timeout_sec` | `1800` | Provider call timeout; doctor uses a shorter probe timeout |
| `agents.<name>.effort_by_phase` | Empty | Override effort for individual phases |
| `agents.<name>.effort` | Unset | Override the default effort table; a per-phase value takes precedence |
| `agents.claude.max_budget_usd` | `0` | Pass a positive value to Claude's per-call budget option |
| `policy.allowed_risks` | `L0_LOW`, `L1_MODERATE`, `L2_HIGH` | Eligible risk levels; `UNKNOWN` and `L3_CRITICAL` remain excluded |
| `policy.max_cycles` / `policy.max_commits` | `25` / `20` | Cycle and refactor-commit limits |
| `policy.max_wall_clock_min` | `180` | Elapsed-time limit checked between units of work |
| `policy.max_consecutive_failures` | `3` | Stop after repeated candidate failures |
| `policy.max_attempts_per_fingerprint` | `2` | Attempt limit for a candidate fingerprint |
| `policy.empty_audits_to_stop` | `2` | Empty audits required to finish |
| `policy.max_files_per_candidate` | `8` | Candidate file limit |
| `policy.max_changed_lines` | `600` | Base change-size limit; split candidates have a derived allowance |
| `policy.max_audit_candidates` | `8` | Candidate selection cap |
| `policy.cooldown_minutes` | `20` | Provider cooldown after resource exhaustion |
| `policy.auto_characterization` | `true` | Permit characterization tests where needed |
| `policy.cross_provider_review` | `true` | Prefer another provider for review |
| `policy.extra_forbidden_globs` | Empty | Add forbidden paths without removing built-in restrictions |

Default reasoning effort:

- `high`: audit, deep check, execution, review
- `medium`: preflight, characterization, re-audit
- `low`: doctor, handoff

Provider configuration can override these values; per-phase settings take precedence.

`schema_version` and `verification` remain in the configuration format. The current loop does not use `verification.locked` to choose validation commands. For explicit commands, use `.refactor/commands.json` as described below.

## Candidate selection

The audit looks for four categories:

| Category | Candidate |
| --- | --- |
| `DEAD_CODE` | Code with no remaining reference from a reachable entrypoint |
| `COMPATIBILITY_REMOVAL` | Obsolete compatibility or migration code |
| `DEDUPLICATION` | Logic repeated in at least three places |
| `LARGE_COMPONENT_SPLIT` | A file over 500 lines with an existing boundary for a split |

Preserve return values, side effects, ordering, errors, rendered output, and persisted data shapes reachable from existing entrypoints and published surfaces. Bug fixes and speculative improvements are excluded.

`--target <dir>` is relative to the current directory and can be repeated.

- It limits candidate discovery. Caller and reachability checks cover the repository.
- Candidates must relate to a target and change a target file. Invalid paths or targets without tracked source files stop execution.
- Candidate validation selects affected areas and the root area when present. See [validation scope](#validation-commands).

## Execution and safety checks

Audit → deep check → optional characterization → preflight → execution → validation → independent review. Accepted commits trigger another audit.

The tool creates a detached worktree outside the source repository. It can copy gitignored build inputs, including dependencies and local environment files. **Apply the source repository’s access controls to the worktree and run records.**

The controller compares the diff with the frozen task packet: forbidden paths, allowed files, change size and category-specific limits, test integrity, and seen tree hashes. It checks for changes to the source checkout and approved worktree state. Rejected edits are rolled back inside the run worktree.

The CLI creates local commits and a result branch. It does not merge, push, deploy, or install application dependencies as a separate step. Existing validation commands and build tools may use the network and populate caches.

`sharpen-cold-review` runs in a separate session. When both providers are available and cross-provider review is enabled, it prefers the provider that did not implement the change.

Quota or authentication failure can switch providers within the same phase. The new session receives that phase’s inputs and frozen task packet where needed. `handoff.md` records the transition; the report contains the final result.

## Skill availability

Install all eight Skills from `soom-kang/sharpen-me`, a separately released required dependency, in the target repository. Review and commit their files, agent links, and `skills-lock.json`.

Doctor checks required Skill paths on disk and in the base checkout.

| Provider | Skill paths |
| --- | --- |
| Claude Code | `.claude/skills` with `--setting-sources project`; a global install alone is insufficient |
| Codex | `.agents/skills` and its supported home path |

Doctor calls models and writes diagnostics by default. A session reporting no visible Skills fails. A partial self-report does not override a complete disk installation. `--no-live-probe` skips this session check.

| Skill | Used when | Result |
| --- | --- | --- |
| `sharpen-clarify` | Audit and characterization need a defined scope | Scope and contracts to check |
| `sharpen-review` | Audit or deep check examines a candidate | Findings backed by repository evidence |
| `sharpen-challenge` | Audit, deep check, characterization, or preflight tests an assumption | Supported objections and checks |
| `sharpen-assess` | Audit assigns change risk | Risk classification; unknown risk excludes a candidate |
| `sharpen-refine` | Execution applies an approved task packet | A bounded change or a justified no-op |
| `sharpen-cold-review` | A separate session reviews the implementation | Review verdict and findings |
| `sharpen-brief` | Provider quota or authentication failure causes a handoff | A snapshot of the transition |
| `sharpen-dedupe` | A duplication candidate reaches deep check or execution | Duplicate analysis and scoped consolidation |

The loop defines each phase's allowed skills, output schema, and permissions. Model and effort settings come from the loop's configuration; skill recommendations do not change them.

The installed Skill files and `skills-lock.json` record the catalog used by this checkout.

## Validation commands

Discovery inspects project areas up to three directory levels deep.

| Source | Commands considered |
| --- | --- |
| `package.json` | Typecheck, lint, test, and build scripts with the detected package manager |
| `go.mod` | `go vet`, `go test -count=1`, `go build` |
| `pyproject.toml` | Ruff, mypy, pytest with a declared runner where applicable |
| `Cargo.toml` | `cargo check`, `cargo test --lib` |
| Gradle JVM build | `testClasses`, `test` |
| `pom.xml` | `-B test-compile`, `-B test` |
| `Makefile` | Relevant targets not already covered by a native command |

Gradle discovery requires a JVM plugin. Executable Gradle and Maven wrappers take precedence over PATH binaries; parent JVM builds cover their submodules.

Discovery excludes recognized browser, E2E, and service-dependent suites and records them in the report.

To supply explicit commands, create `.refactor/commands.json` in the target repository:

```json
{
  "locked": true,
  "commands": [
    {
      "id": ".:T2:node:test",
      "area": ".",
      "tier": "T2",
      "name": "test",
      "argv": ["node", "--test"],
      "cwd": ".",
      "timeoutMs": 180000,
      "source": "operator-defined"
    }
  ]
}
```

Use unique IDs, repository-relative `area` and `cwd`, argument arrays, and tiers `T1`, `T2`, or `T3`. A locked file replaces discovery. Review the commands before running; the controller executes them in the worktree.

### Baseline and candidate validation

The baseline runs selected commands across discovered areas, distinguishing passes, readable failures, unrunnable commands, and failures without useful signatures. **The run stops if no command passes.**

After a change, validation selects the deepest affected area for each path and the root area when present. These rules apply:

- Skip commands with baseline results `TIMEOUT`, `UNRUNNABLE`, or `OPAQUE`.
- Readable baseline failures must gain no new errors, and passing commands must keep passing. The first failed check stops validation and rejects the candidate.
- Matching baseline failures still count as failures. The result does not establish that every repository area passed.

## Reports and language

### Language and saved files

`--lang en|ko` applies to `run` and `report`; the default is `en`. It translates the final summary, `report.md`, and fixed handoff header. Progress logs and doctor output remain in English. Model explanations, errors, paths, and commit subjects keep their original wording.

Each run writes one `report.md` and one `report.json`. `report --lang ko` renders the latest JSON without changing files. Older JSON may omit version or usage fields. Missing or invalid JSON produces an error and preserves existing Markdown. `--json` returns the same data in either language.

### Code comparison

Compare `state.baseOid` with `state.publishedOid` in the source repository. The comparison includes net published changes and characterization commits, even after worktree cleanup.

- Preview: file statistics and three context lines; at most 200 complete lines or UTF-8 32 KiB.
- `changes.patch`: the full text patch, omitting binary bodies while retaining binary and file-mode metadata. External diff and textconv are disabled.

The optional `codeComparison` object in `report.json` contains:

| Field | Content |
| --- | --- |
| `status` | `AVAILABLE`, `NO_CHANGES` or `UNAVAILABLE` |
| `baseCommit`, `resultCommit` | Recorded full OIDs; `resultCommit` is null without publication |
| `files` | Path, old path for a rename, Git status, old/new mode, insertions, deletions and binary flag |
| `totals` | File count, text insertions/deletions and binary-file count; binary line counts are null per file |
| `patchFile` | `changes.patch`, relative to the run directory, or null if no patch was saved |
| `preview`, `truncated` | Stored diff excerpt and whether the full patch exceeds it |
| `error` | Original collection/storage error, or null |

| Comparison condition | Result |
| --- | --- |
| No published commit | `NO_CHANGES`, no patch |
| Published commits have identical trees | Empty patch |
| Missing Git objects or patch-storage failure | `UNAVAILABLE` with a reason; run status and exit code unchanged |
| Older JSON has no comparison field | Missing-comparison notice |

Viewing a report does not recollect the comparison. `accepted.patch` is a pre-review snapshot and may belong to a rejected candidate. Use the final comparison to inspect committed changes.

### Usage and exit codes

Failed calls and schema-repair processes count toward usage. Missing cost is not zero; a mixed total with missing costs is a lower bound.

**The loop does not enforce a total monetary budget.** Claude’s budget option is a provider setting, not a total run limit. Set cycle, commit, and elapsed-time limits.

| Exit code | Meaning |
| --- | --- |
| `0` | Completed or partially completed; inspect status and branch |
| `2` | Aborted or CLI error; inspect diagnostics |
| `4` | Safety invariant failed; worktree retained as evidence |

Inspect `.refactor/runs/<id>/` for state, events, provider outputs, validation evidence, and reports. `last-run.json` points to the latest result. Records retain their worktree path, so older records remain readable after cache defaults change.

## Local development checks

From the repository root:

```bash
node --test tool/test/*.test.mjs
node --check tool/bin/refactor-me.mjs
node tool/bin/refactor-me.mjs version --json
```

There is no separate lint, typecheck, or build command. Tests use Node.js built-ins and temporary repositories. **Live model decision quality and CLI Skill loading require separate provider runs.**
