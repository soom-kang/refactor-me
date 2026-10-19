# Changelog

## Unreleased

- Pass measured post-edit validation evidence to review and reuse existing lint/typecheck results. Rerun only affected checks after relevant code/environment changes or new failure evidence; preserve failed and missing statuses, reset candidate evidence, and keep implementer rationale out of the independent review.

## 0.10.0-beta.9

[GitHub prerelease](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.9) · [Fresh installation](https://github.com/soom-kang/refactor-me/actions/runs/39198427620) · [Beta.8 upgrade](https://github.com/soom-kang/refactor-me/actions/runs/39198430208). Release commit: `b87bde4ca5d56dd2cdc4ee2e5a8453d814de40d0`. [Verification record](release/HOMEBREW.md#beta9-verification-record).

- Share a minimal-test policy across audit, deep check and later candidate phases. Proven pure deletions need no new tests; other edits require a concrete affected behavior and evidence gap before adding focused characterization. Reuse existing test infrastructure and keep unresolved reachability or side effects blocking.

## 0.10.0-beta.8

[GitHub prerelease](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.8) · [Fresh installation](https://github.com/soom-kang/refactor-me/actions/runs/38988891975) · [Beta.7 upgrade](https://github.com/soom-kang/refactor-me/actions/runs/38988894754). Release commit: `e1ccf3bde29f063ec0334a3257870b61f8f16170`. [Verification record](release/HOMEBREW.md#beta8-verification-record).

- Add dated standard API estimates for Codex `gpt-6-astra` and correct Claude Sonnet 5.5 cache-read pricing to USD 0.10 per million tokens. Recheck all bundled rates on 2026-10-17; clarify that unknown models are absent from the bundled table, not necessarily from Artificial Analysis. Preserve provider-reported prices and historical reports without retroactive repricing.
- Keep the full committed diff in `changes.md` and show its link with file statistics in the report. Omit the duplicate inline preview and patch link when Markdown was saved; retain the legacy and Markdown-write-failure fallback and raw patch evidence.
- Use the active Git author and committer identity for refactor and characterization commits instead of a fixed `refactor-me` account. Preserve Git configuration and explicit environment overrides; stop without guessing an identity when required configuration is missing. Branch-conditioned includes are evaluated in the detached run worktree.
- Add regression coverage for Astra cache accounting, missing/invalid usage and timeout subtotals, bilingual Markdown report selection/fallback, and Git identity precedence and missing-identity preservation.

## 0.10.0-beta.7

[GitHub prerelease](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.7) · [Fresh installation](https://github.com/soom-kang/refactor-me/actions/runs/38195197188) · [Beta.6 upgrade](https://github.com/soom-kang/refactor-me/actions/runs/38195202547). Release commit: `daec4844affaca08d1e42dd394be12636fa84312`.

[QA remediation](../docs/qa/2026-10-12-remediation.md) · [12 ticket resolutions](../docs/qa/2026-10-12-tickets.md) · [Follow-up verification](../docs/qa/2026-10-12-live-verification.md). Includes all 12 QA fixes. The prior source checks cover bounded live doctor probes for Codex and Claude, isolated candidate installation/upgrade and govulncheck; [release verification](release/HOMEBREW.md#beta7-verification-record) records tagged artifacts and public-install evidence separately.

- Reject malformed locked validation input and zero-check candidate success. Normalize omitted areas/cwd, confine command working directories to the worktree, and validate known configuration types and numeric bounds while retaining defaults, extension keys and zero semantics (RM-QA-001, 002, 004).
- Skip entire hydrated build inputs with unsafe links, preserve existing destinations, and recheck source HEAD/fingerprint before approved commit and result publication (RM-QA-003, 005).
- Preserve per-invocation retry/repair transcripts, exit status and usage. Reject nonzero or missing Claude process exits despite successful payloads (RM-QA-006, 008).
- Preserve unfinished edits on validation cancellation, record explicit cancellation evidence/progress, and enforce `L3_CRITICAL` exclusion at audit and packet gates regardless of custom configuration (RM-QA-007, 012).
- Sanitize human report fields without rewriting raw evidence, truncate at valid UTF-8 boundaries, and require exact annotated release tags matching HEAD before public source/binary artifacts are written (RM-QA-009, 010, 011).

## 0.10.0-beta.6

[GitHub prerelease](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.6) · [Fresh installation](https://github.com/soom-kang/refactor-me/actions/runs/37625488585) · [Beta.5 upgrade](https://github.com/soom-kang/refactor-me/actions/runs/37625494290). Release commit: `01b407ce81e9803878c003c181860e499718b0fa`.

- Record controller preparation failures as the latest failed attempt when failure-marker storage succeeds, preserving earlier completed reports. Pre-controller CLI/configuration/input errors do not update that marker. Explain at run startup that the selected time limit is soft and is not a spending cap.
- Propagate Ctrl-C and SIGTERM through run and doctor and stop retries and fallback after cancellation. On timeout or cancellation, terminate provider descendants even if their leader exits first. Preserve accepted commits and unfinished worktrees for inspection.
- Confine declared deletions to the worktree and check the actual characterization diff before creating its commit, recording the gate and patch evidence.
- Build public standalone binaries from isolated tagged Git objects and ignore ambient Go workspace, settings and overlay flags. Keep archive version, commit, architecture and dirty-state checks.
- Embed a reproducible sharpen-me content reference and report custom or changed catalogs without claiming live compatibility. Add optional Beta-to-Beta Homebrew upgrade checks for config/report byte preservation and non-destructive rejection of unsupported reports.

## 0.10.0-beta.5

[GitHub prerelease](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.5) · [Public installation check](https://github.com/soom-kang/refactor-me/actions/runs/37573464996). Release commit: `8bfebe90b04e54cf1ef843915aad2901be0db41d`.

- Add run-only `--max-minutes` and a macOS terminal time selector with 30/60/180/360/custom choices, configured-default Enter, safe cancellation and no prompt for JSON or redirected input/output. Keep overrides in memory and report selected/actual time; stop before new work at safe boundaries while an in-flight candidate finishes.
- Add default elapsed-time and stage/provider context with structured, safe read/search/edit/write/delete and generic command/tool activity. Preserve stderr-only progress, raw transcript storage, tool-call accounting and suppression of commands, output, prose and sensitive paths.
- Save `changes.md` with file review checkboxes and the full committed text diff, including rename, binary and mode metadata. Keep `changes.patch` unchanged and preserve reviewed checkboxes when viewing reports.
- Keep provider-reported costs first and supplement missing prices with offline, dated Artificial Analysis standard API estimates for exact supported models. Record token categories, source URLs and per-call assumptions; distinguish reported, estimated and unknown amounts without treating estimates as subscription charges. Preserve report schema 3 and released Beta provenance.

## 0.10.0-beta.4

[GitHub prerelease](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.4) · [Public installation check](https://github.com/soom-kang/refactor-me/actions/runs/37208178549). Release commit: `0032e594eaab3240d4dee1aa133be5b9d6eb3c42`.

- Pass measured baseline checks and completed characterization evidence between candidate phases so execution can verify prerequisites without rediscovering run logs or rejecting tests just prepared by the controller.
- Save characterization validation results, preserve failed and unavailable check statuses, and include the implementer's rationale in execution rejection diagnostics.
- Clarify phase ownership of stop conditions and allow one correction of incidental whitespace from an editing tool within the same allowed production file. Keep scope, behavior, validation and run-limit gates unchanged.
- Reduce required release checks to formatting, Go tests, vet, CLI build and packaging tests on main, archive integrity checks during packaging, and one public Homebrew installation check. Remove duplicate tag CI and make race, lint, vulnerability, formula audit and live-provider checks optional for relevant changes.

## 0.10.0-beta.3

[GitHub prerelease](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.3) · [Public installation check](https://github.com/soom-kang/refactor-me/actions/runs/36971167653). Release commit: `01a3fad54149cb127e8bd6c5d638eda0562a5897`.

- Replace raw terminal tool commands with stage and candidate progress messages. Extend the existing `--lang en|ko` to run progress while keeping English as the default and standalone doctor output in English.
- Report each audit's proposed, inspected when capped, and eligible counts. Distinguish measured baseline results, unchanged baseline failures, rejection, no changes, regression and confirmed rollback or local commit publication.
- Add 30-second current-stage elapsed-time updates, serialized stderr output and bounded, sanitized dynamic display values. Preserve report JSON on stdout with `run --json` and original transcripts and tool-call accounting in local records.
- Explain provider repair, permission denial, retries and switching with fixed messages, and point failures to saved diagnostic records when available. Update English and Korean user documentation and development checks without changing configuration or report schemas.

## 0.10.0-beta.2

[GitHub prerelease](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.2) · [Public installation check](https://github.com/soom-kang/refactor-me/actions/runs/36956289983). Release commit: `c30dbdadc4229958e29366fdc14d79df3da8cb02`.

- Consolidate English and Korean installation, first-run, reference and workflow documentation.
- Use the unpinned global sharpen-me installation command and add project-state and execution diagrams.
- Remove retired migration documents and historical validation reports; retain version history here.
- Add bilingual Homebrew guidance, title artwork and license explanations.

- Add `--model`, `--fallback-model`, `--effort` and `--fallback-effort` to `run` and `doctor`. Resolve each role against the selected provider order and keep overrides in memory without rewriting project configuration.
- Require an explicit effective model for every selected provider before `run` or live doctor starts. Offline doctor and non-model commands still work without configured models; existing phase effort defaults remain unchanged.
- Document one-time Homebrew tap and formula trust, followed by short-name `brew install refactor-me` and `brew upgrade refactor-me` commands.
- Add optional public-install verification to the existing Verify workflow. The requested check uses a fresh Apple Silicon runner, runs formula style/audit checks, and compares the public installation's version and commit with the selected checkout; it does not call provider models or publish releases.

## 0.10.0-beta.1

macOS Apple Silicon Beta with a source-build Homebrew formula and global sharpen-me Skills.

- Install the CLI globally with Homebrew; add `--repo` to select a target Git repository from any directory. No arguments now show help; use explicit `run` to start refactoring.
- Add optional, non-overwriting `init`. Remove project-local `install` and `uninstall`, legacy Node ownership data and migration/rollback functionality.
- Require the eight global sharpen-me Skills from `~/.agents/skills`. Record content hashes and delivery paths, reject conflicting sources, and stop publication on unexpected Skill changes. Give Claude a dedicated Skill copy while retaining project-only settings.
- Require configuration schema 2 and saved report schema 3. Reject older formats without modifying existing records.
- Remove the Node implementation and its test runner. Keep Go fixtures and optional JavaScript examples; fixture generation no longer copies Skills. Preserve JavaScript/TypeScript target support.
- Update English and Korean installation, execution, configuration and release documentation. Homebrew removal leaves project data, result branches, worktrees and global Skills in place.

## 0.9.20-beta.1

First Go CLI public Beta for macOS Apple Silicon. Source builds without release version injection report `dev`.

- Implement `run`, `doctor`, `report`, `clean`, `help`, `version`, `install`, and `uninstall` in Go. Keep the Node CLI available for comparison and recovery.
- Install one Go executable in each target repository while preserving existing configuration and run records. Read older Node reports without rewriting their JSON.
- Add a `darwin/arm64` release archive with `LICENSE`, `INSTALL.md`, and build information, plus a separate `SHA256SUMS` file. The first Beta is unsigned and not notarized.
- Use the verified `v0.8.8-beta.1` Node tag for collision-checked rollback. Do not change users' saved runs or existing Git tags.

## Earlier Beta changes — 2026-09-17

- Add `policy.unknown_risk`. The default `set_aside` keeps the existing behavior: a candidate whose risk level is UNKNOWN is recorded for a human and never executed. Set it to `deep_check` to send the one resolvable shape — UNKNOWN risk together with `NEEDS_EVIDENCE` readiness — to the deep-check phase, whose job is closing exactly that evidence gap. Every other UNKNOWN is still set aside, and `allowed_risks` still cannot enable UNKNOWN. An unrecognized value stops the run before it starts.
- Check the task packet's own `risk_level` after deep check, for every candidate. A deep check that returns UNKNOWN, or raises the risk above `allowed_risks`, now stops the candidate instead of proceeding on the audit's earlier estimate.
- Stop treating a candidate filtered during ranking as a path a previous attempt violated. Those paths were reported to the next audit as forbidden ground, which could leave a re-audit believing it had nothing it was allowed to propose.
- Report why an audit produced no work: whether the audit proposed nothing, or every proposal was filtered by policy. The distinction appears in the run log with a per-reason count, in `report.json` counters, in `report.md`, and in the stop reason, in both languages.
- Write each cycle's audit prompt and transcript under `audits/<cycle>/`. Re-audits previously overwrote the first cycle's artifacts. `audits/<cycle>.json` is unchanged.
- Stop telling a re-audit that earlier cycles landed commits when none did.
- Record the audit's own stated objection on an UNKNOWN-risk candidate instead of a fixed sentence naming a skill.

## Public Beta changes — 2026-09-11

Public Beta of the refactor-me CLI. The version is the repository's selected release identifier, not a count of prior CLI releases. An earlier Skill-package release tag was superseded by this CLI release; sharpen-me remains a separate required dependency.

- Apply the MIT license to the CLI and documentation and include it in installed runtime copies.
- Publish tagged installation instructions, Beta status, and a local verification workflow for Codex and Claude Code integration contracts.

- Add a committed-code comparison to reports with file statistics, a bounded diff preview and `changes.patch`. Preserve report viewing for older JSON.
- Add English and Korean Workflow documents with schema-checked response examples and failure branches. Clarify candidate validation scope and provider switching.

- Use `refactor-me` for the CLI, installer output, version identity, reports, commit author, and new worktree cache directories.
- Route all eight skills to `soom-kang/sharpen-me`, including phase prompts and doctor checks.
- Keep `.refactor` configuration and run records, stored worktree paths, and the `refactor/auto-` branch prefix.
- Generate English reports by default. Support `--lang ko` for runs and saved-report viewing without translation calls or changes to JSON fields.
- Check project skill availability in the base commit. Uncommitted project skills block execution. Claude uses project skill paths; Codex also checks its supported home path.
- Measure base-commit skill availability in a real worktree checkout instead of reading the git index. The index cannot report a path through a symlink, so the standard `.claude/skills/<name>` link into `.agents/skills/<name>` was reported as uncommitted and blocked every Claude run against a fully committed catalog.
- Run that check for every available provider rather than one. Skill paths differ per provider, so a catalog committed for Codex but not for Claude now blocks instead of silently running Claude phases without skills.
- Provide English documentation and Korean translations with current installation and validation instructions.

## Earlier internal development

Previous internal version identifier; no GitHub CLI release was published under this version. The tool audits candidates, validates one change at a time in a detached worktree, requests an independent review, and publishes accepted commits to a local branch.

Reports record the tool version, validation results, provider usage, skipped candidates, and worktree location. Validation discovery covers JavaScript, Go, Python, Rust, JVM builds, and Makefile targets where supported commands can be identified.

Existing configuration is preserved on reinstall. Review defaults and local settings when updating; a pre-stable version does not promise compatibility.
