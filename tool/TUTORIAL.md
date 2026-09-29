# refactor-me guide

[Project](../README.md) · [한국어](TUTORIAL.ko.md) · [Reference](README.md) · [Workflow](WORKFLOW.md)

Prepare the target repository, then follow these five steps. Check each command’s working directory and replace the example paths.

<a id="prepare-the-target"></a>

## 1. Prepare the target

Use macOS Apple Silicon and Homebrew. The formula manages Go as a build dependency; a manual source build requires Go 1.27. From the repository you want to refactor:

```bash
cd /path/to/target-repo
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

## 2. Install the tool and global Skills

`0.10.0-beta.1` is a release candidate. The Homebrew instructions require the tap and source release to be published; use the [development build](../README.md#install) until then.

```sh
brew install soom-kang/refactor-me/refactor-me
refactor-me version --json
npx skills add \
  https://github.com/soom-kang/sharpen-me/tree/v0.9.0-beta.2 \
  --global --skill '*' --agent codex claude-code
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo --no-live-probe
```

Homebrew builds the CLI using Go and places it on PATH. The external `npx` installer needs Node.js; CLI execution does not. The shared Skill source is `~/.agents/skills`. Do not commit Skills to target repositories for this workflow. Distinct same-name Skills in provider search paths block execution; doctor reports the paths for you to inspect. Aliases resolving to the same source are allowed.

`init` is optional and creates `.refactor/config.json` without overwriting an existing file. It copies neither the binary nor Skills. Running `refactor-me` without arguments shows help.

Codex uses the selected global Skill paths. Claude receives a dedicated per-run copy through `--add-dir`, while `--setting-sources project` continues to exclude user settings. Skill contents are checked around provider calls; an unexpected change stops publication.

For a development build, replace `refactor-me` in every command below with `/private/tmp/refactor-me`.

The disk-only doctor check writes diagnostics but does not establish live Skill loading. To test the authenticated provider sessions, run the following separately; it calls models and consumes account usage:

```sh
refactor-me doctor --repo /path/to/target-repo
```

This Beta is not Apple-signed or notarized. If macOS blocks a downloaded executable, follow [Apple's individual-app instructions](https://support.apple.com/en-gb/102445). Do not disable system-wide protections.

<a id="set-limits-and-run"></a>

## 3. Set limits

After `init`, edit `/path/to/target-repo/.refactor/config.json`: set `policy.max_commits` to `1` and `policy.max_wall_clock_min` to a suitable limit (for example `30`) for a first run. Characterization test commits are counted apart from this refactor-commit limit.

**The loop does not enforce a total monetary budget.** Claude’s budget option is not a total run limit either. See [configuration and defaults](README.md#configuration).

## 4. Run

From the target repository, use Codex only:

```bash
refactor-me run --provider codex --fallback none
```

With Claude available as a fallback:

```bash
refactor-me run --provider codex --fallback claude
```

To find candidates in one directory:

```bash
refactor-me run --target app/web
```

`--target` is relative to your current directory when `--repo` is omitted. To run from any directory, use `refactor-me run --repo /path/to/target-repo --target app/web`; targets then start at the selected Git root. `--repo` accepts a repository or a directory within it, and relative repository paths start at your calling directory. Paths resolving outside the selected repository are rejected. Caller changes and validation can extend beyond the target within the repository. Keep the source checkout unchanged during a run.

Accepted changes remain on a local `refactor/auto-*` branch. The loop stops for no eligible candidates, run limits, repeated failures, unavailable providers, or safety violations.

**Exit code `0` can mean partial completion.** Check the status in the next step.

<a id="read-the-result"></a>

## 5. Read the result

```bash
refactor-me report
refactor-me report --lang ko
refactor-me report --json
```

To write the report and final summary in Korean when running:

```bash
refactor-me run --provider codex --fallback none --lang ko
```

Viewing another language preserves the saved `report.md`. `--lang` applies to `run` and `report`; doctor and progress logs remain in English. See [reports and language](README.md#reports-and-language) for translated fields and supported JSON format.

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
| Missing global Skill | Check the named directory under `~/.agents/skills` and rerun the global installer if needed |
| Skill name conflict | Inspect the reported provider/project paths and choose one canonical source; the CLI does not delete them |
| Skill changed during a run | Preserve diagnostics, finish catalog maintenance, then start a new run |
| Dirty source checkout | Finish or set aside your work, then rerun doctor |
| No usable baseline | Inspect command failures, dependencies, and build caches; define commands if discovery is insufficient |
| No eligible candidates | Read the exclusion reasons; a run can finish without changes |
| Partial completion | Inspect accepted commits and the stop reason before starting another run |
| Safety halt, exit `4` | Keep the worktree and diagnostics; investigate the violated invariant |
| Report JSON missing or invalid | Restore the recorded JSON if available; existing Markdown is not overwritten |

For explicit validation commands, see [Validation commands](README.md#validation-commands). Run records may contain source excerpts and command output; review them before sharing.

## Update and remove

```sh
brew upgrade soom-kang/refactor-me/refactor-me
refactor-me version --json
refactor-me doctor --repo /path/to/target-repo --no-live-probe
```

An upgrade changes the executable used by every project. Review the release notes before upgrading. Global Skills are maintained separately; inspect local modifications before updating the pinned catalog and do not update it while a run is active.

Current configuration requires `schema_version: 2`; reports require `schemaVersion: 3`. Earlier formats are rejected without rewriting or deleting them. There is no migration or Node rollback command. If an old configuration exists, preserve it outside the active `.refactor/config.json` path, run `init`, and transfer reviewed settings into the generated schema. Keep old run records as archives; the current CLI cannot render them. `init` itself never moves or overwrites existing files.

To remove eligible finished worktrees:

```sh
refactor-me clean --repo /path/to/target-repo
```

Partial, unfinished and safety-halted worktrees remain available for investigation. To remove the Homebrew executable:

```sh
brew uninstall refactor-me
```

Removal leaves project configuration, reports, worktrees, result branches and global Skills in place. The CLI no longer implements project-local `install` or `uninstall`. It does not remove older local executables. Use `command -v refactor-me` and `version --json` to identify the executable in use; inspect any old project files before removing them yourself.
