# Install refactor-me

[한국어](https://github.com/soom-kang/refactor-me/blob/main/tool/release/INSTALL.ko.md) · [Usage guide](https://github.com/soom-kang/refactor-me/blob/main/tool/TUTORIAL.md)

The public Beta supports macOS Apple Silicon. Use Homebrew for a shared executable and install sharpen-me Skills separately.

## Homebrew

Prepare Git, an authenticated Codex or Claude Code CLI, and the target project's validation tools. Node.js is needed for the Skill installer, not refactor-me itself.

```sh
brew install soom-kang/refactor-me/refactor-me
refactor-me version --json
npx skills add soom-kang/sharpen-me \
  --global --skill '*' --agent codex claude-code
```

Homebrew builds fixed release source with Go as a build dependency. Required Skills resolve from `~/.agents/skills`. Homebrew does not install Skills or write project configuration.

Continue with the [usage guide](https://github.com/soom-kang/refactor-me/blob/main/tool/TUTORIAL.md): select a clean Git repository, run the non-live doctor check and set limits before starting `run`. Default doctor checks and `run` call models and consume provider usage.

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
brew upgrade soom-kang/refactor-me/refactor-me
```

An upgrade changes the CLI used by every project. Repeat the non-live doctor check after upgrading. Maintain global Skills separately, between runs.

```sh
brew uninstall refactor-me
```

Uninstalling keeps project settings, reports, worktrees, result branches and global Skills.
