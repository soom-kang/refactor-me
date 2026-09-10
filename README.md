![refactor-me](docs/assets/refactor-me-title.png)

# refactor-me

Run behavior-preserving refactors with Codex and Claude Code. Review the resulting local branch and validation report before merging.

[한국어](docs/README.ko.md) · [Guide](tool/TUTORIAL.md) · [Reference](tool/README.md) · [Workflow](tool/WORKFLOW.md) · [Changelog](tool/CHANGELOG.md)

refactor-me audits a repository, selects one candidate, checks its evidence, implements the change in a detached worktree, validates it, and requests an independent review. It commits accepted changes to a local `refactor/auto-*` branch and repeats within the configured limits.

The loop uses the eight skills from [sharpen-me](https://github.com/soom-kang/sharpen-me). This repository contains the CLI that coordinates them. The installed skill files and `skills-lock.json` record the catalog used by this checkout.

## Install

Use macOS, Node.js 24 or later, Git, and at least one authenticated provider CLI: `codex` or `claude`. Install the target project's dependencies and populate its build-tool caches before running. Provider calls require network access.

From this repository checkout, install the tool into the repository you want to refactor:

```bash
node tool/install.mjs /path/to/target-repo
```

Install the skills in the **target repository**, using Project scope:

```bash
cd /path/to/target-repo
npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code
```

Review and commit the installed skill files, agent links, and lockfile before starting a run. Each phase uses a checkout of the base commit, so uncommitted project skills are unavailable there. Claude loads project skills under this tool's isolation settings; a global Claude installation does not satisfy that requirement.

The installer copies the tool into `.refactor/`. It preserves existing configuration and run records. No npm package installation or build is required for the tool itself.

## Run

From the target repository:

```bash
./.refactor/bin/refactor-me version
./.refactor/bin/refactor-me doctor
./.refactor/bin/refactor-me --provider codex --fallback none
```

`doctor` checks prerequisites and writes diagnostic records. Its default provider probes make model calls. Use `doctor --no-live-probe` to skip those calls; that check does not verify session visibility.

To use both providers or narrow candidate discovery:

```bash
./.refactor/bin/refactor-me --provider codex --fallback claude
./.refactor/bin/refactor-me --target app/web
./.refactor/bin/refactor-me --target app/web --target app/api
```

`--target` limits where the audit looks for candidates. A candidate can require changes to callers and tests outside that directory. Reachability searches cover the repository. Baseline validation runs the discovered commands; candidate validation selects changed areas and the root area when present.

A run can create local commits and a result branch. It does not merge, push, or deploy. Keep the source checkout clean during execution.

## Skills in the loop

| Skill | Used when | Result |
| --- | --- | --- |
| `sharpen-clarify` | Audit and characterization need a defined scope | Scope and contracts to check |
| `sharpen-review` | Audit or deep check examines a candidate | Findings backed by repository evidence |
| `sharpen-challenge` | Audit, deep check, characterization, or preflight tests an assumption | Supported objections and checks |
| `sharpen-assess` | Audit assigns change risk | Risk classification; unknown risk excludes a candidate |
| `sharpen-refine` | Execution applies an approved task packet | A bounded change or a justified no-op |
| `sharpen-cold-review` | A separate session reviews the implementation | Review verdict and findings |
| `sharpen-brief` | Provider quota or authentication failure causes a handoff | A snapshot of the transition |
| `sharpen-dedupe` | A duplication candidate reaches deep check or execution | Duplicate analysis and scoped consolidation |

The loop defines each phase's allowed skills, output schema, and permissions. Model and effort settings come from the loop's configuration; skill recommendations do not change them.

## Reports

English is the default. Choose Korean for a run or for viewing a saved report:

```bash
./.refactor/bin/refactor-me --lang ko
./.refactor/bin/refactor-me report
./.refactor/bin/refactor-me report --lang ko
./.refactor/bin/refactor-me report --json
```

Each run writes one `report.md` in the selected language and a language-neutral `report.json`. Reading a report with another language renders the saved JSON without overwriting either file or calling a model. Translated labels cover headings, statuses, and known reasons. Commit subjects, model explanations, commands, and error text retain their original wording.

The code comparison uses the run's recorded start and final published commits. It includes file statistics, a bounded diff preview and a link to `changes.patch`, the full text patch. It includes published characterization tests and excludes rejected edits. See the [Workflow](tool/WORKFLOW.md) for response examples and failure branches.

## Verify

From this repository root:

```bash
node --test tool/test/*.test.mjs
node tool/bin/refactor-me.mjs help
node tool/bin/refactor-me.mjs version --json
```

The tests cover local routing, gates, reporting, installation, and CLI behavior. They do not establish the quality of live provider decisions. See the [reference](tool/README.md) for validation boundaries and the [guide](tool/TUTORIAL.md) for reviewing a result.
