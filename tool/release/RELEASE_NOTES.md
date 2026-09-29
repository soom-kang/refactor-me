# refactor-me 0.10.0-beta.1 — Homebrew and global Skills

Release candidate; publication is pending verification and approval. This Beta targets macOS Apple Silicon and introduces a shared Homebrew executable and global sharpen-me Skills.

## Installation and execution

After publication:

```sh
brew install soom-kang/refactor-me/refactor-me
npx skills add \
  https://github.com/soom-kang/sharpen-me/tree/v0.9.0-beta.2 \
  --global --skill '*' --agent codex claude-code
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo --no-live-probe
refactor-me run --repo /path/to/target-repo
```

The formula builds pinned source with Go as a build dependency. Runtime prerequisites remain Git, authenticated provider CLIs and the target's validation tools. The external Skills installer requires Node.js; refactor-me does not. Configure run limits before the first model run. Doctor's default live checks and `run` consume provider usage.

## Changes to existing usage

- Use `--repo` from any directory, or run inside the selected repository. With `--repo`, relative `--target` paths start at its Git root; otherwise they start at the calling directory.
- No arguments display help. Start automation with `run`. Optional `init` creates configuration without overwriting it.
- The eight global Skills resolve from `~/.agents/skills`. Claude receives a dedicated copy through `--add-dir` and retains project-only settings. Conflicting sources and unexpected Skill changes block publication.
- Project-local `install` and `uninstall`, Node migration and rollback are removed. Existing local binaries and data are not automatically removed.
- Configuration schema 2 and report schema 3 are required; unsupported old formats are rejected without rewriting or deleting them. Save older records separately if needed.
- `brew upgrade` changes the executable for every project. `brew uninstall` retains configuration, runs, branches, worktrees and Skills.

## Distribution and validation

The Beta is unsigned and not notarized. macOS can warn or block downloaded binaries. Checksums verify download integrity and do not authenticate the publisher. Read the [installation guide](INSTALL.md) and [Apple's individual-app opening instructions](https://support.apple.com/en-gb/102445).

The release review must record local Go and formula checks, source archive hash, commit, CI results and separate Codex/Claude live checks. Those checks are not established by this candidate document. Existing published tags and assets remain unchanged.
