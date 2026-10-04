# Prepare a Homebrew release

[한국어](HOMEBREW.ko.md) · [Development checks](../DEVELOPMENT.md) · [User installation](INSTALL.md)

Maintain the source-built macOS Apple Silicon formula in `soom-kang/homebrew-refactor-me`. The generator fixes the source URL, SHA-256 and commit. Homebrew manages Go as a build dependency; it does not install Skills or change target repositories.

The current public release is [`0.10.0-beta.4`](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.4), commit `0032e594eaab3240d4dee1aa133be5b9d6eb3c42`. A version change in the working tree is preparation, not publication; the procedure below applies to future releases too.

## 1. Optional local candidate

Use macOS Apple Silicon, Git, Go 1.27+, Homebrew and Python 3. A local candidate is useful when changing packaging or the formula; ordinary releases can proceed to step 3. From the repository root, choose a fresh output directory outside the checkout:

```sh
python3 tool/release/prepare-homebrew.py \
  /tmp/refactor-homebrew-candidate --candidate
(cd /tmp/refactor-homebrew-candidate && shasum -a 256 -c SHA256SUMS)
```

Candidate mode includes local source changes and records the base commit and dirty state. Its `file://` formula is for local testing only. The archive includes source, embedded schemas and templates, LICENSE and build metadata; it excludes `.git` and user Skills.

## 2. Optional formula check

Use this check for formula or packaging changes that need a candidate installation. Use a clean disposable macOS Apple Silicon runner with no refactor-me installation. These commands install a candidate and change trust only for the owned test formula. If the runner already has refactor-me, use a different environment; preserve the user's installed package and trust settings.

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

The required code checks are `gofmt`, `go test -count=1 ./...`, `go vet ./...`, a CLI build and the four Python release tests, as described in [development checks](../DEVELOPMENT.md). PR and main CI run this suite. Before tagging, require a successful `Verify` run triggered by a push to `main` whose `headSha` equals the release commit. Record that run URL. A green run for another commit does not qualify, and no separate tag CI is required.

Reuse those results without rerunning the suite for the same source. Add race tests, lint, vulnerability scans, formula audits, independent review or installation cold reads only when the changed behavior or an unresolved failure warrants them. Live Codex/Claude fixtures are optional checks for provider integration changes and require authorization with bounded scope and recorded limits and usage. Keep raw provider logs private; they may contain source or account information.

Record the release commit, required CI result, archive hashes, formula and release notes. Use existing authorization for commit, push, release publication and tap updates; obtain approval only for actions outside that authorization.

## 4. Publish an approved version

Check remote refs and tap access. Set a new release version in `tool/RELEASE_VERSION` and add the matching CHANGELOG entry before the release commit. Do not reuse or move an existing tag.

After that commit's main CI passes, tag it as `v<release-version>`. On its clean checkout, generate the source first and the binary second, once each. Use fresh output directories outside the checkout:

```sh
python3 tool/release/prepare-homebrew.py /tmp/refactor-homebrew-release
bash tool/release/build-macos-arm64.sh /tmp/refactor-binary-release
```

Source release generation requires a clean checkout and a local version tag that identifies HEAD. Keep that HEAD and source unchanged through the binary build, which verifies the embedded version, commit and architecture. Prepare the source archive, binary archive and one `SHA256SUMS` containing both archive entries from the two output directories. Verify both hashes together with `shasum -a 256 -c SHA256SUMS` before uploading them to a draft prerelease. Write version-specific release notes from verified changes and known limits. Confirm uploaded asset names and hashes, then publish using the recorded main CI result; tag pushes do not start another test run.

Publish the generated `Formula/refactor-me.rb` to the tap after its public source URL is available. Keep the fixed hash and injected commit. No bottle or automatic tap publishing is configured. Do not claim Apple signing or notarization; checksums verify bytes, not publisher identity.

## 5. Verify the public installation

After the release assets are public and the tap formula is updated, dispatch the workflow from this repository against the published release tag:

```sh
release_tag="v$(cat tool/RELEASE_VERSION)"
gh workflow run verify.yml --ref "$release_tag" -f public_install=true
```

Check that this dispatch completes with `public-install` and aggregate `verify` successful and `go` skipped. The optional `js-fixture` job is skipped unless requested. In `public-install`, the installed JSON version must equal the selected tag checkout's `tool/RELEASE_VERSION`; the commit must equal that checkout's HEAD, with platform `darwin` and architecture `arm64`. A passing run on another ref, or a run without `public_install=true`, does not satisfy this release check. Inspect the public-install logs and preserve the run URL as evidence.

