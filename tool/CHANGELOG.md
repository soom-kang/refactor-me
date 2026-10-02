# Changelog

## Unreleased

Development changes for the next release; public Beta `0.10.0-beta.2` and its Homebrew formula are unchanged.

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
