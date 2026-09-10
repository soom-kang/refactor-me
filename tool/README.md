# refactor-me reference

[Project](../README.md) · [한국어](README.ko.md) · [Guide](TUTORIAL.md) · [Changelog](CHANGELOG.md)

Use this reference to configure a run and interpret its checks. The [guide](TUTORIAL.md) covers installation and result review.

## Candidate selection

The audit looks for four categories:

| Category | Candidate |
| --- | --- |
| `DEAD_CODE` | Code with no remaining reference from a reachable entrypoint |
| `COMPATIBILITY_REMOVAL` | Obsolete compatibility or migration code |
| `DEDUPLICATION` | Logic repeated in at least three places |
| `LARGE_COMPONENT_SPLIT` | A file over 500 lines with an existing boundary for a split |

Behavior preservation includes return values, side effects, ordering, errors, rendered output, and persisted data shapes. The loop assesses behavior reachable from existing entrypoints and published surfaces. It excludes bug fixes and speculative improvements.

`--target <dir>` is repeatable and resolves relative to the directory where you invoke the command. It limits candidate discovery, while checks of callers and validation remain repository-wide. Candidates must relate to a target, and the resulting change must include a target file. Invalid paths and targets without tracked source files stop execution.

## Execution and safety checks

The sequence is audit, deep check, optional characterization, preflight, execution, validation, and independent review. Accepted commits trigger another audit.

The tool creates a detached worktree outside the source repository. It can hydrate gitignored build inputs, including dependency directories and local environment files, into that worktree. Keep the worktree and run directory under the same access controls as the source repository.

After implementation, the controller checks the diff against the frozen task packet: forbidden paths, allowed files, change size, category-specific size rules, test integrity, and previously seen tree hashes. It also checks that the source checkout and approved worktree state have not changed unexpectedly. Rejected candidates are rolled back inside the run's worktree.

The tool creates local commits and a result branch. It does not merge, push, deploy, or install application dependencies as a separate step. Existing validation commands and build tools may access the network or populate caches.

`sharpen-cold-review` runs in a separate provider session. When both providers are available and cross-provider review is enabled, the loop prefers the provider that did not implement the change. A quota or authentication failure switches providers at a candidate boundary; the next provider receives the frozen task packet. `handoff.md` describes that transition, not the final result.

## Skill availability

Install all eight skills from `soom-kang/sharpen-me` into the target repository and commit their files and agent links. The [project README](../README.md#skills-in-the-loop) lists each skill's role.

Doctor resolves every required skill on disk. Claude uses `.claude/skills` under `--setting-sources project`. Codex uses `.agents/skills` and its supported home skill path. Repo-local skills must be present in the base checkout.

The live probe also asks each provider which skills it can see. A partial self-report does not override a complete disk installation; a session reporting none fails the visibility check. `--no-live-probe` skips that evidence. Doctor writes diagnostics and normally makes provider calls, even though it does not refactor code.

## Configuration

The installer creates `.refactor/config.json` only when it is absent. Reinstalling preserves your file. Runtime defaults come from `src/config.mjs` and `src/loop.mjs`; the complete installation template is [config.default.json](config.default.json).

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

The default effort is `high` for audit, deep check, execution, and review; `medium` for preflight and characterization; `low` for doctor and handoff. Re-audits request `medium` unless the provider configuration overrides it.

`schema_version` and `verification` remain in the configuration format. The current loop does not use `verification.locked` to choose validation commands. For explicit commands, use `.refactor/commands.json` as described below.

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

Gradle discovery requires an applied JVM plugin. Executable Gradle and Maven wrappers take precedence over PATH binaries. Parent JVM builds cover their submodules. Discovery excludes recognized browser, E2E, and service-dependent suites; the report records omitted checks.

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

Use unique IDs, repository-relative `area` and `cwd`, argument arrays, and tiers `T1`, `T2`, or `T3`. A locked file replaces discovery. Review these commands before running: the controller executes them in the worktree.

Baseline results distinguish passing checks, readable failures, commands that cannot run, and failures without useful signatures. A readable baseline failure is compared with later failures; it is not counted as a passing check. New errors and failures of previously passing commands reject a candidate. A baseline without a passing signal stops the run.

## Reports and language

`--lang en|ko` is supported by `run` and `report`, with `en` as the default. It changes the final summary, `report.md`, and fixed handoff header. Progress logs and doctor output remain in English. Model explanations, raw errors, paths, and commit subjects are not translated.

Each run writes one `report.md` and one `report.json`. `report --lang ko` reads the latest JSON and renders Korean to stdout without updating stored artifacts. It works for older JSON reports with missing version or usage fields. Missing or invalid JSON produces an error; existing Markdown remains intact. `--json` returns the same data in either language.

Usage includes failed calls and schema-repair processes. Missing provider cost stays unreported; a mixed total with missing costs is a lower bound. The loop does not enforce a combined monetary budget. Claude's budget option is a provider setting, not a total run limit. Use cycle, commit, and elapsed-time limits to bound a run.

| Exit code | Meaning |
| --- | --- |
| `0` | Completed or partially completed; inspect status and branch |
| `2` | Aborted or CLI error; inspect diagnostics |
| `4` | Safety invariant failed; worktree retained as evidence |

`.refactor/runs/<id>/` contains state, events, provider outputs, validation evidence, and reports. `last-run.json` points to the latest result. Run records store their worktree path, so older records remain usable after the cache default changes.

## Local development checks

From the repository root:

```bash
node --test tool/test/*.test.mjs
node --check tool/bin/refactor-me.mjs
node tool/bin/refactor-me.mjs version --json
```

The tool has no separate lint, typecheck, or build command. Tests use Node.js built-ins and temporary repositories. Provider decision quality and live CLI skill loading require separate provider runs; unit tests do not establish those results.
