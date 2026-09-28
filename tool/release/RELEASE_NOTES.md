# refactor-me 0.9.20-beta.1 — Go CLI public Beta

This is the first Go CLI Beta for **macOS Apple Silicon**. The repository-scoped
installer places one executable at `.refactor/bin/refactor-me`. Existing
`.refactor/config.json`, `commands.json`, and run reports remain readable.

The Go CLI provides `run`, `doctor`, `report`, `clean`, `help`, `version`,
`install`, and `uninstall`. `version --json` now reports `go` and `arch` instead
of the Node runtime field. A new run writes schema version 2; older reports are
read without rewriting their JSON. Git, an authenticated Codex or Claude CLI,
the eight committed sharpen-me Skills, and the target project's validation
tools are still required.

## Download and verification

Download `refactor-me_0.9.20-beta.1_darwin_arm64.zip` and `SHA256SUMS` from this
release. In the download directory, run `shasum -a 256 -c SHA256SUMS` before
extracting the archive. The archive includes `INSTALL.md` with installation
steps. The checksum checks for a changed download; it does not independently
authenticate the publisher.

**This Beta is unsigned and has not been notarized by Apple.** macOS may warn
or block execution. Read the [installation guide](https://github.com/soom-kang/refactor-me/blob/v0.9.20-beta.1/tool/TUTORIAL.md)
and [Apple's guidance for opening an individual app](https://support.apple.com/en-gb/102445)
before choosing to run it. Do not disable system-wide macOS protections.

## Updating and recovery

`install` replaces a recognized Node shim or owned Go binary. It preserves
unverified legacy files and reports their paths. `uninstall` removes only the
owned Go executable and its marker; configuration, runs, worktrees, and result
branches remain. The installation guide documents a collision-checked return
to the earlier `v0.8.8-beta.1` Node CLI.

This Beta does not include Intel, Linux, or Windows binaries. It is an
unattended refactoring tool: review its proposed changes and local result
branch before merging them.
