// The one place refactor-me's own version is written down.
//
// Why it lives in src/: install.mjs copies src/ and bin/ into
// <repo>/.refactor/lib and nothing else. A version file anywhere above that —
// a package.json, a VERSION file at the tool root — is invisible to the thing
// people actually run.
//
// Why not `git describe`: an installed copy sits INSIDE the repository it
// refactors, so any git call rooted at import.meta.dirname reads the target
// project's tags and confidently reports someone else's version. `--version`
// also has to work outside a git repository at all.
//
// Why not package.json: shipping one would put a manifest into every target
// repository, where `find . -name package.json`, IDE workspace detection and
// npm itself would see it. "refactor-me은 npm 패키지가 아닙니다" is a promise
// the install contract makes to the target repo, not a stylistic preference.
//
// Bumping it: edit this line, add the matching section to CHANGELOG.md, then
// tag `v<version>`. test/version.test.mjs fails if the first two drift.

export const VERSION = '0.8.8-beta.1';
