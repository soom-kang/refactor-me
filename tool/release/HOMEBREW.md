# Homebrew source release

This procedure prepares `0.10.0-beta.1` for `soom-kang/homebrew-refactor-me`.
The tap and public source asset are pending publication. The formula supports macOS ARM only, builds with Homebrew Go, and never installs Skills or modifies project state during installation.

## Local candidate

Requirements: macOS Apple Silicon, Git, Go 1.27+, Homebrew, Python 3. No provider call is made by these commands.

From the repository root:

```sh
python3 -m unittest discover -s tool/release -p 'test_*.py'
python3 tool/release/prepare-homebrew.py /tmp/refactor-homebrew-candidate --candidate
cd /tmp/refactor-homebrew-candidate
shasum -a 256 -c SHA256SUMS
```

Choose a fresh output directory each time. Candidate mode includes local source changes, records the base commit and `dirty` status in `BUILD-INFO.json`, and produces a local `file://` formula. It is not publishable. The source archive includes the Go module, embedded schemas and fixture templates, license, release version and build metadata; it contains no `.git` or user Skill directory.

To test the formula, create a disposable **local** tap (this changes the local Homebrew installation):

```sh
brew tap-new local/refactor-me-check
cp /tmp/refactor-homebrew-candidate/refactor-me.rb \
  "$(brew --repository local/refactor-me-check)/Formula/refactor-me.rb"
brew style local/refactor-me-check/refactor-me
brew install --build-from-source local/refactor-me-check/refactor-me
brew test local/refactor-me-check/refactor-me
refactor-me version --json
```

Do not replace an existing installation without checking its provenance. For cleanup after testing a newly installed candidate:

```sh
brew uninstall local/refactor-me-check/refactor-me
brew untap local/refactor-me-check
```

`brew audit --strict` can additionally inspect formula metadata; online checks require network access. Candidate file URLs are intentionally local. Check the final HTTPS formula again before publishing. `brew test` verifies version/commit and state initialization without authenticating or invoking providers.

## Release gates

From `tool/go`:

```sh
go test -race -count=1 ./...
go vet ./...
golangci-lint run --config ../../.golangci.yml
govulncheck ./...
```

Use golangci-lint `v2.14.0` and govulncheck `v1.8.0`, installed separately as development tools. Run default checks through `tool/check-no-node.sh`. Database access is required for govulncheck; a failed download is not a passed scan. CI installs fixed tool versions outside the module. No production dependency is added.

Complete independent code review, EN/KO installation cold read, and separately approved bounded Codex/Claude fixture runs. Record provider Skill loading separately from file/hash verification. Preserve source checkout, provider usage and run reports. One provider failing the required gate blocks publication.

## Publication boundary

Commit, push, tag, GitHub release and remote tap changes require the user's release approval. Existing tags/assets must not be replaced. First confirm remote state, release version, test evidence and tap access. Commit the approved source and create annotated `v0.10.0-beta.1` at that exact commit. On its clean checkout:

```sh
python3 tool/release/prepare-homebrew.py /tmp/refactor-homebrew-release
cd /tmp/refactor-homebrew-release
shasum -a 256 -c SHA256SUMS
```

Release mode refuses a dirty checkout or a tag not identifying HEAD. The generated formula uses a fixed GitHub release asset URL, SHA-256 and commit. Upload `refactor-me_0.10.0-beta.1_source.tar.gz` and its checksum to the draft prerelease. Review the formula and publish it as `Formula/refactor-me.rb` in the dedicated tap only after its URL is available. No bottle or automatic tap publishing is configured.

Record the final source hash after the approved commit; a dirty candidate hash cannot serve as release evidence. Publish the prerelease after CI passes and verify the public source again:

```sh
brew install soom-kang/refactor-me/refactor-me
brew test soom-kang/refactor-me/refactor-me
refactor-me version --json
```

Confirm the reported commit, target selection and `.refactor` paths in a temporary Git repository. `brew upgrade` changes the shared executable for every project. `brew uninstall` removes the executable and leaves each project's settings, records and result branches intact. This release does not claim Apple signing or notarization. A checksum verifies file integrity, not publisher identity.
