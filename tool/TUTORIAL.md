# refactor-me guide

[Project](../README.md) · [한국어](TUTORIAL.ko.md) · [Reference](README.md)

This walkthrough installs the tool, runs it on a target repository, and reviews the resulting branch. Run commands from the directories shown and substitute your own paths.

## Prepare the target

Use macOS and Node.js 24 or later. From the repository you want to refactor:

```bash
cd /path/to/target-repo
node --version
git rev-parse --show-toplevel
git status --short
```

Complete or set aside your existing changes before running. The source checkout must be clean, including untracked files. Install the project's dependencies and run its normal build and tests once to prepare caches and understand baseline failures.

Check the provider you intend to use:

```bash
codex login status
# If using Claude Code:
claude auth status
```

Provider calls require network access and consume the provider account's available usage.

## Install the tool and skills

In your refactor-me checkout:

```bash
node tool/install.mjs /path/to/target-repo
```

Then install the catalog in the target repository:

```bash
cd /path/to/target-repo
npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code
```

Use Project scope. Review the installed files and commit the eight Skill directories, the corresponding agent links, and `skills-lock.json` through your normal Git workflow. These files must reach the base commit used by the worktree. Avoid staging unrelated files.

Check the installed tool:

```bash
./.refactor/bin/refactor-me version
./.refactor/bin/refactor-me help
./.refactor/bin/refactor-me doctor
```

Doctor writes diagnostics under `.refactor/runs/` and probes the providers. Resolve blocking failures before running. A disk-only check is available as `doctor --no-live-probe`, but it does not confirm that a model session can load the skills.

## Set limits and run

Review `.refactor/config.json`. For a first run, set `policy.max_commits` to `1` and choose a suitable elapsed-time limit. Characterization commits are recorded separately from the refactor-commit counter.

With Codex only:

```bash
./.refactor/bin/refactor-me --provider codex --fallback none
```

With Claude available as a fallback:

```bash
./.refactor/bin/refactor-me --provider codex --fallback claude
```

To find candidates in one directory:

```bash
./.refactor/bin/refactor-me --target app/web
```

The target is relative to your current directory. Caller changes and validation can extend beyond it. Do not edit the source checkout while the run is active.

The controller may create local commits and publish them to a `refactor/auto-*` branch. Stop conditions include no remaining eligible candidates, configured limits, repeated failures, unavailable providers, and safety violations. Exit code `0` can mean partial completion; read the reported status.

## Read the result

```bash
./.refactor/bin/refactor-me report
./.refactor/bin/refactor-me report --lang ko
./.refactor/bin/refactor-me report --json
```

To write the report and final summary in Korean when running:

```bash
./.refactor/bin/refactor-me --provider codex --fallback none --lang ko
```

Each run saves one `report.md`. Viewing another language renders `report.json` without replacing the saved Markdown. Fixed labels and known reasons are translated; model explanations and errors stay in their original language. `--lang` applies only to `run` and `report`. Doctor and progress logs remain in English.

The report lists committed changes, skipped candidates, validation evidence, provider usage, and the worktree path. Missing cost is not zero cost. A partial cost total is a lower bound.

Use the exact base commit and result branch shown in the report:

```bash
git log --oneline <base-commit>..<result-branch>
git diff <base-commit>...<result-branch>
```

Check the diff and run any omitted service, browser, or integration checks before deciding whether to merge. Baseline failures that remained unchanged are not passing tests. `handoff.md` is an intermediate snapshot; use the final report for the outcome.

## Handle a stopped run

| Symptom | Next action |
| --- | --- |
| Missing skill | Install the named Skill in Project scope and check the agent link |
| `skill-worktree` failure | Review and commit the project Skill files before starting again |
| Dirty source checkout | Finish or set aside your work, then rerun doctor |
| No usable baseline | Inspect command failures, dependencies, and build caches; define commands if discovery is insufficient |
| No eligible candidates | Read the exclusion reasons; a run can finish without changes |
| Partial completion | Inspect accepted commits and the stop reason before starting another run |
| Safety halt, exit `4` | Keep the worktree and diagnostics; investigate the violated invariant |
| Report JSON missing or invalid | Restore the recorded JSON if available; existing Markdown is not overwritten |

For explicit validation commands, see [Validation commands](README.md#validation-commands). Run records may contain source excerpts and command output; review them before sharing.

## Update and remove

From an updated refactor-me checkout, reinstall with the same command:

```bash
node tool/install.mjs /path/to/target-repo
```

The installer replaces its runtime files and preserves `.refactor/config.json`, run records, and `last-run.json`. It removes a recognized previous generated command when replacing it with the current command. If an old command file has been customized, installation stops before replacing the runtime; inspect that file first. No compatibility alias is installed.

Review local Skill edits before updating the catalog in the target repository. Use the [sharpen-me documentation](https://github.com/soom-kang/sharpen-me) for catalog maintenance, then commit the reviewed update.

To remove finished worktrees, run from the target repository:

```bash
./.refactor/bin/refactor-me clean
```

The command retains partial, unfinished, and safety-halted worktrees. To uninstall the tool, run from its source checkout:

```bash
node tool/install.mjs /path/to/target-repo --uninstall
```

Uninstall removes `.refactor/lib` and `.refactor/bin`. It keeps configuration and run records, and does not remove the Skill catalog, result branches, or worktrees. Keep custom commands outside the tool's installation directories.
