![refactor-me](docs/assets/refactor-me-title.png)

# refactor-me

[![Verify](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml/badge.svg)](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Run behavior-preserving refactors with Codex and Claude Code. The CLI edits and validates in a separate worktree and saves results on a local `refactor/auto-*` branch. Review the diff and report before merging.

**Public Beta:** `0.10.0-beta.1` for macOS Apple Silicon, distributed through the dedicated Homebrew tap. Development builds report `dev`.

[한국어](docs/README.ko.md) · [Guide](tool/TUTORIAL.md) · [Reference](tool/README.md) · [Workflow](tool/WORKFLOW.md) · [Changelog](tool/CHANGELOG.md)

<a id="quick-start"></a>

## Install

Prepare macOS Apple Silicon, Homebrew, Git, an authenticated `codex` or `claude` CLI, and the target project's validation tools. The Homebrew formula builds from pinned source and installs Go as a build dependency. refactor-me has no Go or Node runtime dependency. The external Skill installer below requires Node.js.

```sh
brew install soom-kang/refactor-me/refactor-me
refactor-me version --json
npx skills add \
  https://github.com/soom-kang/sharpen-me/tree/v0.9.0-beta.2 \
  --global --skill '*' --agent codex claude-code
```

The eight required Skills use `~/.agents/skills` as their shared source. No Skill commit or executable copy is required in a target project. `doctor` reports missing files and conflicting same-name installations; it never removes them. A provider alias resolving to the same canonical Skill is permitted.

For development, build from a source checkout with Go 1.27:

```sh
git clone https://github.com/soom-kang/refactor-me.git
cd refactor-me/tool/go
go build -o /private/tmp/refactor-me ./cmd/refactor-me
/private/tmp/refactor-me version --json
/private/tmp/refactor-me doctor --repo /path/to/target-repo --no-live-probe
```

When using the development build, replace `refactor-me` in subsequent commands with `/private/tmp/refactor-me`.

This Beta does not claim Apple signing or notarization. For downloaded binaries, checksum verification detects changed bytes but does not authenticate the publisher. Follow [Apple's instructions for opening an individual app](https://support.apple.com/en-gb/102445) if macOS blocks a download; do not disable system-wide protections.

## Run

Use a clean target checkout. Set limits using the [guide](tool/TUTORIAL.md#set-limits-and-run) before your first run:

```sh
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo --no-live-probe
refactor-me run --repo /path/to/target-repo --provider codex --fallback none
```

For Claude only, use `--provider claude --fallback none` with both `doctor` and `run`. With neither flag, the default is Codex with Claude fallback.

`init` is optional: it creates configuration without overwriting existing settings. `run` also works with built-in defaults. Running `refactor-me` without arguments displays help. `doctor` without `--no-live-probe` calls models and consumes account usage; the disk-only check does not establish that a live session can load Skills.

From inside a target repository, omit `--repo`:

```sh
refactor-me run --target app/web
refactor-me run --target app/web --target app/api --fallback claude
```

With `--repo`, relative targets start at the selected Git root. Without it, they start at your current directory. `--target` limits candidate discovery; caller checks, edits and validation can extend beyond that directory within the repository. See [scope and validation rules](tool/README.md#candidate-selection).

The CLI creates local commits and a result branch. It does not merge, push, or deploy. Keep the source checkout unchanged during execution.

## Reports

```sh
refactor-me report --repo /path/to/target-repo
refactor-me report --repo /path/to/target-repo --lang ko
refactor-me report --repo /path/to/target-repo --json
```

Reports include file statistics, a diff preview and `changes.patch`. Viewing a report does not call a model or overwrite files. Use `run --lang ko` to write a new Korean report. Current configuration uses schema 2 and reports use schema 3; unsupported older formats produce an error and are not migrated or deleted. See [report formats and limits](tool/README.md#reports-and-language).

## Update and remove

```sh
brew upgrade soom-kang/refactor-me/refactor-me
brew uninstall refactor-me
```

An upgrade changes the CLI used by all projects. Removal leaves project configuration, runs, result branches, worktrees and global Skills in place. Project-local `install` and `uninstall` commands are no longer available. Existing local binaries are not removed automatically; use `command -v refactor-me` and `version --json` to confirm which executable you invoke.

## Verify

From this repository root:

```sh
cd tool/go
go test -race -count=1 ./...
go vet ./...
go build -o /private/tmp/refactor-me ./cmd/refactor-me
/private/tmp/refactor-me version --json
```

The [fixture guide](tool/fixtures/README.md) covers Go examples and optional JavaScript validation. Default development checks do not need Node. Live provider loading and model decisions require separate runs. See [validation boundaries](tool/README.md#local-development-checks).


For maintainers, the [Homebrew release guide](tool/release/HOMEBREW.md) covers source archives, formula generation and publication gates.

## License

The CLI and documentation use the [MIT License](LICENSE), copyright 2026 soom-kang. Required sharpen-me Skills retain their MIT license files. Codex and Claude Code are external prerequisites governed by their providers' terms.
