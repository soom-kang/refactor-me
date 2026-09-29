# Prepare a Homebrew release

[한국어](HOMEBREW.ko.md) · [Development checks](../DEVELOPMENT.md) · [User installation](INSTALL.md)

Maintain the source-built macOS Apple Silicon formula in `soom-kang/homebrew-refactor-me`. The generator fixes the source URL, SHA-256 and commit. Homebrew manages Go as a build dependency; it does not install Skills or change target repositories.

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

Use a disposable local tap. These commands change your local Homebrew installation. Check an existing `refactor-me` installation first; do not replace it as part of an unrelated test.

```sh
brew tap-new local/refactor-me-check
cp /tmp/refactor-homebrew-candidate/refactor-me.rb \
  "$(brew --repository local/refactor-me-check)/Formula/refactor-me.rb"
brew style local/refactor-me-check/refactor-me
brew audit --strict local/refactor-me-check/refactor-me
brew install --build-from-source local/refactor-me-check/refactor-me
brew test local/refactor-me-check/refactor-me
refactor-me version --json
```

`brew test` checks version, commit and initialization without provider calls. Review the final HTTPS formula again before publication. To remove only the candidate installation and disposable tap after testing:

```sh
brew uninstall local/refactor-me-check/refactor-me
brew untap local/refactor-me-check
```

## 3. Complete release checks

Run the [development checks](../DEVELOPMENT.md), lint and vulnerability scan. Review code changes independently and cold-read English and Korean installation instructions.

Run separately approved bounded Codex and Claude fixtures. Record source checkout integrity, Skill paths/hashes, session-loading results, limits, outcomes and usage. File validation and provider self-report are different evidence. A failing required provider check blocks publication.

Record the candidate commit, local results, CI status, source hash, formula and release notes for approval. Keep raw provider logs private; they may contain source or account information.

## 4. Publish an approved version

Check remote refs and tap access. Set a new release version in `tool/RELEASE_VERSION` and add the matching CHANGELOG entry before the approved commit. Do not reuse or move an existing tag.

After commit/push approval, tag that commit as `v<release-version>`. On its clean checkout:

```sh
python3 tool/release/prepare-homebrew.py /tmp/refactor-homebrew-release
(cd /tmp/refactor-homebrew-release && shasum -a 256 -c SHA256SUMS)
```

Release mode refuses dirty source or a tag that does not identify HEAD. Upload the generated source archive and checksums to a draft prerelease. Write version-specific release notes from verified changes and known limits. Wait for tag CI to pass, confirm asset names/hashes, then publish.

Publish the generated `Formula/refactor-me.rb` to the tap after its public source URL is available. Keep the fixed hash and injected commit. No bottle or automatic tap publishing is configured. Do not claim Apple signing or notarization; checksums verify bytes, not publisher identity.

## 5. Verify the public installation

On a machine without refactor-me installed:

```sh
brew install soom-kang/refactor-me/refactor-me
brew test soom-kang/refactor-me/refactor-me
refactor-me version --json
```

For an existing installation, review and use `brew upgrade` instead. Confirm version and commit, then test repository selection, repeated `init` and report paths in a temporary Git repository. Preserve the source HEAD, index and tracked files. Updating docs alone does not require a new tag, asset or formula version.
