# refactor-me guide

[Project](../README.md) · [한국어](TUTORIAL.ko.md) · [Reference](README.md) · [Workflow](WORKFLOW.md)

Prepare the target repository, then follow these five steps. Check each command’s working directory and replace the example paths.

<a id="prepare-the-target"></a>

## 1. Prepare the target

Use macOS Apple Silicon. Go 1.27 is needed only for a source build. From the repository you want to refactor:

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

## 2. Install the tool and skills

Download the archive and checksum file from the [`v0.9.20-beta.1` release](https://github.com/soom-kang/refactor-me/releases/tag/v0.9.20-beta.1). In a terminal:

```bash
RELEASE_DIR="$(mktemp -d)"
(
set -e
RELEASE_VERSION=0.9.20-beta.1
RELEASE_URL="https://github.com/soom-kang/refactor-me/releases/download/v${RELEASE_VERSION}"
curl -fL "$RELEASE_URL/refactor-me_${RELEASE_VERSION}_darwin_arm64.zip" -o "$RELEASE_DIR/refactor-me_${RELEASE_VERSION}_darwin_arm64.zip"
curl -fL "$RELEASE_URL/SHA256SUMS" -o "$RELEASE_DIR/SHA256SUMS"
cd "$RELEASE_DIR"
shasum -a 256 -c SHA256SUMS
unzip -q "refactor-me_${RELEASE_VERSION}_darwin_arm64.zip"
cat INSTALL.md
RELEASE_INFO="$(./refactor-me version --json)"
printf '%s\n' "$RELEASE_INFO"
printf '%s\n' "$RELEASE_INFO" | grep -Fq "\"version\": \"$RELEASE_VERSION\""
printf '%s\n' "$RELEASE_INFO" | grep -Fq '"platform": "darwin"'
printf '%s\n' "$RELEASE_INFO" | grep -Fq '"arch": "arm64"'
./refactor-me install /path/to/target-repo
)
```

The archive contains `refactor-me`, `LICENSE`, `INSTALL.md`, and `BUILD-INFO.txt`. Read `INSTALL.md` and confirm the displayed version before installing. `SHA256SUMS` checks the downloaded bytes, not the publisher's identity. This first Beta is unsigned and not notarized; macOS may block or warn about it. Follow [Apple's individual-app opening instructions](https://support.apple.com/en-gb/102445) if needed. Do not disable macOS protections system-wide. For a development source build, use the [README instructions](../README.md#install).

Install the eight required sharpen-me Skills in the target repository:

```bash
cd /path/to/target-repo
npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code
```

Choose Project scope. Review and commit the eight Skill directories, agent links, and `skills-lock.json`. Worktrees read files from the base commit. Keep unrelated files out of the commit.

```bash
git add .agents .claude skills-lock.json && git commit
```

Both directories: the real files land in `.agents/skills/<name>/`, and `.claude/skills/<name>` is a symlink into them. Committing one without the other leaves a link that resolves to nothing in the worktree.

Check the installed tool:

```bash
./.refactor/bin/refactor-me version
./.refactor/bin/refactor-me help
./.refactor/bin/refactor-me doctor --no-live-probe
./.refactor/bin/refactor-me doctor
```

Doctor writes diagnostics under `.refactor/runs/` and calls models. Resolve blocking failures. `doctor --no-live-probe` checks disk installation without verifying session Skill loading.

<a id="set-limits-and-run"></a>

## 3. Set limits

In `.refactor/config.json`, set `policy.max_commits` to `1` and `policy.max_wall_clock_min` to a suitable limit (for example `30`) for a first run. Characterization test commits are counted apart from this refactor-commit limit.

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
| `skill-worktree` failure | A required Skill is missing from the base-commit checkout. Commit both `.agents` and `.claude`; the `.claude` entries are symlinks into `.agents`, so one without the other is not enough |
| Dirty source checkout | Finish or set aside your work, then rerun doctor |
| No usable baseline | Inspect command failures, dependencies, and build caches; define commands if discovery is insufficient |
| No eligible candidates | Read the exclusion reasons; a run can finish without changes |
| Partial completion | Inspect accepted commits and the stop reason before starting another run |
| Safety halt, exit `4` | Keep the worktree and diagnostics; investigate the violated invariant |
| Report JSON missing or invalid | Restore the recorded JSON if available; existing Markdown is not overwritten |

For explicit validation commands, see [Validation commands](README.md#validation-commands). Run records may contain source excerpts and command output; review them before sharing.

## Update and remove

For an update, choose the **published newer version** in `NEW_VERSION` and download it in a new directory. This block works in a fresh terminal; it stops before installation if the hash or embedded version differs. Keep the previous verified archive until the update is working.

```bash
NEW_VERSION=0.9.20-beta.1  # replace with the published newer version
RELEASE_DIR="$(mktemp -d)"
(
set -e
RELEASE_URL="https://github.com/soom-kang/refactor-me/releases/download/v${NEW_VERSION}"
curl -fL "$RELEASE_URL/refactor-me_${NEW_VERSION}_darwin_arm64.zip" -o "$RELEASE_DIR/refactor-me_${NEW_VERSION}_darwin_arm64.zip"
curl -fL "$RELEASE_URL/SHA256SUMS" -o "$RELEASE_DIR/SHA256SUMS"
cd "$RELEASE_DIR"
shasum -a 256 -c SHA256SUMS
unzip -q "refactor-me_${NEW_VERSION}_darwin_arm64.zip"
cat INSTALL.md
RELEASE_INFO="$(./refactor-me version --json)"
printf '%s\n' "$RELEASE_INFO" | grep -Fq "\"version\": \"$NEW_VERSION\""
printf '%s\n' "$RELEASE_INFO" | grep -Fq '"platform": "darwin"'
printf '%s\n' "$RELEASE_INFO" | grep -Fq '"arch": "arm64"'
./refactor-me install /path/to/target-repo
)
```

Reinstalling replaces the owned Go binary and preserves `.refactor/config.json`, run records, and `last-run.json`. A recognized Node shim can be replaced during migration; custom or unidentified files are preserved and reported. An unrecognized command stops installation before replacement. Check the output and investigate any preserved file before deleting it yourself.

Review local Skill edits before updating the catalog in the target repository. Use the [sharpen-me documentation](https://github.com/soom-kang/sharpen-me) for catalog maintenance, then commit the reviewed update.

To remove finished worktrees, run from the target repository:

```bash
./.refactor/bin/refactor-me clean
```

The command retains partial, unfinished, and safety-halted worktrees. To uninstall the Go tool, use the verified release executable you downloaded. Replace `RELEASE_DIR` with the absolute path of its extracted directory:

```bash
RELEASE_DIR=/absolute/path/to/verified/refactor-me-release
test -x "$RELEASE_DIR/refactor-me"
"$RELEASE_DIR/refactor-me" uninstall /path/to/target-repo
```

Uninstall removes the owned Go binary and its ownership marker. It keeps configuration, run records, custom files, Skill catalog, result branches, and worktrees.

To return to the earlier Node CLI, use Node.js 24 and fetch the vetted `v0.8.8-beta.1` source. Its expected commit is `fa845c98fba01f87466531ae50c5f1d671f0392f`. The old installer replaces `.refactor/lib/src` and `.refactor/lib/bin`, so the command stops if any library file remains after migration. Inspect and resolve those files separately; do not remove them merely to pass the check. Replace `RELEASE_DIR` with the absolute path of the verified Go release directory. The following leaves `.refactor/config.json` and `.refactor/runs/` in place:

```bash
RELEASE_DIR=/absolute/path/to/verified/refactor-me-release
test -x "$RELEASE_DIR/refactor-me"
ROLLBACK_SOURCE="$(mktemp -d)/refactor-me-v0.8.8-beta.1"
git clone --quiet --depth 1 --branch v0.8.8-beta.1 https://github.com/soom-kang/refactor-me.git "$ROLLBACK_SOURCE"
(
set -e
TARGET=/path/to/target-repo
test "$(git -C "$ROLLBACK_SOURCE" rev-parse HEAD)" = fa845c98fba01f87466531ae50c5f1d671f0392f
if [ -d "$TARGET/.refactor/lib" ] && [ -n "$(find "$TARGET/.refactor/lib" -mindepth 1 -print -quit)" ]; then
  echo 'Stop: inspect retained .refactor/lib files before Node rollback' >&2
  exit 2
fi
"$RELEASE_DIR/refactor-me" uninstall "$TARGET"
node "$ROLLBACK_SOURCE/tool/install.mjs" "$TARGET"
"$TARGET/.refactor/bin/refactor-me" version
"$TARGET/.refactor/bin/refactor-me" doctor --no-live-probe
)
```

If the Node installer or its `doctor` check fails after Go removal, keep the verified Go archive and run its `install` command to recover the Go CLI. If that installer reports an unrecognized command or file, stop and inspect the partial Node installation before retrying; do not overwrite it manually. A successful version check alone does not establish that the preserved configuration works with the older Node CLI.
