# Landing page and documentation review

Reviewed on 12 October 2026 against checkout `c2a0a819`, with the accompanying
documentation changes. This is a source and locally rendered site review;
it does not measure the behaviour of visitors to the deployed site.

## Assessment

Zoomies already has unusually detailed operational documentation, real UI
screenshots, useful problem explanations, self-hosting trade-offs and explicit
platform evidence. The main opportunity is making that material easier to enter
and use. The previous home page made readers work through a large feature list
before choosing a journey. The navigation mixed evaluation pages, installation
methods, optional features and reference. The quick start explained important
implementation details but gave a beginner too little help deciding whether
each step had succeeded.

The refresh centres on three adoption opportunities: a first trusted job,
controlled migration of existing workflows, and operating a larger fleet with
measurements. Private host capacity remains visible as a differentiator.
No calendar campaign, discount, guaranteed saving or unsupported performance
claim is introduced.

## Scope and evidence

- Inventoried the MkDocs navigation and its documentation destinations, theme
  configuration, metadata hooks, screenshots and responsive styles.
- Closely reviewed the landing page, quick start, UI tour, hosts and pools,
  configuration, CLI and API entry points, troubleshooting and queued-job paths.
- Cross-checked tuning guidance with elastic CPU, elastic memory, caches,
  automatic pools, metrics, security, recovery and upgrade documentation.
- Reviewed evaluation and support claims in costs, FAQ, home-lab guidance,
  alternatives entry points and the support matrix.
- Checked installer pool creation in `internal/installer/installer.go`, the
  pool editor defaults and backend rules in `web/src/lib/pools/draft.ts`, and
  the UI-page and runner-image documentation guards.

This does not claim a line-by-line audit of every API route, configuration row
or generated catalogue. Existing exact reference material remains the source
for those contracts. No live fleet, GitHub credentials or paid service was
used. Pricing statements were not independently refreshed from live sources.

## Findings and changes

| Priority | Finding and effect | Change |
| --- | --- | --- |
| High | The home page presented eleven feature cards, long migration explanations and repeated installation prompts. A newcomer had to work out a starting point. | Reduced the rendered article from 2,112 to 1,235 words, about 42%. Added basic, migration and advanced paths before the product screenshot, six concise benefit cards and an explicit first-job action. |
| High | There was no concise explanation separating the controller, agent, host, pool and runner. Misunderstanding pool labels and host selectors can leave a first job queued. | Added `docs/fundamentals.md` with definitions, a job-flow diagram, a complete manual test workflow, label examples and capacity arithmetic. |
| High | Seventeen top-level navigation entries mixed lifecycle tasks and audiences. Eli appeared twice. | Reorganised into nine entries: Home, Documentation, Get started, Run your fleet, Scale and tune, Assistants and repository checks, Reference, Choosing Zoomies and About. Every previous documentation destination remains present. |
| High | The quick-start introduction said everything after installation happened in the browser, while the native path completed account, App and pool setup in the terminal. | Explained both deployment paths, where to open the UI and how to check the GitHub installation and pool before triggering work. |
| High | A fixed example label and a four-CPU default-pool claim could be copied onto a different host or a pool created by the editor. | Told readers to copy their own pool label. Reframed the table as an Ubuntu amd64 Docker example with four configured slots, rather than promising that every four-CPU host gets it. |
| High | Landing-page memory copy implied that a failed build would be saved; CPU copy claimed a four-minute job would finish in two. Neither result is guaranteed. | Replaced these with conditional benefits tied to headroom and workload. Explained that memory elasticity can still miss a sudden allocation and that exit 137 alone does not establish an out-of-memory kill. |
| Medium | Tuning pages said every new pool began observing, despite the native installer's first pool leaving burst policies off and the editor restricting CPU elasticity by backend and sizing. | Distinguished editor defaults from installer defaults, observation from automatic changes, and CPU lending from host-pressure throttling. |
| Medium | Windows evidence conflicted: the home page reported a manual host-and-job trial, while costs said none had happened and FAQ described it as wholly future work. | Aligned costs, FAQ and home-lab copy with the support matrix: builds and unit tests, a reported 11 October manual trial, and written qualification still pending. |
| Medium | Advanced features were individually documented but lacked an order for adopting them. | Added `docs/advanced.md`: baseline, placement and capacity, observation before tuning, monitoring, recovery and scoped automation. Each step links to exact references and states relevant limits. |
| Medium | Configuration, hosts and pools, and the API are large lookup pages with no short task-oriented entry list. | Added compact jump tables or links at their openings. The detailed reference, generated content and established headings remain intact. |
| Medium | Troubleshooting began with a native service log command that a Compose operator or fleet user might not be able to use. | Started with UI and authenticated CLI reads, then distinguished native logs, Compose logs and the information to share with the instance operator. |
| Medium | Permissions detail occupied a substantial part of the basic setup path before the reader reached the first pool. | Kept optional-access decisions visible and placed exact grants and their consequences in an expandable section. No permission requirement was removed. |
| Medium | The home page hid primary navigation, leaving no prominent route into documentation as a whole. | Added a Documentation link in the hero and a `start-here.md` hub with evaluation, first-job and advanced paths, task destinations and success checks. |
| Low | Rewording feature headings would remove existing public deep links. | Preserved the old home-page heading anchors, including feature anchors, while giving the refreshed sections clearer wording. |

