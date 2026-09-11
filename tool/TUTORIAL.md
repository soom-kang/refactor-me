# refactor-me guide

[Project](../README.md) · [한국어](TUTORIAL.ko.md) · [Reference](README.md) · [Workflow](WORKFLOW.md)

Prepare the target repository, then follow these five steps. Check each command’s working directory and replace the example paths.

<a id="prepare-the-target"></a>

## 1. Prepare the target

Use macOS and Node.js 24 or later. From the repository you want to refactor:

```bash
cd /path/to/target-repo
node --version
git rev-parse --show-toplevel
git status --short
```

Finish or set aside existing changes. The source checkout must be clean, including untracked files.

Install project dependencies and run its build and tests once to prepare caches and identify baseline failures.

Check the provider you intend to use:

```bash
codex login status
# If using Claude Code:
claude auth status
```

Provider calls require network access and consume the provider account's available usage.

<a id="install-the-tool-and-skills"></a>

## 2. Install the tool and skills

Clone using the [Beta installation instructions](../README.md#install), then run from that checkout:

```bash
node tool/install.mjs /path/to/target-repo
```

Install the eight required sharpen-me Skills in the target repository:

```bash
cd /path/to/target-repo
npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code
```

Choose Project scope. Review and commit the eight Skill directories, agent links, and `skills-lock.json`. Worktrees read files from the base commit. Keep unrelated files out of the commit.

Check the installed tool:

```bash
./.refactor/bin/refactor-me version
./.refactor/bin/refactor-me help
./.refactor/bin/refactor-me doctor
```

Doctor writes diagnostics under `.refactor/runs/` and calls models. Resolve blocking failures. `doctor --no-live-probe` checks disk installation without verifying session Skill loading.

<a id="set-limits-and-run"></a>

## 3. Set limits

In `.refactor/config.json`, set `policy.max_commits` to `1` for a first run and choose an elapsed-time limit. Characterization test commits are counted apart from this refactor-commit limit.

**The loop does not enforce a total monetary budget.** Claude’s budget option is not a total run limit either. See [configuration and defaults](README.md#configuration).

## 4. Run

From the target repository, use Codex only:

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

`--target` is relative to your current directory. Caller changes and validation can extend beyond it. Keep the source checkout unchanged during a run.

Accepted changes remain on a local `refactor/auto-*` branch. The loop stops for no eligible candidates, run limits, repeated failures, unavailable providers, or safety violations.

**Exit code `0` can mean partial completion.** Check the status in the next step.

<a id="read-the-result"></a>

## 5. Read the result

```bash
./.refactor/bin/refactor-me report
./.refactor/bin/refactor-me report --lang ko
./.refactor/bin/refactor-me report --json
```

To write the report and final summary in Korean when running:

```bash
./.refactor/bin/refactor-me --provider codex --fallback none --lang ko
```

Viewing another language preserves the saved `report.md`. `--lang` applies to `run` and `report`; doctor and progress logs remain in English. See [reports and language](README.md#reports-and-language) for translated fields and JSON compatibility.

Check committed changes, skipped candidates, validation, usage, and the worktree path. Missing cost is not zero; a partial total is a lower bound.

Read the file statistics and diff preview, then open `.refactor/runs/<id>/changes.patch` for the full comparison. It uses fixed start and final published commit OIDs. A comparison failure is separate from the run result; inspect its reason.

Use the full start and final published OIDs from `codeComparison` for another Git view:

```bash
git log --oneline <base-commit>..<published-commit>
git diff <base-commit> <published-commit>
```

Before merging, review the diff and run omitted service, browser, or integration checks. Candidate validation selects changed areas and the root area when present; it does not establish that all areas passed. Unchanged baseline failures are still failures.

See [Workflow](WORKFLOW.md) for phase checks. `handoff.md` is an intermediate record; use the final report for the outcome.

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

Reinstalling replaces runtime files and preserves `.refactor/config.json`, run records, and `last-run.json`. It removes recognized old generated commands without adding compatibility aliases. A customized old command stops installation before replacement; inspect it first.

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
