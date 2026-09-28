![refactor-me](docs/assets/refactor-me-title.png)

# refactor-me

[![Verify](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml/badge.svg)](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Run behavior-preserving refactors with Codex and Claude Code. The CLI edits and validates in a separate worktree and saves results on a local `refactor/auto-*` branch. Review the diff and report before merging.

**Go development build:** `dev`. The next public release starts at `0.9.20-beta.1`; it is not published by this change.

[한국어](docs/README.ko.md) · [Guide](tool/TUTORIAL.md) · [Reference](tool/README.md) · [Workflow](tool/WORKFLOW.md) · [Changelog](tool/CHANGELOG.md)

## Install

Prepare these prerequisites:

- macOS Apple Silicon, Git, and Go 1.27 to build from source
- An authenticated `codex` or `claude` CLI and network access
- All eight Skills from [sharpen-me](https://github.com/soom-kang/sharpen-me), a separately released required dependency
- The target project's dependencies and build-tool caches

1. Build the Go CLI from this checkout and install it in a target repository. Replace the example path.

```bash
git clone https://github.com/soom-kang/refactor-me.git
cd refactor-me
cd tool/go
go build -o /private/tmp/refactor-me ./cmd/refactor-me
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
```

The installer writes the real files to `.agents/skills/<name>/` and makes `.claude/skills/<name>` a symlink into them. Commit both directories: committing one without the other leaves a link that resolves to nothing in the run worktree.

The installer copies the Go binary to `.refactor/bin/refactor-me` and preserves existing configuration and run records. The target repository needs no Go toolchain for this binary. The `npx skills add` step still needs Node.js.

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
