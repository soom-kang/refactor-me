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
| `run` | Start the automated refactoring loop |
| `report [--json] [--lang en\|ko]` | Read the latest supported saved report |
| `clean` | Remove eligible finished worktrees |

`init`, `doctor`, `run`, `report` and `clean` accept `--repo <path>`. A relative path starts at the calling directory; the CLI finds its Git root. Without `--repo`, it finds the current directory's Git root. `help` and `version` work outside Git. Homebrew manages installation.

Configuration, locks and runs belong to the selected repository, independently of the executable's Homebrew path.

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
| `agents.<name>.model` | Empty | Use the provider CLI's model default |
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

Provider configuration can override these values; per-phase settings take precedence.

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

The loop defines each phase's allowed skills, output schema, and permissions. Model and effort settings come from the loop's configuration; skill recommendations do not change them.

Run reports record the resolved Skill paths and content hashes, CLI/provider versions and the delivery path used for each provider. Keep these records when comparing runs after a global catalog update.

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

`--lang en|ko` applies to `run` and `report`; the default is `en`. It translates the final summary, `report.md`, and fixed handoff header. Progress logs and doctor output remain in English. Model explanations, errors, paths, and commit subjects keep their original wording.

Each run writes one `report.md` and one `report.json`. `report --lang ko` renders the latest JSON without changing files. Current reports use `schemaVersion: 3`. Missing, invalid or unsupported older JSON produces an error and preserves existing files. Reports are not automatically migrated. `--json` prints the stored JSON bytes for a supported report and is independent of language.

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
| Unsupported report schema | Error; stored files remain unchanged |

Viewing a report does not recollect the comparison. `accepted.patch` is a pre-review snapshot and may belong to a rejected candidate. Use the final comparison to inspect committed changes.

### Usage and exit codes

Failed calls and schema-repair processes count toward usage. Missing cost is not zero; a mixed total with missing costs is a lower bound.

**The loop does not enforce a total monetary budget.** Claude’s budget option is a provider setting, not a total run limit. Set cycle, commit, and elapsed-time limits.

| Exit code | Meaning |
| --- | --- |
| `0` | Completed or partially completed; inspect status and branch |
| `2` | Aborted or CLI error; inspect diagnostics |
| `4` | Safety invariant failed; worktree retained as evidence |

Inspect `.refactor/runs/<id>/` for state, events, provider outputs, validation evidence, and reports. `last-run.json` points to the latest result. Supported records retain their worktree path independently of the current cache default. Unsupported older formats are not read or cleaned by the current CLI.

## Local development checks

Build and test with Go using the [development guide](DEVELOPMENT.md). Generate disposable repositories with the [fixture guide](fixtures/README.md). Live provider loading and model decisions require separate checks.
