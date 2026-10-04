![refactor-me](docs/assets/refactor-me-title.png)

# refactor-me

[![Release](https://img.shields.io/badge/Release-0.10.0--beta.4-2f6f5e)](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.4) [![Verify](https://img.shields.io/github/actions/workflow/status/soom-kang/refactor-me/verify.yml?branch=main&label=Verify)](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml) [![MIT License](https://img.shields.io/badge/License-MIT-555555)](LICENSE)

Automate behavior-preserving refactoring with Codex or Claude Code. The CLI works in an isolated Git worktree, validates each candidate, and saves accepted commits on a local branch for your review.

[한국어](docs/README.ko.md) · [Usage guide](tool/TUTORIAL.md) · [Reference](tool/README.md) · [Workflow](tool/WORKFLOW.md)

## Install

Public Beta **0.10.0-beta.4** supports **macOS Apple Silicon**. You need Homebrew, Git, an authenticated Codex or Claude Code CLI, and your target project's build and test tools.

Release commit: `0032e594eaab3240d4dee1aa133be5b9d6eb3c42`. See the [public installation check](https://github.com/soom-kang/refactor-me/actions/runs/37208178549) for the published tag and Homebrew formula. The Verify badge follows the CLI workflow on `main`.

<a id="quick-start"></a>

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
refactor-me version --json
```

Register and trust this formula once with Homebrew 6 or later. Future updates use `brew upgrade refactor-me`. A fresh Homebrew installation needs these setup commands before the short install command.

Install sharpen-me Skills globally. This separate installer requires Node.js; running refactor-me does not.

```sh
npx skills add soom-kang/sharpen-me \
  --global --skill '*' --agent codex claude-code
```

Homebrew manages the executable. Required Skills resolve from `~/.agents/skills`. Each target repository keeps its own configuration and run records.

![One global CLI and Skill catalog serve separate Git repositories, each with its own .refactor state.](docs/assets/workflow/installation.en.png)

## First run

Confirm `refactor-me version --json` reports `0.10.0-beta.4` before following these commands. If an older version is installed, run `brew upgrade refactor-me` first.

Replace the path with a clean Git repository that has at least one commit. Prepare its dependencies first. The commands below select Codex only; a Claude Code check uses `--provider claude --fallback none`.

```sh
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo \
  --provider codex --fallback none --no-live-probe
```

`init` preserves existing configuration. Run it before setting first-run limits in the configuration file; otherwise initialization is optional. This doctor check makes no model calls. **Set [first-run limits](tool/TUTORIAL.md#set-limits-and-run) before starting:**

```sh
refactor-me run --repo /path/to/target-repo \
  --provider codex --fallback none \
  --model gpt-6.1-sol --effort xhigh
refactor-me report --repo /path/to/target-repo
```

Choose a model for every selected provider, either with command options or in project configuration. The example model and effort are choices for this run, not defaults. Claude Code uses `--provider claude --fallback none --model claude-sonnet-5-5 --effort xhigh`. These options do not change configuration files; see [model and effort selection](tool/README.md#model-and-effort-selection) for fallback examples and precedence.

`run` and doctor without `--no-live-probe` consume provider usage. Exit `0` can mean partial completion: inspect the report and diff before merging. The CLI does not merge, push or deploy results.

## Readable progress logs

Public Beta `0.10.0-beta.4` shows the current stage, candidate and confirmed result instead of shell commands. The existing `--lang en|ko` selects progress logs, the final summary and report language; English remains the default. Add `--lang ko` for Korean progress.

Progress goes to `stderr`, so `run --json` keeps report JSON alone on `stdout`. Each audit reports its own proposed and eligible candidate counts; long stages report elapsed time every 30 seconds. Original provider transcripts and validation output remain in local run records. See [progress logs](tool/README.md#progress-logs).

## Read next

| Task | Document |
| --- | --- |
| Install, choose a target, run and inspect results | [Usage guide](tool/TUTORIAL.md) |
| Look up commands, settings and exit codes | [Reference](tool/README.md) |
| Understand isolation, validation and review | [Workflow](tool/WORKFLOW.md) |
| Build, test or prepare a release | [Development](tool/DEVELOPMENT.md) · [Homebrew release](tool/release/HOMEBREW.md) |
| Check changes between versions | [Changelog](tool/CHANGELOG.md) |

This Beta has no Apple signing or notarization. See [installation details](tool/release/INSTALL.md) for standalone downloads and macOS warnings.

## License

[MIT](LICENSE), copyright 2026 soom-kang. Keep the copyright and license notice when redistributing the CLI or documentation. The software comes without warranty. sharpen-me retains its own license; Codex and Claude Code follow their providers' terms.
