# Changelog

The runtime version is defined in `src/version.mjs`. Release tags use `v<version>`.

## 0.8.8-beta.1 — 2026-09-11

Public Beta of the refactor-me CLI. The version is the repository's selected release identifier, not a count of prior CLI releases. The incorrect `v0.8.10-beta.1` Skill-package release is replaced by this CLI release; sharpen-me remains a separate required dependency.

- Apply the MIT license to the CLI and documentation and include it in installed runtime copies.
- Publish tagged installation instructions, Beta status, and a local verification workflow for Codex and Claude Code integration contracts.

- Add a committed-code comparison to reports with file statistics, a bounded diff preview and `changes.patch`. Preserve report viewing for older JSON.
- Add English and Korean Workflow documents with schema-checked response examples and failure branches. Clarify candidate validation scope and provider switching.

- Use `refactor-me` for the CLI, installer output, version identity, reports, commit author, and new worktree cache directories.
- Route all eight skills to `soom-kang/sharpen-me`, including phase prompts and doctor checks.
- Keep `.refactor` configuration and run records, stored worktree paths, and the `refactor/auto-` branch prefix.
- Generate English reports by default. Support `--lang ko` for runs and saved-report viewing without translation calls or changes to JSON fields.
- Check project skill availability in the base commit. Uncommitted project skills block execution. Claude uses project skill paths; Codex also checks its supported home path.
- Provide English documentation and Korean translations with current installation and validation instructions.

## 0.1.0

Previous internal version identifier; no GitHub CLI release was published under this version. The tool audits candidates, validates one change at a time in a detached worktree, requests an independent review, and publishes accepted commits to a local branch.

Reports record the tool version, validation results, provider usage, skipped candidates, and worktree location. Validation discovery covers JavaScript, Go, Python, Rust, JVM builds, and Makefile targets where supported commands can be identified.

Existing configuration is preserved on reinstall. Review defaults and local settings when updating; a `0.x` version does not promise compatibility.
