# Component gallery captures

14 October 2026. These are captures of the development-only component gallery
that [roadmap/archive/theme-implementation-plan.md](../../../roadmap/archive/theme-implementation-plan.md) asks
for under N-T4, not captures of a product page.

They are not part of the product and not the docs screenshot gallery: the
gallery document (`web/gallery.html`) is never built, never embedded in the
binary and never linked from the navigation. What it renders is the real
components from `web/src/lib/components`, in the states a healthy fleet never
shows: disabled and refused controls, loading, empty, failed, disconnected and
retry states, the status vocabulary, focus rings and the four states of a data
grid.

| | Classic | Mission Control |
| --- | --- | --- |
| Light, 1440px | [Preview](gallery-classic-light-1440.webp) | [Preview](gallery-mission-control-light-1440.webp) |
| Dark, 1440px | [Preview](gallery-classic-dark-1440.webp) | [Preview](gallery-mission-control-dark-1440.webp) |
| Light, 375px | [Preview](gallery-classic-light-375.webp) | [Preview](gallery-mission-control-light-375.webp) |
| Dark, 375px | [Preview](gallery-classic-dark-375.webp) | [Preview](gallery-mission-control-dark-375.webp) |

Refresh them with:

```sh
make screenshots-gallery
```

The script starts the gallery's Vite dev server, photographs it at 1440×900 and
375×900 in both styles and both colour modes, and stops the server again. It
needs Python 3 with Pillow, as `make screenshots` does. `web/tests/gallery.spec.ts`
is the automated half: it checks every combination, the refusal reasons and
focus rings, live contrast of status badges and state labels, the drawer and
confirmation focus order, all four grid states and the absence of page-level
overflow at 1440 and 375 pixels.

Open the live page with `make gallery` and browse to
`http://127.0.0.1:5173/gallery.html?style=mission-control&mode=dark`. The
`style` and `mode` query parameters choose the combination; an unknown value
falls back to Classic and System. The switcher on the page changes the same
combination in place and writes no browser preference.
