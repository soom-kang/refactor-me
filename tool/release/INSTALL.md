# Install refactor-me

[한국어](https://github.com/soom-kang/refactor-me/blob/main/tool/release/INSTALL.ko.md) · [Usage guide](https://github.com/soom-kang/refactor-me/blob/main/tool/TUTORIAL.md)

The public Beta supports macOS Apple Silicon. Use Homebrew for a shared executable and install sharpen-me Skills separately.

The current public release is `0.10.0-beta.1`. Candidate `0.10.0-beta.2` has not been published; its model and effort options are described in the usage guide for pre-release review.

## Already extracted an archive

If you are reading the `INSTALL.md` enclosed in an archive, use that archive's executable. Skip the Homebrew and public-download steps below. From its extracted directory:

```sh
cat BUILD-INFO.txt
./refactor-me version --json
```

Compare the enclosed `version` and `commit` with the JSON output; both must match. The executable must identify itself as `refactor-me`, with platform `darwin` and architecture `arm64`, also matching `BUILD-INFO.txt`. Stop if any value differs. Keep it outside the target repository and use its absolute path for later commands.

A locally built `0.10.0-beta.2` candidate has its own enclosed version and commit. These do not make it a published release. The download example below describes the separate public `0.10.0-beta.1` archive. For that version, follow the [tagged public guide](https://github.com/soom-kang/refactor-me/blob/v0.10.0-beta.1/tool/TUTORIAL.md); only a verified beta.2 candidate can use the new model/effort options in the [candidate guide](https://github.com/soom-kang/refactor-me/blob/main/tool/TUTORIAL.md).

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

If Homebrew reports `0.10.0-beta.1`, follow the [tagged public usage guide](https://github.com/soom-kang/refactor-me/blob/v0.10.0-beta.1/tool/TUTORIAL.md), rather than the candidate's model/effort commands. Select a clean Git repository, run the non-live doctor check and set limits before starting `run`. Default doctor checks and `run` call models and consume provider usage.

## Standalone download

For `0.10.0-beta.1`, create a new empty directory and download the archive and checksum file from the [GitHub release](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.1). In that directory:

```sh
release_url=https://github.com/soom-kang/refactor-me/releases/download
archive=refactor-me_0.10.0-beta.1_darwin_arm64.zip
curl -fLO "$release_url/v0.10.0-beta.1/$archive"
curl -fLO "$release_url/v0.10.0-beta.1/SHA256SUMS"
awk -v file="$archive" '$2 == file {print}' SHA256SUMS | shasum -a 256 -c -
```

Proceed only when the checksum reports `OK`. Extract into the empty directory, then inspect the binary:

```sh
unzip "$archive"
./refactor-me version --json
```


Use the extracted executable by absolute path with the same commands as the Homebrew CLI. Keep it outside the target repository. Check version `0.10.0-beta.1`, platform `darwin`, architecture `arm64` and commit `cf31fdf797a68d6bf5cab8a0f22eff9b55766071` before running it. This is the commit behind the [release tag](https://github.com/soom-kang/refactor-me/tree/v0.10.0-beta.1).

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
