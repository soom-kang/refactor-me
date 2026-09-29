# Install refactor-me `0.10.0-beta.1`

This document describes the macOS Apple Silicon public Beta distributed through the dedicated Homebrew tap.

## Homebrew

```sh
brew install soom-kang/refactor-me/refactor-me
refactor-me version --json
npx skills add \
  https://github.com/soom-kang/sharpen-me/tree/v0.9.0-beta.2 \
  --global --skill '*' --agent codex claude-code
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo --no-live-probe
```

Replace the target path with an existing Git repository. Homebrew builds from fixed source and manages Go as a build dependency. The executable does not need Go or Node at runtime. The external `npx` Skill installer requires Node.js. Git, an authenticated Codex or Claude CLI and the target's validation tools are separate prerequisites.

The eight global Skills use `~/.agents/skills` as the canonical source. `init` creates optional configuration without overwriting an existing file. No executable or Skill commit is needed in the target project. The disk-only check does not verify live provider loading; `doctor` without `--no-live-probe` calls models and consumes usage.

After [setting run limits](https://github.com/soom-kang/refactor-me/blob/v0.10.0-beta.1/tool/TUTORIAL.md#set-limits-and-run), start explicitly. The default is Codex with Claude fallback. For Claude only, add `--provider claude --fallback none` to both `doctor` and `run`:

```sh
refactor-me run --repo /path/to/target-repo
refactor-me report --repo /path/to/target-repo
```

## Standalone archive

If using a verified release archive, invoke its executable directly, for example `./refactor-me run --repo /path/to/target-repo`. It has no project installation command. Check the archive against the published `SHA256SUMS` before extracting it, then inspect `version --json` for version, architecture and commit provenance. A checksum detects changed bytes; it does not authenticate the publisher.

This Beta is unsigned and not notarized. macOS may warn or block downloaded files. Use [Apple's individual-app instructions](https://support.apple.com/en-gb/102445) if needed; do not disable system-wide security settings.

## Update and remove

```sh
brew upgrade soom-kang/refactor-me/refactor-me
brew uninstall refactor-me
```

An update changes the shared CLI for every project. Removal retains project configuration, runs, branches, worktrees and global Skills. Configuration schema 2 and report schema 3 are required. Older formats are rejected without conversion or deletion; preserve old data separately before creating new configuration. See the [full guide](https://github.com/soom-kang/refactor-me/blob/main/tool/TUTORIAL.md) for limits, diagnostics and cleanup.
