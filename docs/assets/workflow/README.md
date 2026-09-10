# Workflow diagrams

The two Workflow editions embed three figures each. Each HTML file is the editable source; its PNG is a browser export of the inline SVG, not a Mermaid rendering.

## Files and reproduction

| Figure | English | Korean |
| --- | --- | --- |
| Overview | [HTML](overview.en.html) · [PNG](overview.en.png) | [HTML](overview.ko.html) · [PNG](overview.ko.png) |
| Preparation | [HTML](preparation.en.html) · [PNG](preparation.en.png) | [HTML](preparation.ko.html) · [PNG](preparation.ko.png) |
| Outcomes | [HTML](outcomes.en.html) · [PNG](outcomes.en.png) | [HTML](outcomes.ko.html) · [PNG](outcomes.ko.png) |

[Original English Mermaid](original.en.mmd) · [Original Korean Mermaid](original.ko.mmd)

The originals preserve the graph before the redraw. Stable node IDs and `data-edge` values in HTML tie the figures to the ledger below. Edge IDs follow the declaration order in the originals. Update both languages together when the controller's behavior changes.

Render HTML with Chromium through an existing Playwright installation. Wait for `document.fonts.ready`, check the requested font faces loaded, set the SVG to 960 × 600 CSS pixels, and capture only the SVG at device scale factor 2. PNG dimensions must be 1920 × 1200. Keep the SVG's cream background rect while omitting the surrounding browser background. The HTML uses no scripts or external images. Google Fonts is its only external resource; offline font substitution can change the HTML's appearance. PNG retains the exported type.

Run `python3 <diagram-design-skill>/scripts/self_check.py <html>` for each source. Inspect connector routing, label clearances, font loading and actual document-width legibility in a browser as well; the installed self-check does not validate geometry. The skill's repository-only geometry script is not included in this installation.

## Project style

These are project-local settings approved for this redraw. Do not copy them over the installed Skill or create a home-directory profile as part of diagram maintenance.

| Role | Value | Basis |
| --- | --- | --- |
| Paper | `#FDF7EC` | Representative background pixel from the title image |
| Ink | `#050505` | Representative black pixel from the title image |
| Accent | `#DE5C20` | Representative orange pixel from the title image |
| Muted text and arrows | `#625B52` | Derived warm neutral; contrast 6.28:1 on paper |
| Secondary surface | `#F4EEE3` | Derived cream |
| Accent surface | `#FAE8DB` | Derived pale orange |
| Hairline | `#CFC7BB` | Derived warm neutral |
| Link arrow | `#625B52` | No external-network path needs a second hue |
| Headings and labels | Geist 400/500/600, Noto Sans KR 400/500/600 | Approved substitutes; the image does not establish a font family |
| Technical identifiers | Geist Mono 400/500 | Technical labels only |

Ink contrast on paper is 19.11:1. Accent contrast is 3.48:1, so orange is reserved for focus strokes and surfaces; text remains ink or muted. The font stylesheet requests Geist, Geist Mono and Noto Sans KR from Google Fonts. `Apple SD Gothic Neo` and generic sans-serif are offline fallbacks, not claims of an exact brand match.

All figures use a 960 × 600 viewBox and a static light layout. Main labels use 16–20 px, supporting text uses 12 px, and the title uses 28 px. Korean text stays at 12 px or above. Connectors use rounded orthogonal bends of 8 px, distinct node attachment points and opaque label masks. One node per figure carries the orange focus. No gradients, shadows, animation or decorative icons are used.

## Fidelity ledger

The source has **16 nodes and 26 directed edges**. The three figures retain the full graph through shared node IDs and the preparation aggregate. No source node or relationship is dropped. Figure 01 collapses `D`, `C`, `T`, `TC`, `P` into **Prepare**; Figure 02 expands them. Figure 03 repeats execution, gate and review as context for their rejection routes. The overview repeats edge e09 from preparation; outcomes repeat e10/e11 from overview. These repetitions are the same transitions, not extra execution steps.

| Source node | Meaning | Figure(s) |
| --- | --- | --- |
| `I` | INIT and doctor | 01 |
| `B` | Baseline | 01 |
| `A` | Audit and rank | 01, 03 |
| `D` | Deep check | 01, 02 |
| `C` | Characterization needed and enabled | 01, 02 |
| `T` | Test-only change and validation | 01, 02 |
| `TC` | Commit and publish tests | 01, 02 |
| `P` | Preflight | 01, 02 |
| `E` | Execute | 01, 02, 03 |
| `G` | Diff gate and validation | 01, 03 |
| `R` | Independent review | 01, 03 |
| `K` | Commit and publish refactor | 01 |
| `S` | Record candidate outcome | 02, 03 |
| `X` | Roll back candidate edits | 03 |
| `F` | Final report and code comparison | 01, 03 |
| `H` | Safety halt and retain evidence | 03 |

| Edge | Source → destination | Condition | Figure(s) |
| --- | --- | --- | --- |
| `e01` | `I` → `B` | Continue | 01 |
| `e02` | `B` → `A` | Continue | 01 |
| `e03` | `A` → `D` | Continue | 01 |
| `e04` | `D` → `C` | Continue | 02 |
| `e05` | `C` → `T` | Yes | 02 |
| `e06` | `T` → `TC` | Continue | 02 |
| `e07` | `TC` → `P` | Continue | 02 |
| `e08` | `C` → `P` | No | 02 |
| `e09` | `P` → `E` | Continue | 01, 02 |
| `e10` | `E` → `G` | Continue | 01, 03 |
| `e11` | `G` → `R` | Continue | 01, 03 |
| `e12` | `R` → `K` | Continue | 01 |
| `e13` | `K` → `A` | Continue | 01 |
| `e14` | `D` → `S` | Not ready | 02 |
| `e15` | `P` → `S` | Blocked | 02 |
| `e16` | `T` → `S` | Rejected | 02 |
| `e17` | `E` → `X` | Rejected | 03 |
| `e18` | `G` → `X` | Rejected | 03 |
| `e19` | `R` → `X` | Rejected | 03 |
| `e20` | `X` → `S` | Continue | 03 |
| `e21` | `S` → `A` | Continue | 03 |
| `e22` | `A` → `F` | No eligible candidates or limit | 01 |
| `e23` | `I` → `F` | Blocking prerequisite | 01 |
| `e24` | `B` → `F` | No passing signal | 01 |
| `e25` | `G` → `H` | Unsafe invariant | 03 |
| `e26` | `H` → `F` | Continue | 03 |

## Reading the split

- **01:** success, re-audit and early termination. `STOP` abbreviates the source condition “No eligible candidates or limit,” also written in the audit node and footer. Preparation failures are expanded in 02; later rejections are expanded in 03.
- **02:** readiness and preflight can stop a candidate before implementation. Tests, when needed and enabled, must pass against unchanged production code before their separate commit. The dashed test-rejection route still ends at the source outcome node `S`; restoring rejected test edits follows the accompanying Workflow prose.
- **03:** implementation, gate and review rejection restore candidate edits, record their outcome and return to audit. A published characterization commit survives that restoration. An unsafe gate reaches `H`, preserves its worktree as evidence and ends at `F` rather than taking the ordinary rollback route.

The original Mermaid's “No” characterization branch means “not both needed and enabled.” It does not claim that tests are unnecessary when automatic characterization is disabled. The diagrams preserve that condition.
