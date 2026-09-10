# Changelog

The runtime version is defined in `src/version.mjs`. This file describes the current tool and unreleased changes. It does not publish a release.

## Unreleased

- Use `refactor-me` for the CLI, installer output, version identity, reports, commit author, and new worktree cache directories.
- Route all eight skills to `soom-kang/sharpen-me`, including phase prompts and doctor checks.
- Keep `.refactor` configuration and run records, stored worktree paths, and the `refactor/auto-` branch prefix.
- Generate English reports by default. Support `--lang ko` for runs and saved-report viewing without translation calls or changes to JSON fields.
- Check project skill availability in the base commit. Uncommitted project skills block execution. Claude uses project skill paths; Codex also checks its supported home path.
- Provide English documentation and Korean translations with current installation and validation instructions.

## 0.1.0

Current version identifier in the source. The tool audits candidates, validates one change at a time in a detached worktree, requests an independent review, and publishes accepted commits to a local branch.

Reports record the tool version, validation results, provider usage, skipped candidates, and worktree location. Validation discovery covers JavaScript, Go, Python, Rust, JVM builds, and Makefile targets where supported commands can be identified.

Existing configuration is preserved on reinstall. Review defaults and local settings when updating; a `0.x` version does not promise compatibility.
