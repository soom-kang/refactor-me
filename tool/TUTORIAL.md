# Run your first refactoring

[Project](../README.md) · [한국어](TUTORIAL.ko.md) · [Reference](README.md)

Follow these five steps on macOS Apple Silicon. Replace `/path/to/target-repo` with your repository path. Quote paths that contain spaces.

<a id="prepare-the-target"></a>

## 1. Prepare the repository

Use a Git repository with at least one commit. Finish or set aside changes, including untracked files, then prepare the project's dependencies and run its checks once.

```sh
cd /path/to/target-repo
git rev-parse --show-toplevel
git status --short
```

`git status --short` should print nothing. Keep the source checkout unchanged while refactor-me runs.

Install and authenticate one provider CLI using its own instructions. Check the provider you will use:

```sh
codex login status
# For Claude Code instead:
claude auth status
```

<a id="install-the-tool-and-skills"></a>

## 2. Install the CLI and Skills

You need Homebrew and Node.js for the separate Skill installer. Homebrew supplies Go to build the CLI; the installed CLI needs neither Go nor Node at runtime.

```sh
brew install soom-kang/refactor-me/refactor-me
refactor-me version --json
npx skills add soom-kang/sharpen-me \
  --global --skill '*' --agent codex claude-code
```

Expect CLI version `0.10.0-beta.1`, platform `darwin` and architecture `arm64`. All eight required Skills must resolve from `~/.agents/skills`. Avoid separate copies of the same required Skill in the target project; doctor reports conflicting paths.

Create configuration and check Codex without calling a model:

```sh
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo \
  --provider codex --fallback none --no-live-probe
```

For Claude Code, use `--provider claude --fallback none` in both doctor and run. Resolve blocking `FAIL` checks before continuing. `init` never overwrites an existing configuration. Run it for this guide so the next step has a file to edit. Without initialization, the CLI uses built-in defaults. Running `refactor-me` alone displays help.

`--no-live-probe` skips model calls but still checks provider executables and prepares temporary diagnostics. To check live session Skill visibility, repeat doctor without that flag; this consumes provider usage.

<a id="set-limits-and-run"></a>

## 3. Set first-run limits

Open `.refactor/config.json` in the target repository. Change these fields inside the existing objects; this is a partial example, not a replacement file:

```json
{
  "agents": {
    "codex": {"timeout_sec": 300},
    "claude": {"timeout_sec": 300}
  },
  "policy": {
    "max_cycles": 1,
    "max_commits": 1,
    "max_wall_clock_min": 20
  }
}
```

These values limit a first trial to one cycle and one refactor commit. Characterization test commits count separately. Provider timeouts apply per call; the elapsed-time limit is checked between work units and is not a hard deadline.

**There is no total monetary cap.** Model calls, retries and live doctor checks consume account usage. See [all settings](README.md#configuration) before increasing limits.

## 4. Start the run

Run Codex without fallback from any directory:

```sh
refactor-me run --repo /path/to/target-repo \
  --provider codex --fallback none
```

For Claude Code, replace `codex` with `claude`. To allow Claude fallback, use `--provider codex --fallback claude`; this is also the default when provider flags are omitted. Add `--lang ko` for a Korean report and final summary.

To search for candidates in selected directories:

```sh
refactor-me run --repo /path/to/target-repo \
  --target app/web --target app/api \
  --provider codex --fallback none
```

Choose existing directories with tracked source files. Omit `--target` to survey the whole repository.

| Path | Resolution |
| --- | --- |
| Relative `--repo` | From the calling directory, then locate its Git root |
| `--target` with `--repo` | From the selected Git root |
| `--target` without `--repo` | From the calling directory |

Paths resolving outside the repository are rejected. **Targets limit candidate discovery, not every edit or validation command.** Caller changes may reach elsewhere inside the repository.

![Check prerequisites, run in an isolated worktree, inspect the saved outcome, then review published changes before merging.](../docs/assets/workflow/execution.en.png)

<a id="read-the-result"></a>

## 5. Inspect the result

Read the report even when the command exits successfully:

```sh
refactor-me report --repo /path/to/target-repo
refactor-me report --repo /path/to/target-repo --lang ko
refactor-me report --repo /path/to/target-repo --json
```

Report viewing makes no model calls. Language selection does not rewrite the saved report; `--json` prints the supported saved JSON bytes.

1. Check the terminal status and stop reason. Exit `0` includes partial completion.
2. Check validation, skipped checks and provider usage. Missing cost is unknown, not zero.
3. Inspect the local result branch and `.refactor/runs/<id>/changes.patch`. No published changes means there may be no patch or result branch.
4. Review the diff and run any omitted integration or browser checks before merging yourself.

Results use local `refactor/auto-*` branches. The CLI does not merge, push or deploy. `accepted.patch` is intermediate review input; use `changes.patch` for the final published comparison.

## When a run stops

| Signal | Next action |
| --- | --- |
| Missing, conflicting or changed Skill | Inspect the paths in doctor; repair the global installation between runs |
| Dirty checkout or invalid target | Finish existing work or correct the path, then rerun doctor |
| Baseline failure | Inspect dependencies and [validation commands](README.md#validation-commands); at least one command must pass |
| No changes or partial completion | Read exclusions and limits before starting another run |
| Exit `4`, safety halt | Preserve the worktree and diagnostics; investigate before retrying |

Exit `2` means an abort or CLI error. Check stderr and the available run records. Configuration schema `2` and report schema `3` are required; unsupported formats produce an error without changing the file. A missing or invalid report is not proof that a run succeeded.

Records can contain source excerpts and command output. Review them before sharing. See [Workflow](WORKFLOW.md) for rollback and publication checks.

## Update or remove

Upgrade the CLI, then repeat the non-live doctor check:

```sh
brew upgrade soom-kang/refactor-me/refactor-me
refactor-me version --json
refactor-me doctor --repo /path/to/target-repo \
  --provider codex --fallback none --no-live-probe
```

An upgrade affects every project using this executable. Update Skills separately with the installation command in step 2, after checking local changes. Do not update Skills during a run.

Remove eligible finished worktrees with `refactor-me clean --repo /path/to/target-repo`. Partial and safety-halted worktrees remain available for investigation.

```sh
brew uninstall refactor-me
```

Uninstalling keeps project settings, reports, worktrees, result branches and global Skills. This Beta is unsigned and not notarized; see [standalone installation and macOS warnings](release/INSTALL.md).
