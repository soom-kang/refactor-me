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
  go test -race -count=1 ./... &&
  go vet ./...
'
python3 -m unittest discover -s tool/release -p 'test_*.py'
```

The Node guard applies to project tooling. GitHub Actions may use JavaScript internally. Optional JavaScript fixture checks require Node; see the [fixture guide](fixtures/README.md).

For the release gates, install golangci-lint `v2.14.0` and govulncheck `v1.8.0` as separate development tools, then run from `tool/go`:

```sh
golangci-lint run --config ../../.golangci.yml
govulncheck ./...
```

A failed vulnerability database download is not a passed scan. CI configuration is in [verify.yml](../.github/workflows/verify.yml).

## 3. Check documentation changes

Keep English and Korean user instructions equivalent. Preserve executable examples and their expected failures. The ten JSON examples in both WORKFLOW editions are inputs to schema and controller tests.

Check relative links, anchors and diagram exports after deleting or moving a document. Diagram editing and export instructions are in the [asset guide](../docs/assets/workflow/README.md).

## 4. Separate local and live checks

Fixtures and test doubles verify controller contracts. They do not establish real provider Skill loading or model decision quality. Real Codex/Claude runs require explicit approval, disposable repositories and recorded limits and usage.

Review implementation changes independently and cold-read installation instructions before a release. Public release and tap changes follow the [Homebrew procedure](release/HOMEBREW.md).
