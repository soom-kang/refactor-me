# Install refactor-me

[한국어](https://github.com/soom-kang/refactor-me/blob/main/tool/release/INSTALL.ko.md) · [Usage guide](https://github.com/soom-kang/refactor-me/blob/main/tool/TUTORIAL.md)

This Beta supports macOS Apple Silicon (`darwin/arm64`) only. Use Homebrew for a shared executable and install sharpen-me Skills separately.

This guide targets `0.10.0-beta.2`. The download commands apply once its tag and assets are public on the [GitHub release page](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.2). Until then, the public release is `0.10.0-beta.1`; use its [tagged guide](https://github.com/soom-kang/refactor-me/blob/v0.10.0-beta.1/tool/TUTORIAL.md).

## Already extracted an archive

If you are reading the `INSTALL.md` enclosed in an archive, use that archive's executable. Skip the Homebrew and public-download steps below. From its extracted directory:

```sh
cat BUILD-INFO.txt
./refactor-me version --json
```

Compare the enclosed `version` and `commit` with the JSON output; both must match. The executable must identify itself as `refactor-me`, with platform `darwin` and architecture `arm64`, also matching `BUILD-INFO.txt`. A public release archive must have `dirty=false`, `signed=no` and `notarized=no`. Stop if any value differs. Keep it outside the target repository and use its absolute path for later commands.

A locally built candidate has its own enclosed version, commit and dirty state. Those values do not establish publication. For a public Beta.2 archive, also compare its commit with the release tag as described below.

Checksums verify archive bytes; `BUILD-INFO.txt` comparison detects a mismatched executable and metadata, not publisher identity. Later website or repository documentation updates do not change this enclosed guide.

## Homebrew

Prepare Git, an authenticated Codex or Claude Code CLI, and the target project's validation tools. Node.js is needed for the Skill installer, not refactor-me itself.

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
refactor-me version --json
npx skills add soom-kang/sharpen-me \
  --global --skill '*' --agent codex claude-code
```

Homebrew builds fixed release source with Go as a build dependency. Required Skills resolve from `~/.agents/skills`. Homebrew does not install Skills or write project configuration.

Tap registration and trust for this formula are one-time setup with Homebrew 6 or later. A new Homebrew environment needs them before `brew install refactor-me` can resolve the third-party formula. Trust only this formula; whole-tap trust is unnecessary.

If Homebrew reports `0.10.0-beta.1`, follow the [tagged public usage guide](https://github.com/soom-kang/refactor-me/blob/v0.10.0-beta.1/tool/TUTORIAL.md). For Beta.2, choose a model for every selected provider with command options or project configuration; there is no fixed model default. Offline `doctor --no-live-probe` needs no model and makes no model calls. Select a clean Git repository and set limits before starting `run`. Default doctor checks and `run` call models and consume provider usage.

## Standalone download

Once Beta.2 is public, create a new empty directory and download the archive and checksum file from its [GitHub release](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.2). In that directory:

```sh
release_url=https://github.com/soom-kang/refactor-me/releases/download
archive=refactor-me_0.10.0-beta.2_darwin_arm64.zip
curl -fLO "$release_url/v0.10.0-beta.2/$archive"
curl -fLO "$release_url/v0.10.0-beta.2/SHA256SUMS"
awk -v file="$archive" '$2 == file {print; count++} END {if (count != 1) exit 1}' \
  SHA256SUMS > "$archive.sha256" &&
  shasum -a 256 -c "$archive.sha256"
```

Proceed only when the checksum reports `OK`. Extract into the empty directory, then inspect the binary:

```sh
unzip "$archive"
cat BUILD-INFO.txt
./refactor-me version --json
git ls-remote https://github.com/soom-kang/refactor-me.git \
  'refs/tags/v0.10.0-beta.2^{}'
```

The last command prints the commit behind the annotated [release tag](https://github.com/soom-kang/refactor-me/tree/v0.10.0-beta.2), followed by its peeled ref. Its first field must match `commit` in both `BUILD-INFO.txt` and the JSON output. The version must be `0.10.0-beta.2`, platform `darwin` and architecture `arm64`; metadata must show `dirty=false`. Stop if the tag is missing or any value differs. For Homebrew, compare `refactor-me version --json` with the same tag commit.

Use the extracted executable by absolute path with the same commands as the Homebrew CLI. Keep it outside the target repository.

Checksums detect altered downloads; they do not authenticate the publisher. This Beta is unsigned and not notarized. If macOS blocks the executable, follow [Apple's individual-app instructions](https://support.apple.com/en-gb/102445). Keep system-wide security protections enabled.

## Upgrade or uninstall

```sh
brew upgrade refactor-me
```

An upgrade changes the CLI used by every project. Repeat the non-live doctor check after upgrading. Maintain global Skills separately, between runs.

```sh
brew uninstall refactor-me
```

Uninstalling keeps project settings, reports, worktrees, result branches and global Skills.
