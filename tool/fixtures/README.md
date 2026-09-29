# Local refactoring fixtures

[한국어](README.ko.md) · [Development checks](../README.md#local-development-checks) · [Workflow](../WORKFLOW.md)

Generate a disposable Git repository to inspect candidate discovery, command detection, and baseline handling. The generator uses Go 1.27 and Git. It creates an initial commit, prints the repository path, and does not call Codex or Claude.

## Go example

From this repository root:

```bash
cd tool/go
FIXTURE="$(go run ./cmd/make-fixture)"
printf '%s\n' "$FIXTURE"
(
  cd "$FIXTURE"
  go vet ./...
  go build ./...
)
```

The default fixture includes unused code, a plugin reached through a string registry, duplicate logic, a large file, compatibility code, and an intentional failure. `go vet ./...` and `go build ./...` should pass (`GREEN`). The following command should fail (`RED`) at `TestKnownBaseline`:

```bash
(cd "$FIXTURE" && go test -count=1 ./...)
```

This failure is part of the input, not a failed generator check. A refactoring candidate must keep the passing commands green and introduce no new failure signatures. Do not fix the intentional failure merely to make the fixture green. A baseline with no passing command cannot reach audit.

## Options and destination safety

Run these examples from `tool/go`. Explicit destinations must not already exist, even if empty. The generator refuses existing files, directories, and symlinks instead of removing them. Omit the destination for a fresh temporary directory.

```bash
go run ./cmd/make-fixture /tmp/my-new-refactor-fixture --lang go --multi
```

`--multi` adds independent `app/api` and `app/worker` areas in the selected language. Each has its own manifest. The generator does not install or copy Skills. Actual provider runs resolve global Skills from `~/.agents/skills`, independently of the fixture base commit. The removed `--skills` option is rejected.

An invalid option or an existing destination is an error. Inspect the diagnostic and choose a new destination; do not delete an existing repository to retry.

## Optional JavaScript example

Generating the files still uses Go. Executing their checks requires Node.js 24+; no npm dependencies need installation. The Workflow document uses this example's `.mjs` paths.

```bash
# From tool/go
JS_FIXTURE="$(go run ./cmd/make-fixture --lang js)"
(
  cd "$JS_FIXTURE"
  node tools/lint.mjs
  node --test test/*.test.mjs
  node tools/build.mjs
)
(cd "$JS_FIXTURE" && node tools/typecheck.mjs)
```

Lint, tests, and build should pass. Typecheck deliberately fails on `src/broken.mjs`; treat it as a readable `RED` baseline. The `.mjs` files are generated in the fixture, not installed as part of the refactor-me CLI. Their source templates are stored as non-executable `.txt` files.

To include JavaScript execution in the generator's integration tests, run from `tool/go`:

```bash
REFACTOR_TEST_JS=1 go test -count=1 ./internal/fixture
```

The default test run does not execute Node. For CI, manually dispatch Verify with `js_fixture: true` to enable JavaScript fixture checks. Neither command above invokes a provider or measures model decision quality. Real runs require separately installed global Skills, authenticated providers and explicit run limits as described in the [guide](../TUTORIAL.md). After generating a fixture, use `refactor-me init --repo "$FIXTURE"` and `refactor-me doctor --repo "$FIXTURE" --no-live-probe`. Starting `run` is a separate, usage-consuming action.
