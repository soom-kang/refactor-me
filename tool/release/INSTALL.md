# Install refactor-me `0.9.20-beta.1` on macOS Apple Silicon

This public Beta is unsigned and has not been notarized by Apple. Download it
only from the `v0.9.20-beta.1` GitHub release. Check the archive hash against
the release's `SHA256SUMS` before extracting it. The hash detects a changed
download; it does not independently authenticate the publisher.

```sh
shasum -a 256 -c SHA256SUMS
unzip refactor-me_0.9.20-beta.1_darwin_arm64.zip -d refactor-me-release
./refactor-me-release/refactor-me version --json
./refactor-me-release/refactor-me install /path/to/target-repo
```

The installed binary is `.refactor/bin/refactor-me`. Configuration and earlier
run reports remain under `.refactor/`. `doctor` writes a diagnostic run record
and makes provider calls unless `--no-live-probe` is supplied. The target
repository also needs Git, authenticated provider CLIs, its own validation
tools, and the committed eight Skill files described in the project README.
Install and commit those Skills before running
`./.refactor/bin/refactor-me doctor --no-live-probe` from the target repository.
Run `doctor` without that flag to verify live provider access; it uses account
quota.

macOS may block an unsigned download. Verify the source and hash first. If you
choose to run it, follow Apple's per-app Privacy & Security guidance:
https://support.apple.com/en-gb/102445 . Do not disable Gatekeeper globally.

To update, verify a newer release archive and run its `install` command for the
same repository. To remove this Go installation, run the extracted binary's
`uninstall /path/to/target-repo`; configuration, runs, and result branches stay.
For a return to the previous Node CLI, follow the collision-checked rollback
procedure in the `v0.9.20-beta.1` guide at
https://github.com/soom-kang/refactor-me/blob/v0.9.20-beta.1/tool/TUTORIAL.md#update-and-remove .
