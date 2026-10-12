# UI tour screenshot refresh

Captured on 12 October 2026 from the UI at
`44f94c5301050d295b3a5b5eb70619b1e45b2dad`. The binary was built locally
with Go 1.27.2 and the production Vite build. No application runtime code changed.

All 84 existing WebP screenshots were refreshed. Twelve images were added,
covering six views in light and dark: phone sign-in, Eli chat, personal provider
settings, PR repair history, repair readiness and repository repair setup.
The desktop sign-in now shows the current full logo on its black surround.

Eli chat uses a personal fake provider and the real `fleet_status` lookup.
The fake model repeats the lookup result; it is not a sample of a hosted
model's reasoning. Repair history contains two labelled sample outcomes,
written through the store API while the disposable screenshot controller is
stopped. Both are terminal before restart. No repair was executed, no real
model was called and no GitHub PR was changed.

Reproduce with `make screenshots`. The capture harness includes all six new
views, creates the personal demo provider and isolates the repair fixture from
the ordinary fleet captures. Lossless WebP compression uses a lower effort
setting to make full refreshes practical without changing pixels.

Validation:

- Production UI build, screenshot fixture build and fixture `go vet` passed.
- `go test -count=1 ./internal/docs/` passed.
- `mkdocs build --strict` passed.
- Screenshot script passed Prettier, ESLint and the Node syntax check.
- All 96 WebP images decoded successfully; the 84 refreshed gallery images
  were compared with their original PNG captures and preserve every pixel.
- The UI tour and Eli guide passed 16 browser checks: light and dark at
  320, 390, 768 and 1440 pixels, with no horizontal overflow, correct theme
  images, successful image decoding and working tour shortcuts.
- All 162 local article links, anchors and image references on those pages
  resolved. The Overview description matches across the tour, landing page
  and README for screen-reader users.