## Information and interaction design

The landing page answers what Zoomies does, who it suits and what to do next.
The documentation hub helps a reader choose a task. Fundamentals explains the
mental model. Quick start is a tutorial with an observable outcome. Run your
fleet contains operational guides; Scale and tune contains optional features;
Reference supplies exact settings, commands and contracts.

The new path cards reuse the site's typefaces, surfaces, borders and accent
tokens. They become one column on smaller screens, have visible keyboard-focus
outlines and give standalone action links a 44px minimum height. Hero
prerequisite text is larger. The refresh introduces no browser dependency or
new animation, and retains self-hosted fonts, theme-specific screenshots and
reduced-motion behaviour.

The detail is still available to advanced readers. Finite pool maxima are
distinguished from actual host capacity, runner scaling from VM provisioning,
workflow labels from host selectors, CPU loans from memory loans, liveness
from readiness, and database backups from their separate encryption key.
The automation contract remains explicitly proposed.

## Validation

- Baseline and refreshed sites built with the pinned requirements using
  `mkdocs build --strict`. Existing non-navigation development pages still
  produce informational inventory messages, as before.
- Parsed rendered links on the home page, documentation hub, fundamentals,
  advanced guide, quick start, configuration, hosts and pools, and API entry
  pages: 1,961 local link and anchor checks, with no missing targets or anchors.
- Compared the previous and refreshed home page's public IDs: none removed.
  Compared navigation destinations: none removed, three guides added.
- Chromium checks on the home page, hub, fundamentals, quick start, advanced
  guide, configuration, hosts and pools, and API reference passed at widths
  320, 390, 768 and 1440, in light and dark themes. No document-level horizontal
  overflow was detected in these 64 page/viewport
  combinations.
- Checked link navigation to fundamentals, opening the permissions
  details and a search for `scheduler.scale_up_delay`.
- Inspected desktop and phone captures. Scrolled before a separate mobile
  capture to verify that the lazy product screenshot loads.
- Checked the existing fourteen-page claim and published runner-image names
  against their documentation guards' expectations; `git diff --check` passed.

The environment has no Go compiler, so the repository's Go documentation
guards were inspected but not executed. This change does not modify Go or
product UI code. The local build cannot retrieve GitHub header facts, and the
browser's external Mermaid request to `unpkg.com` is blocked by the environment
policy. Diagram markup builds, but live diagram rendering was not verified.
Link navigation works; Material's instant-navigation lifecycle was not verified
on the local origin, whose address differs from the configured production URL.
These limits must not be mistaken for a deployed-site or real-fleet test.

## Further opportunities

| Opportunity | Why it matters | A useful next step |
| --- | --- | --- |
| Break up very large reference material | Configuration has about 28,500 source words, hosts and pools 18,300, and the API 22,500, including tables and code. Entry links help, but a single document is still demanding. | Separate conceptual explanations and operational recipes from exact reference, retaining old URLs and anchors with an explicit compatibility plan. |
| Remove the external diagram runtime dependency | Material loads Mermaid from a third-party CDN. A restricted environment can show diagram source instead of a diagram, as this review observed. | Evaluate a pinned, locally served Mermaid runtime and its payload cost, with light, dark and instant-navigation checks. |
| Consolidate qualification statements | Platform evidence is repeated across several evaluation pages and can drift when a manual trial occurs. | Give each platform a canonical evidence section and link from summary pages; update summaries when the recorded evidence changes. |
| Validate journeys with real users | Shorter copy and clearer paths are design improvements, not evidence of a conversion increase. | Observe a newcomer running the first workflow and an operator diagnosing a queued job. Record completion, wrong-label errors, abandoned steps and time to a useful answer. |
| Keep pricing and comparisons fresh | These are external facts that can change independently of a release. | Recheck against current vendor sources on a stated cadence and retain the date, assumptions and ownership costs. |

The changes are local and reviewable. This work does not publish the site or
change deployment, authentication, fleet configuration or workload behaviour.
