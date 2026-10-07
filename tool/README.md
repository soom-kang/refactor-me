# refactor-me reference

[Project](../README.md) · [한국어](README.ko.md) · [Guide](TUTORIAL.md) · [Workflow](WORKFLOW.md) · [Changelog](CHANGELOG.md)

[Configuration](#configuration) · [Validation](#validation-commands) · [Reports](#reports-and-language) · [Skills](#skill-availability)

Follow the [guide](TUTORIAL.md) for installation.

## Commands and repository selection

| Command | Purpose |
| --- | --- |
| `help` or no arguments | Show usage without starting a run |
| `version [--json]` | Show build version, Go runtime, platform, architecture, executable source path and commit provenance |
| `init` | Create optional project configuration without overwriting existing settings |
| `doctor [--no-live-probe]` | Check repository, providers and global Skills; the default includes model calls |
| `run [--json] [--lang en\|ko]` | Start the automated refactoring loop |
| `report [--json] [--lang en\|ko]` | Read the latest supported saved report |
| `clean` | Remove eligible finished worktrees |

`init`, `doctor`, `run`, `report` and `clean` accept `--repo <path>`. A relative path starts at the calling directory; the CLI finds its Git root. Without `--repo`, it finds the current directory's Git root. `help` and `version` work outside Git. Homebrew manages installation.

Configuration, locks and runs belong to the selected repository, independently of the executable's Homebrew path.

This reference covers public Beta `0.10.0-beta.5`, including run time selection, detailed activity logs, Markdown change checklists and standard API cost estimates.

<a id="run-time-selection"></a>

## Run time selection

`run` accepts `--max-minutes <n>`, a positive whole number of minutes that fits a Go time duration. The option overrides `policy.max_wall_clock_min` for this run without saving the choice. It is rejected on other commands.

Without the option, a macOS run prompts only when both stdin and stderr are terminals and `--json` is absent. Choose `30`, `60`, `180`, `360`, or `custom`; Enter keeps the project/default limit. Invalid input retries; `q`, `cancel`, or end of input aborts before providers or worktrees start. Other platforms and redirected/JSON runs use configuration without prompting.

Priority is CLI option, terminal selection, then project/default configuration (`180` minutes). The clock starts after selection and includes run preparation. This is a soft limit: it is checked before a new audit and after audit before a candidate starts. An in-flight work unit finishes its validation, review and commit or rollback, and can exceed the limit. The report's optional `runLimits` records the selected minutes, source, `candidate_boundary` stop mode, actual seconds and whether the limit was reached. Per-provider `timeout_sec` is a separate limit.

## Model and effort selection

These options are available in public Beta `0.10.0-beta.5`.

`run` and live `doctor` require a model for every selected provider. There is no fixed model default. A command option overrides `agents.<provider>.model`; if both are empty, the command fails before making provider calls. Offline `doctor --no-live-probe` needs no model.

| Option | Applies to | Behavior |
| --- | --- | --- |
| `--model <id>` | Selected primary provider | Override its configured model for this command |
| `--fallback-model <id>` | Selected fallback provider | Override its configured model for this command |
| `--effort <level>` | Selected primary provider | Force one effort level across all phases in this command |
| `--fallback-effort <level>` | Selected fallback provider | Force one effort level across all phases in this command |

The options follow the selected provider order. For `--provider claude --fallback codex`, `--model` selects Claude's model and `--fallback-model` selects Codex's. Without provider flags, the order comes from project configuration, defaulting to Codex then Claude. An enabled fallback also needs a model even if the primary succeeds. Use `--fallback none` to select only one provider. Command options never write `.refactor/config.json`.

For example, select both providers and force `xhigh` for each:

```sh
refactor-me run --repo /path/to/target-repo \
  --provider codex --fallback claude \
  --model gpt-6.1-sol --fallback-model claude-sonnet-5-5 \
  --effort xhigh --fallback-effort xhigh
```

These model IDs are examples, not defaults or a guarantee of account access. The CLI passes nonblank model and effort strings as separate provider arguments without a built-in allowlist. Use values supported by your authenticated provider CLI; accepting an option does not verify provider support. The same options apply to live `doctor`; provider calls consume usage. Fallback model/effort options require a distinct selected fallback provider.

An omitted effort option preserves the existing policy. Operational overrides use `low` for doctor/handoff and `medium` for a re-audit. Other calls use `effort_by_phase`, then the provider's configured `effort`, then the phase defaults below. A CLI effort option overrides every level, including the operational values.

## Configuration

`refactor-me init --repo <path>` creates `.refactor/config.json` if absent and never overwrites it. Initialization is optional; commands use built-in defaults when configuration is absent. Runtime defaults are in `go/internal/surface/config.go` and `go/internal/controller`; the initialization template is [config.default.json](config.default.json). Configuration requires `schema_version: 2`. Unsupported or missing versions in an existing file are errors, not migration requests.

| Setting | Default | Effect |
| --- | --- | --- |
| `workspace.branch_prefix` | `refactor/auto-` | Prefix for result branches |
| `workspace.worktree_parent` | Empty | Use `~/.cache/refactor-me`; a nonempty path overrides it |
| `workspace.keep_worktree` | `true` | Keep worktrees; `false` removes them after `DONE` or `NO_CHANGES` |
| `agents.primary` / `agents.fallback` | `codex` / `claude` | Initial provider order |
| `agents.<name>.enabled` | `true` | Enable a provider |
| `agents.<name>.bin` | Provider name | CLI executable |
| `agents.<name>.model` | Empty | Model ID; required for live calls unless overridden on the command line |
| `agents.<name>.timeout_sec` | `1800` | Provider call timeout; doctor uses a shorter probe timeout |
| `agents.<name>.effort_by_phase` | Empty | Override effort for individual phases |
| `agents.<name>.effort` | Unset | Override the default effort table; a per-phase value takes precedence |
| `agents.claude.max_budget_usd` | `0` | Pass a positive value to Claude's per-call budget option |
| `policy.allowed_risks` | `L0_LOW`, `L1_MODERATE`, `L2_HIGH` | Eligible risk levels; `UNKNOWN` and `L3_CRITICAL` remain excluded |
| `policy.unknown_risk` | `set_aside` | How an `UNKNOWN` risk level is handled. `deep_check` sends an `UNKNOWN` candidate whose readiness is `NEEDS_EVIDENCE` to deep check instead of setting it aside; every other `UNKNOWN` is still set aside |
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

Provider configuration can override ordinary phase defaults; per-phase settings take precedence over the provider's configured effort. Doctor/handoff and re-audit keep their operational overrides. A CLI effort option forces one level for every call.

`verification` remains in the configuration format. The current loop does not use `verification.locked` to choose validation commands. For explicit commands, use `.refactor/commands.json` as described below.

## Candidate selection

The audit looks for four categories:

| Category | Candidate |
| --- | --- |
| `DEAD_CODE` | Code with no remaining reference from a reachable entrypoint |
| `COMPATIBILITY_REMOVAL` | Obsolete compatibility or migration code |
| `DEDUPLICATION` | Logic repeated in at least three places |
| `LARGE_COMPONENT_SPLIT` | A file over 500 lines with an existing boundary for a split |

Preserve return values, side effects, ordering, errors, rendered output, and persisted data shapes reachable from existing entrypoints and published surfaces. Bug fixes and speculative improvements are excluded.

`--target <dir>` can be repeated. With `--repo`, targets are relative to the selected Git root; without it, they are relative to the current directory. Real paths are resolved, including symlinks, and must remain inside the selected repository.

- It limits candidate discovery. Caller and reachability checks cover the repository.
- Candidates must relate to a target and change a target file. Invalid paths or targets without tracked source files stop execution.
- Candidate validation selects affected areas and the root area when present. See [validation scope](#validation-commands).

## Execution and safety checks

The [workflow](WORKFLOW.md) describes worktree isolation, diff gates, validation, independent review and commit checks. Results stay on a local branch until you review and merge them.

The worktree may contain gitignored build inputs, including local environment files. Protect worktrees and run records as you would the source repository. Validation commands may use the network and populate caches.

## Skill availability

Install all eight required Skills globally from sharpen-me in the [guide](TUTORIAL.md#install-the-tool-and-skills). The shared source is `~/.agents/skills`; a project-local copy does not replace a missing global Skill.

The CLI resolves each Skill directory, validates `SKILL.md` and its supporting files, and records the real path and SHA-256 of the full content. A Skill directory may be a symlink, but links inside it must not escape the resolved directory. Broken links and unsupported file structures are errors. Distinct same-name Skills in project/provider discovery paths block execution; aliases to the same canonical source are permitted and no user files are removed.

| Provider | Skill delivery |
| --- | --- |
| Codex | Native global discovery with the selected absolute Skill paths in phase prompts |
| Claude Code | A dedicated per-run `.claude/skills` copy passed through `--add-dir`; `--setting-sources project` continues to exclude user settings |

The Claude directory contains only the selected Skills and supporting files; the CLI does not grant access to the whole home directory through `--add-dir`. Content hashes are checked around provider calls. Unexpected drift halts the run before publishing that result.

Doctor validates the global source and delivery paths. By default it also calls models, consumes account usage and writes diagnostics. `--no-live-probe` checks local prerequisites and provider executables without model calls; it does not prove live session loading. Local fixture tests and live provider checks are separate evidence. The live probe requires the provider to report all eight visible Skill names; this is session self-report, not independent proof of every file read. File paths and hashes are measured separately.

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

The loop defines each phase's allowed skills, output schema, and permissions. Model and effort settings resolve from command options and project configuration; skill recommendations do not change them.

Run reports record the resolved Skill paths and content hashes, CLI/provider versions and the delivery path used for each provider. Keep these records when comparing runs after a global catalog update.

Run reports and doctor diagnostics also record `providerSettings` by provider: `requestedModel` is the model resolved from CLI/configuration, and optional `cliEffort` records an explicit command override. These are requested settings, not proof that a provider ran that model or effort. Offline doctor may record an empty requested model. Report schema remains `3`.

The report's `Default effort` phase column shows policy, not observed provider effort. Read the requested provider settings separately; neither field verifies provider execution.

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
      "id": ".:T2:go:test",
      "area": ".",
      "tier": "T2",
      "name": "test",
      "argv": ["go", "test", "-count=1", "./..."],
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

`--lang en|ko` applies to `run` and `report`; the default is `en`. It translates run progress logs, the final summary, `report.md`, and fixed handoff header. The standalone `doctor` command remains in English. Model explanations, stored errors, paths, identifiers and commit subjects keep their original wording.

Each run writes one `report.md` and one `report.json`. `report --lang ko` renders the latest JSON without changing files. Current reports use `schemaVersion: 3`. Missing, invalid or unsupported older JSON produces an error and preserves existing files. Reports are not automatically migrated. `--json` prints the stored JSON bytes for a supported report and is independent of language.

### Progress logs

The following behavior is included in public Beta `0.10.0-beta.5`.

- `run` reports environment checks, worktree preparation, baseline checks, audit, candidate checks, edits, validation, independent review and local commit publication. Each audit reports the proposed and eligible counts, plus the number inspected when a candidate cap applies. Later audits can find new candidates, so there is no fixed total or completion percentage.
- Candidate labels use the translated category, unchanged `primary_symbol` and repository-relative path, falling back to `candidate_id` when needed. Validation messages identify the command name and area, without argv or command output. An unchanged readable baseline failure remains a failure; it is never labeled as passed.
- Messages distinguish policy exclusions, rejected execution, no changes, validation regression, timeout and commands that cannot run. A rollback is confirmed only after restoration succeeds. A local commit is confirmed only after the result branch and state are saved.
- Long stages report the current stage and elapsed time every 30 seconds. These updates give no ETA or invented progress. Provider recovery messages describe schema repair, permission denial, retries and switching providers without displaying raw provider notes or tool commands.
- Progress always goes to `stderr`, including with `run --json`; `stdout` then contains only report JSON. Failure guidance points to a successfully saved `state.json` or an existing phase/doctor record. If no relevant record was created, it says so.

Dynamic display values are reduced to one line, stripped of terminal control sequences and limited to 160 Unicode characters. If this formatting would change an absolute diagnostic path, the CLI shows a path relative to the selected repository and labels it accordingly. Original provider transcripts, tool-call accounting and validation output remain in the local run records; this display limit does not truncate those saved originals. Protect and inspect records before sharing them.

The CLI adds elapsed time and stage/provider context to these logs by default. Structured provider events identify read, search, edit, write, deletion and generic command/tool activity, with safe worktree-relative paths when available. Codex command events stay generic; the CLI does not interpret shell text to guess what was read. Sensitive or outside-worktree paths, search patterns, command arguments, output and provider prose are omitted. Provider completion reports measured duration, recognized tool-event count and exit status. A Claude assistant event containing one or more tool uses counts once. Tool activity is separate from validated changes and accepted commits; existing tool-call accounting is preserved.

<a id="code-comparison"></a>

### Code comparison

Compare `state.baseOid` with `state.publishedOid` in the source repository. The comparison includes net published changes and characterization commits, even after worktree cleanup.

- Preview: file statistics and three context lines; at most 200 complete lines or UTF-8 32 KiB.
- `changes.patch`: the full text patch, omitting binary bodies while retaining binary and file-mode metadata. External diff and textconv are disabled.
- `changes.md`: a file review checklist with status, additions/deletions, rename and mode metadata, followed by the complete text patch. Checkboxes record human review; they do not select changes or modify the result branch. Editing the checklist leaves `changes.patch` unchanged.

The optional `codeComparison` object in `report.json` contains:

| Field | Content |
| --- | --- |
| `status` | `AVAILABLE`, `NO_CHANGES` or `UNAVAILABLE` |
| `baseCommit`, `resultCommit` | Recorded full OIDs; `resultCommit` is null without publication |
| `files` | Path, old path for a rename, Git status, old/new mode, insertions, deletions and binary flag |
| `totals` | File count, text insertions/deletions and binary-file count; binary line counts are null per file |
| `patchFile` | `changes.patch`, relative to the run directory, or null if no patch was saved |
| `markdownFile`, `markdownError` | `changes.md` if saved, and a separate Markdown write error if any |
| `preview`, `truncated` | Stored diff excerpt and whether the full patch exceeds it |
| `error` | Original collection/storage error, or null |

| Comparison condition | Result |
| --- | --- |
| No published commit | `NO_CHANGES`, no patch |
| Published commits have identical trees | Empty patch |
| Missing Git objects or patch-storage failure | `UNAVAILABLE` with a reason; run status and exit code unchanged |
| Unsupported report schema | Error; stored files remain unchanged |

Viewing a report does not recollect the comparison. `accepted.patch` is a pre-review snapshot and may belong to a rejected candidate. Use the final comparison to inspect committed changes.

`changes.md` preserves binary metadata without binary bodies and is still saved for `NO_CHANGES` or `UNAVAILABLE` when possible. A Markdown write failure leaves the patch/report evidence and run status intact. Report viewing does not overwrite checked review boxes or update saved evidence.

<a id="usage-and-exit-codes"></a>

### Usage and exit codes

Failed calls and schema-repair processes count toward usage. Provider-reported USD totals include only calls that supplied a price. Missing prices are unknown rather than zero; the estimates below are shown separately.

The CLI uses provider-reported USD first. A missing price can receive an offline standard API estimate from bundled rates checked on **2026-10-07**, using [Artificial Analysis](https://artificialanalysis.ai/) with provider documentation for cache rates. Exact supported IDs are Codex `gpt-5.6-sol`, Codex `gpt-6.1-sol`, and Claude `claude-sonnet-5-5`; aliases are not guessed. Pricing is not fetched during a run or report viewing.

Each estimate records the requested model, token categories, rates, source URLs, checked date and assumptions. Cached input and cache writes use separate rates. Reasoning output is included in output tokens and is not charged twice. Missing cache fields are assumed zero and noted; missing Claude cache-write TTL assumes five minutes and is noted. Estimates assume standard API pricing without speed, region, batch or long-context modifiers and do not calculate ChatGPT/Codex/Claude subscription charges. Requested models do not prove executed models.

The report separates reported and estimated USD, their combined known amount, and calls still unpriced because of an unknown model or missing/invalid usage. A partial amount is not a complete bill; unpriced calls remain unknown rather than zero. Optional schema-3 fields include `costPolicy`, token splits, `estimatedCostUsd`, per-call `costEstimates` and `estimateMissingReasons`. Old saved reports remain readable without retroactive estimates.

If the initial doctor or run preparation fails, no final run report is generated; inspect any saved `doctor.json` for the observed usage and cost.

**The loop does not enforce a total monetary budget.** Claude’s budget option is a provider setting, not a total run limit. Set cycle, commit, and elapsed-time limits.

| Exit code | Meaning |
| --- | --- |
| `0` | Completed or partially completed; inspect status and branch |
| `2` | Aborted or CLI error; inspect diagnostics |
| `4` | Safety invariant failed; worktree retained as evidence |

Inspect `.refactor/runs/<id>/` for state, events, provider outputs, validation evidence, and reports. `last-run.json` points to the latest result. Supported records retain their worktree path independently of the current cache default. Unsupported older formats are not read or cleaned by the current CLI.

## Local development checks

Build and test with Go using the [development guide](DEVELOPMENT.md). Generate disposable repositories with the [fixture guide](fixtures/README.md). Live provider loading and model decisions require separate checks.

The cancellation and latest-failure behavior below is **Unreleased** for Beta.6 builds; it is not included in the published Beta.5 binary.

Ctrl-C or SIGTERM during `run`/`doctor` cancels managed provider and validation processes. Cancellation stops retries and fallback; accepted commits remain intact and an unfinished worktree is retained for inspection. SIGKILL cannot provide cleanup guarantees.

If run preparation fails before a final report, `report` returns exit 2 and shows the latest failure rather than a stale successful result. The previous completed report stays in its original run directory; a later completed run clears the failure marker.
