# Frontend browser tests and review tools

The TypeScript Playwright suite owns browser testing, accessibility measurements,
and visual review utilities. It replaces the Python scripts formerly in `dev/`.
There is one npm dependency and browser installation for all of them.

## Setup and automated tests

From `web/` (Node 24, as in CI):

```bash
npm ci
npx playwright install chromium
npm run typecheck:e2e
npm run lint
npm run test:e2e
```

The suite starts a temporary Vite server and mocks release APIs; the backend and
its credentials are not needed. CI installs Chromium with `--with-deps` and runs
these tests. Existing specs cover portfolio navigation, shipment status, source
popover placement, and exact-FBC validation presentation. Accessibility specs
cover cards, stages, briefing, and workbench at 1440px/420px in both themes.

Set `PLAYWRIGHT_BASE_URL` to use an already-running frontend. Specs still mock
API responses. `PLAYWRIGHT_CHROMIUM_EXECUTABLE=/absolute/path/to/chrome` selects
an existing browser for both tests and tools; otherwise use Playwright's pinned
browser. Test outputs go to ignored `web/test-results/`.

`fixtures/portfolio.ts` supplies shared API data. `helpers/` owns browser startup
settings, readiness, screenshots, and accessibility measurement. `tools/` has
standalone commands for a running app; those do not launch the app themselves.

## Screenshots of a running app

```bash
npm run shots -- --width 1440 --width 420 \
  http://localhost:8089 ./test-results/shots / /releases/quay-v3.18.1
npm run shots -- --dark --width 420 \
  http://localhost:8089 ./test-results/shots /releases/quay-v3.18.1
```

Output filenames include the route, theme and width (for example
`overview-light-420.png`). The command prints JSON containing card text, clipping,
page overflow, and console/uncaught errors. It exits nonzero for those failures,
failed page loads, or incorrect theme selection. It still saves screenshots when
possible. PatternFly scrolls `<main>` separately; the shared helper grows the
viewport to include that content, then restores the viewport. It does not silently
return a truncated image if the page fails to stabilize or exceeds 30,000px.

Override selected API responses with JSON files, preserving the old tool's
`GLOB=FILE` option:

```bash
npm run shots -- --route '**/api/v1/releases/quay-v3.18.1/candidate=/tmp/candidate.json' \
  http://localhost:8089 ./test-results/shots /releases/quay-v3.18.1
```

To review the frontend against a local backend, start it with:

```bash
RR_API_TARGET=http://127.0.0.1:8088 npm run dev -- --port 5174 --strictPort
```

Then point `shots` at `http://localhost:5174` and the release route to capture.

## Accessibility diagnostics

```bash
npm run a11y -- --width 1440 --width 420 --theme light --theme dark \
  http://localhost:8089 / /releases/quay-v3.18.1
```

Defaults: 1440px, both themes. The same `--route GLOB=FILE` option is supported.
Reports contain measured text/icon contrast, target size/spacing, horizontal
overflow in the document and PatternFly main, browser errors, and failures.
They also list **unmeasured** nodes (for example complex gradients/opacity).
Unmeasured does not mean passed; this heuristic audit is not a complete WCAG
conformance assessment. The port preserves the existing contrast and target-size
rules. Each automated page audit attaches its full JSON report to Playwright.

The tools use isolated contexts with a fixed locale/timezone and reduced motion,
wait for visible app content, initial API requests, fonts and layout, and avoid
fixed sleeps or network-idle waits that interact badly with polling.

## Annotated comparisons and contact sheets

```bash
npm run compare -- /tmp/comparison.json
```

A before/after spec:

```json
{
	"before": "shots/overview-light-1440.png",
	"after": "shots/overview-dark-1440.png",
	"out": "comparison.png",
	"title_before": "Before",
	"title_after": "After",
	"marks": [{ "n": 1, "side": "after", "x": 40, "y": 120, "w": 300, "h": 80 }]
}
```

Or a contact sheet:

```json
{
	"sheet": [
		{ "img": "shots/overview-light-1440.png", "title": "Light" },
		{ "img": "shots/overview-dark-1440.png", "title": "Dark" }
	],
	"out": "sheet.png"
}
```

**Relative paths inside specs resolve against the spec file's directory**, unlike
the former Python command's working-directory-relative paths. Absolute paths
continue to work. Marks use source-image pixel coordinates and must fit inside
the image. Titles and badge labels are rendered as text. Input images must be PNG;
image decoding is awaited before capture. Several specs can be passed together.
This is a manual review compositor, not a pixel-diff assertion or baseline updater.

Historical design captures remain available in Git history. Only intentionally
reviewed visual baselines should be added to tests.
