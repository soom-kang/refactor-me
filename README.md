![refactor-me](docs/assets/refactor-me-title.png)

# refactor-me

[![Verify](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml/badge.svg)](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Run behavior-preserving refactors with Codex and Claude Code. The CLI edits and validates in a separate worktree and saves results on a local `refactor/auto-*` branch. Review the diff and report before merging.

**Go development build:** `dev`. The first Go Beta release is `0.9.20-beta.1` for macOS Apple Silicon.

[한국어](docs/README.ko.md) · [Guide](tool/TUTORIAL.md) · [Reference](tool/README.md) · [Workflow](tool/WORKFLOW.md) · [Changelog](tool/CHANGELOG.md)

## Install

Prepare these prerequisites:

- macOS Apple Silicon and Git; Go 1.27 only if building from source
- An authenticated `codex` or `claude` CLI and network access
- All eight Skills from [sharpen-me](https://github.com/soom-kang/sharpen-me), a separately released required dependency
- The target project's dependencies and build-tool caches

1. Download the macOS Apple Silicon archive and checksum file from the [`v0.9.20-beta.1` release](https://github.com/soom-kang/refactor-me/releases/tag/v0.9.20-beta.1). Verify the archive before extracting or running it. Replace the example repository path.

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

The archive contains `refactor-me`, `LICENSE`, `INSTALL.md`, and `BUILD-INFO.txt`. Compare `version --json` with the release version and read `INSTALL.md` before installation. `SHA256SUMS` detects a changed download; by itself it does not authenticate its publisher. This first Beta is **unsigned and not notarized**. macOS may block or warn about the download. If that happens, follow [Apple's instructions for opening an individual app](https://support.apple.com/en-gb/102445); do not disable system-wide protections.

For development, build from a source checkout with Go 1.27:

```bash
git clone https://github.com/soom-kang/refactor-me.git
cd refactor-me/tool/go
go build -o /private/tmp/refactor-me ./cmd/refactor-me
/private/tmp/refactor-me version --json  # version is dev
/private/tmp/refactor-me install /path/to/target-repo
```

2. Install the Skills in the **target repository** and choose Project scope.

```bash
cd /path/to/target-repo
npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code
```

3. Review and commit the Skill files, agent links, and `skills-lock.json`. Run worktrees read the base commit, so they cannot use uncommitted Skills. A global Claude installation does not satisfy this requirement.

```bash
git add .agents .claude skills-lock.json && git commit
./.refactor/bin/refactor-me doctor --no-live-probe
```

The installer writes the real files to `.agents/skills/<name>/` and makes `.claude/skills/<name>` a symlink into them. Commit both directories: committing one without the other leaves a link that resolves to nothing in the run worktree.

The installer copies the Go binary to `.refactor/bin/refactor-me` and preserves existing configuration and run records. It only replaces a recognized previous installation; inspect any ownership error rather than overwriting the file. The target repository needs no Go toolchain for the release binary. The `npx skills add` step still needs Node.js.

For reinstall, removal, or a return to the earlier Node CLI, follow the [update and rollback steps](tool/TUTORIAL.md#update-and-remove). The old Node installer replaces `.refactor/lib/src` and `.refactor/lib/bin`; inspect those directories for user files before running it.

## Run

Set limits using the [guide](tool/TUTORIAL.md#set-limits-and-run) before your first run. From the target repository:

```bash
./.refactor/bin/refactor-me version
./.refactor/bin/refactor-me doctor
./.refactor/bin/refactor-me --provider codex --fallback none
```

`doctor` writes diagnostics and calls models, consuming account usage. `doctor --no-live-probe` skips model calls and does not verify session Skill loading.

<details>
<summary>Provider switching and discovery scope options</summary>

```bash
./.refactor/bin/refactor-me --provider codex --fallback claude
./.refactor/bin/refactor-me --target app/web
./.refactor/bin/refactor-me --target app/web --target app/api
```

`--target` limits candidate discovery. Caller changes and validation can extend beyond that directory. See [scope and validation rules](tool/README.md#candidate-selection).

</details>

Keep the source checkout clean during execution. The CLI creates local commits and a result branch; it does not merge, push, or deploy.

## Reports

Read the result after a run. English is the default language.

```bash
./.refactor/bin/refactor-me report
./.refactor/bin/refactor-me report --lang ko
./.refactor/bin/refactor-me report --json
```

Viewing reports does not overwrite files or call a model. To select Korean for a **new run**:

```bash
./.refactor/bin/refactor-me --lang ko
```

Reports include file statistics, a diff preview, and `changes.patch`, the full text patch. Review the status and changes before merging. See the reference for [report formats and limits](tool/README.md#reports-and-language) and [Skill roles](tool/README.md#skill-availability).

## Verify

From this repository root, enter `tool/go`:

```bash
cd tool/go
go test -race ./...
go vet ./...
go build -o /private/tmp/refactor-me ./cmd/refactor-me
/private/tmp/refactor-me version --json
```

Local tests check installation and CLI behavior. Live model decision quality requires separate provider runs. See [validation boundaries](tool/README.md#local-development-checks).

## License

The CLI and documentation are distributed under the [MIT License](LICENSE), copyright 2026 soom-kang. The repository license applies to installed Go binaries. The required sharpen-me Skills retain their own MIT license files. Codex and Claude Code are external prerequisites governed by their providers' terms.