The workflow performs this installation once on a clean disposable runner without refactor-me. It registers the tap, trusts only the formula and verifies the short install command:

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
brew test refactor-me
refactor-me version --json
```

`brew test` already checks version, commit, help, repository selection, configuration defaults and preservation across repeated `init`. The workflow also compares the installed version and commit with the selected release tag. Do not repeat these checks locally or add live provider calls for installation verification. Preserve the user's installed package and unrelated Homebrew trust.

Only after public installation passes, update the published version and commit in English/Korean installation documents and both tap READMEs, remove candidate notices, and add a `Verify` badge linked to the actual main-repository workflow. These changes update the repository documents, not an already published archive's enclosed guide. A tap README should label that badge `CLI Verify`; it does not certify tap CI or live provider execution. Keep the title image. Existing users may then run `brew upgrade refactor-me` themselves. Documentation-only changes do not require a new tag, asset or formula version.

## Beta.4 verification record

Release commit: `0032e594eaab3240d4dee1aa133be5b9d6eb3c42`. [Main CI](https://github.com/soom-kang/refactor-me/actions/runs/37207945672) · [Selected-tag public installation check](https://github.com/soom-kang/refactor-me/actions/runs/37208178549).

- Main CI: `PASS` for the exact release commit, covering formatting, Go tests, vet, CLI build and the four Python release tests. No duplicate tag CI was run.
- Public installation: `PASS`. `brew install` and `brew test` passed; the installed version and commit matched the selected release tag, with platform `darwin` and architecture `arm64`. `public-install` and aggregate `verify` passed; `go` and `js-fixture` were skipped.
- The source archive, binary archive and combined `SHA256SUMS` were verified before publication.
- Live Codex/Claude fixtures and a complete HAPJOO rerun: `NOT_RUN` for this release. The checks above do not establish completion of that target's refactoring workflow.

## Beta.3 verification record

Release commit: `01a3fad54149cb127e8bd6c5d638eda0562a5897`. [Tag CI](https://github.com/soom-kang/refactor-me/actions/runs/36970731411) · [Selected-tag public installation check](https://github.com/soom-kang/refactor-me/actions/runs/36971167653).

- Selected release-tag public-install result: `PASS`. Required successful jobs are `go`, `public-install` and aggregate `verify`. This run checks the public formula's style, audit, install and test, and compares version/commit with the selected checkout. Installation checks make no provider calls.
- Codex: `PASS` for all 12 acceptance criteria; one reviewed `DEAD_CODE` commit, six CLI calls (one per phase), no retries or fallback; 1,108.8 seconds; `DONE_PARTIAL` at the one-cycle limit. Requested `gpt-6.1-sol`/`xhigh`, a 900-second refactoring-phase provider timeout, at most three attempts, one refactor commit and a 20-minute elapsed limit checked between work units; embedded doctor only, with a 180-second live-probe timeout. Aggregate reported usage: 987,494 input tokens, 27,695 output tokens; `costUsd=null` (unavailable).
- Integrity: clean release source `01a3fad54149cb127e8bd6c5d638eda0562a5897`; final binary SHA-256 `b55f0af5e389dd611772ac5592295aa0d6619c5910f6703944570737c303d911` matches the live-tested binary. All 94 measured runtime files and eight global Skills remained unchanged; source fixture HEAD, index, tracked files and configuration were preserved. `go vet` and `go build` remained GREEN; `go test` retained only the expected RED signature `GOFAIL:TestKnownBaseline`, `contract_test.go:7`. Skill-loading probe responses are provider self-report; file hashes were measured separately.
- Claude: user manual verification `PASS`; automated verification `NOT_RUN` because tokens were exhausted. No automated Claude call was made for Beta.3.
- Preserve raw logs privately. The Codex result covers the bounded fixture described above.

## Beta.2 verification record

Release commit: `c30dbdadc4229958e29366fdc14d79df3da8cb02`. [Selected-tag public installation check](https://github.com/soom-kang/refactor-me/actions/runs/36956289983). The following evidence covers this release and the bounded fixtures, not every target repository or provider account.

- Main and tag CI check the Go implementation and aggregate `verify`. The selected-tag public-install run also requires `public-install`, verifies the public formula with `brew style`, `brew audit --strict`, install and test, and compares version/commit with the selected checkout. Optional `js-fixture` may be skipped. Installation checks make no provider calls.
- Codex: `PASS; 1 reviewed commit, 6 calls, no retries; 1,049.1 seconds; DONE_PARTIAL at cycle limit`. One additional run used a fresh Go fixture, requested `gpt-6.1-sol`/`xhigh`, no fallback, a 900-second call timeout and the existing maximum of three attempts. Earlier audit attempts at 300 seconds timed out three times without a usable response; that failure remains part of the record. No extra standalone doctor was added. The clean test binary was built from `265d3547b2541ce695f2ee8cedfab7225916cc18`; all runtime files under `tool/go` match the release commit.
- Claude: the earlier passing fixture was reused after all 85 files under `tool/go` and all eight Skill `name/path/sha256` records matched the release source and catalog. Its requested settings were `claude-sonnet-5-5`/`xhigh`, with no fallback, and it produced one reviewed refactor commit.
- Each fixture is limited to one cycle and at most one refactor commit. The 20-minute elapsed-time limit is checked between work units; it is not a hard whole-process deadline or a monetary cap. Acceptance requires one reviewed refactor commit, `go vet ./...` and `go build ./...` remaining GREEN, and only the known `go test -count=1 ./...` RED signature `GOFAIL:TestKnownBaseline`, `contract_test.go:7`. Source HEAD, index, tracked files and configuration must remain unchanged.
- `providerSettings` records requested model/effort, not observed execution. Live Skill-loading responses are provider self-report; measured file hashes are separate evidence. Keep raw provider logs private. A successful bounded fixture does not certify model quality or unrestricted compatibility.
