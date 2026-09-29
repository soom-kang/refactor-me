# Edit the documentation diagrams

The user guides and workflow pages share two diagrams, each in English and Korean. Edit the HTML source, update its standalone SVG, then export the PNG used by GitHub Markdown.

## Figures

| Diagram | English | Korean |
| --- | --- | --- |
| Global installation and project state | [HTML](installation.en.html) · [SVG](installation.en.svg) · [PNG](installation.en.png) | [HTML](installation.ko.html) · [SVG](installation.ko.svg) · [PNG](installation.ko.png) |
| Execution and result inspection | [HTML](execution.en.html) · [SVG](execution.en.svg) · [PNG](execution.en.png) | [HTML](execution.ko.html) · [SVG](execution.ko.svg) · [PNG](execution.ko.png) |

The installation figure separates shared executable/Skills from repository-owned configuration and records. Its arrows mean repository selection, not copying the executable. Worktree storage is noted separately.

The execution figure summarizes user actions and exit-code handling. It does not enumerate internal retries or every phase gate; the [workflow](../../../tool/WORKFLOW.md) explains those details. Exit `0` includes partial completion. Exits `2` and `4` require diagnostics; `4` preserves the worktree.

## Style

Reuse the [title image](../refactor-me-title.png) palette. These tokens belong to this project; do not change an installed Skill or a home-directory profile.

| Role | Value |
| --- | --- |
| Paper / ink | `#FDF7EC` / `#050505` |
| Accent / accent surface | `#DE5C20` / `#FAE8DB` |
| Muted text / hairline | `#625B52` / `#CFC7BB` |
| Type | Geist, Geist Mono, Noto Sans KR |
| Offline fallback | Apple SD Gothic Neo, sans-serif, monospace |

Use orange for one focal node, with dark text. Keep layouts static and connectors orthogonal. The installation canvas is 960 × 650; execution is 960 × 760. PNG exports use scale 2. Google Fonts supplies web fonts; offline SVG/HTML viewers may substitute them. PNG preserves the rendered text.

## Export and check

1. Update the HTML and matching SVG. Keep the SVG's first-child title, description and unique `aria-labelledby` IDs.
2. Open the HTML with an existing Playwright/Chromium installation. Wait for `document.fonts.ready` and confirm Geist and Noto Sans KR loaded.
3. Capture only the SVG at scale 2, keeping its paper background. Export installation at 1920 × 1300 and execution at 1920 × 1520.
4. Inspect both languages at document width. Check labels, arrow endpoints and outcome meanings against the text.
5. Run the diagram-design self-check, if installed, and check Markdown image links. Keep both languages synchronized.

An installed diagram-design Skill supplies `scripts/self_check.py`; it checks structure and accessibility, not visual geometry. Browser rendering is a documentation-authoring tool and does not add a Node runtime dependency to the CLI.
