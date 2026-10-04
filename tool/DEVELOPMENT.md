# Develop and verify refactor-me

[Project](../README.md) · [Fixtures](fixtures/README.md) · [Release procedure](release/HOMEBREW.md)

Use macOS Apple Silicon, Git and Go 1.27 or later. The module lives in `tool/go` and uses the standard library. Default project checks do not require Node or call providers.

## 1. Build a development binary

From the repository root:

```sh
cd tool/go
go build -o /private/tmp/refactor-me-dev ./cmd/refactor-me
/private/tmp/refactor-me-dev version --json
/private/tmp/refactor-me-dev help
```

Expect version `dev`. Use this absolute binary path for development commands; `refactor-me` on PATH may be the Homebrew release. Keep release builds tied to the commit and version in `tool/RELEASE_VERSION`.

## 2. Run checks

From the repository root:

```sh
bash tool/check-no-node.sh sh -c '
  cd tool/go &&
  test -z "$(gofmt -l .)" &&
  go test -count=1 ./... &&
  go vet ./... &&
  go build -o /private/tmp/refactor-me-dev ./cmd/refactor-me
'
python3 -m unittest discover -s tool/release -p 'test_*.py'
```

These are the default PR and main checks. A release uses the successful main run for its exact commit; tags and public-install checks do not repeat this suite. Reuse passing results for unchanged source instead of repeating the same checks locally and in CI.

The Node guard applies to project tooling. GitHub Actions may use JavaScript internally. Optional JavaScript fixture checks require Node; see the [fixture guide](fixtures/README.md).

Add checks when the changed behavior warrants them: use `go test -race` for the affected concurrency packages, formula style/audit checks for formula changes, and lint or vulnerability scans for relevant static-analysis, dependency, toolchain or security concerns. These are not routine release gates. If needed, install golangci-lint `v2.14.0` and govulncheck `v1.8.0` as separate development tools, then run from `tool/go`:

```sh
golangci-lint run --config ../../.golangci.yml
govulncheck ./...
```

A failed vulnerability database download is not a passed scan. CI configuration is in [verify.yml](../.github/workflows/verify.yml).

## 3. Check documentation changes

Keep English and Korean user instructions equivalent. Preserve executable examples and their expected failures. The ten JSON examples in both WORKFLOW editions are inputs to schema and controller tests.

Check model/effort examples against CLI parsing and provider-argument tests. Cover required models for live commands, the offline doctor exception, CLI-over-configuration precedence, explicit effort across all phases and unchanged configuration bytes. `providerSettings` records requested values; it does not establish that a provider executed them. Before updating published version/commit and workflow badges, require successful public-install verification against the selected release tag.

When changing progress display, use the development binary built in step 1 and test the affected behavior: English defaults and `--lang ko`, per-audit counts and caps, unchanged baseline failures, confirmed rollback/commit results, and diagnostic paths. Cover provider repair/retry/fallback without exposing raw notes or commands, Unicode display limits, and `run --json` with progress on stderr and only report JSON on stdout. Use the race detector when changing heartbeat shutdown or serialized output. Saved transcripts, tool-call accounting and report/configuration schemas must retain their existing contracts. Keep published version and verification records consistent in both languages and the tap READMEs.

Check relative links, anchors and diagram exports after deleting or moving a document. Diagram editing and export instructions are in the [asset guide](../docs/assets/workflow/README.md).

## 4. Separate local and live checks

Fixtures and test doubles verify controller contracts. They do not establish real provider Skill loading or model decision quality. Use a bounded live fixture only when a provider integration change or unresolved failure needs that evidence; it is not a routine release gate. Real Codex/Claude runs require authorization, disposable repositories and recorded limits and usage. Existing explicit authorization applies; do not request it again for the same scope.

Review the changed code and instructions within their affected scope. Independent reviews and installation cold reads are optional when they address a concrete remaining risk. Public release and tap changes follow the [Homebrew procedure](release/HOMEBREW.md).
