# Prepare a Homebrew release

[한국어](HOMEBREW.ko.md) · [Development checks](../DEVELOPMENT.md) · [User installation](INSTALL.md)

Maintain the source-built macOS Apple Silicon formula in `soom-kang/homebrew-refactor-me`. The generator fixes the source URL, SHA-256 and commit. Homebrew manages Go as a build dependency; it does not install Skills or change target repositories.

The next candidate is `0.10.0-beta.2`; the current public release remains `0.10.0-beta.1` until publication. A version change in the working tree is preparation, not a published release.

## 1. Build a local candidate

Use macOS Apple Silicon, Git, Go 1.27+, Homebrew and Python 3. From the repository root, choose a fresh output directory:

```sh
python3 -m unittest discover -s tool/release -p 'test_*.py'
python3 tool/release/prepare-homebrew.py \
  /tmp/refactor-homebrew-candidate --candidate
(cd /tmp/refactor-homebrew-candidate && shasum -a 256 -c SHA256SUMS)
```

Candidate mode includes local source changes and records the base commit and dirty state. Its `file://` formula is for local testing only. The archive includes source, embedded schemas and templates, LICENSE and build metadata; it excludes `.git` and user Skills.

## 2. Check the formula

Use a clean disposable macOS Apple Silicon runner with no refactor-me installation. These commands install a candidate and change trust only for the owned test formula. If the runner already has refactor-me, stop and use a different environment; preserve the user's installed package and trust settings.

```sh
brew tap-new local/refactor-me-check
cp /tmp/refactor-homebrew-candidate/refactor-me.rb \
  "$(brew --repository local/refactor-me-check)/Formula/refactor-me.rb"
brew trust --formula local/refactor-me-check/refactor-me
brew style local/refactor-me-check/refactor-me
brew audit --strict local/refactor-me-check/refactor-me
brew install --build-from-source local/refactor-me-check/refactor-me
brew test local/refactor-me-check/refactor-me
refactor-me version --json
```

`brew test` checks version, commit and initialization without provider calls. Review the final HTTPS formula again before publication. To remove only the candidate installation and disposable tap after testing:

```sh
brew uninstall local/refactor-me-check/refactor-me
brew untrust --formula local/refactor-me-check/refactor-me
brew untap local/refactor-me-check
```

## 3. Complete release checks

Run the [development checks](../DEVELOPMENT.md), lint and vulnerability scan. Review code changes independently and cold-read English and Korean installation instructions.

Run separately approved bounded Codex and Claude fixtures with explicit selected models and effort, or models already configured in the fixture. Record source checkout integrity, Skill paths/hashes, session-loading results, limits, outcomes and usage. The model IDs in examples are user choices, not verified provider support. File validation, requested provider settings and provider self-report are different evidence. A failing required provider check blocks publication.

Record the candidate commit, local results, CI status, source hash, formula and release notes for approval. Keep raw provider logs private; they may contain source or account information.

## 4. Publish an approved version

Check remote refs and tap access. Set a new release version in `tool/RELEASE_VERSION` and add the matching CHANGELOG entry before the approved commit. Do not reuse or move an existing tag.

After commit/push approval, tag that commit as `v<release-version>`. On its clean checkout:

```sh
python3 tool/release/prepare-homebrew.py /tmp/refactor-homebrew-release
(cd /tmp/refactor-homebrew-release && shasum -a 256 -c SHA256SUMS)
bash tool/release/build-macos-arm64.sh /tmp/refactor-binary-release
```

Release mode refuses dirty source or a tag that does not identify HEAD. Prepare the source archive, binary archive and one `SHA256SUMS` containing both archive entries from the two output directories. Recheck both hashes together before uploading them to a draft prerelease. Write version-specific release notes from verified changes and known limits. Wait for tag CI to pass, confirm asset names/hashes, then publish.

Publish the generated `Formula/refactor-me.rb` to the tap after its public source URL is available. Keep the fixed hash and injected commit. No bottle or automatic tap publishing is configured. Do not claim Apple signing or notarization; checksums verify bytes, not publisher identity.

## 5. Verify the public installation

After the release assets are public and the tap formula is updated, dispatch the workflow from this repository against the published candidate tag:

```sh
gh workflow run verify.yml --ref v0.10.0-beta.2 -f public_install=true
```

Check that this dispatch completes with `go`, `public-install` and aggregate `verify` all successful. The optional `js-fixture` job may be skipped unless requested. In `public-install`, the installed JSON version must equal the selected tag checkout's `tool/RELEASE_VERSION`; the commit must equal that checkout's HEAD, with platform `darwin` and architecture `arm64`. A passing run on another ref, or a run without `public_install=true`, does not satisfy this release check. Inspect the public-install logs and preserve the run URL as evidence.

Use a clean disposable runner without refactor-me or its tap. Register the tap, trust only the formula and verify the short install command:

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
brew test refactor-me
refactor-me version --json
```

Confirm the published version and release commit, then test repository selection, repeated `init` and offline doctor in a temporary Git repository. Check report viewing with a saved fixture report; do not start a provider run for an installation check. Preserve the source HEAD, index and tracked files. Do not replace the user's installed package or change unrelated Homebrew trust to obtain this evidence.

Only after public installation passes, update the published version and commit in English/Korean installation documents and both tap READMEs, remove candidate notices, and add a `Verify` badge linked to the actual main-repository workflow. These changes update the repository documents, not an already published archive's enclosed guide. A tap README should label that badge `CLI Verify`; it does not certify tap CI or live provider execution. Keep the title image. Existing users may then run `brew upgrade refactor-me` themselves. Documentation-only changes do not require a new tag, asset or formula version.
