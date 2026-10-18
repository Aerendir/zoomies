# The edge of zoomies.sh

The site is served by a Cloudflare Worker from the deployment's static assets,
the directory the docs build writes (declared in `wrangler.toml`). Nothing in
the chain dials an origin server, so no origin resolution can take a page or a
discovery document dark; GitHub Pages is no longer part of the serving path.

What the assets pipeline cannot do itself, the worker does around it:

* Answers a request carrying `Accept: text/markdown` with the page's Markdown
  copy, at `Content-Type: text/markdown`, with `Vary: Accept` and an
  `x-markdown-tokens` estimate. Every other request, including a browser's,
  is served from the assets with its body untouched.
* Every page answer carries a `Link` header (RFC 8288) with the relations an
  agent follows: `api-catalog` to the API Catalog (RFC 9727), `describedby` to
  `llms.txt`, and `service-desc` and `service-doc` to the REST API's OpenAPI
  document and its human-facing page.
* `/.well-known/api-catalog` is served from `api-catalog.json`, with the
  `application/linkset+json` content type the RFC asks for. A file's
  extension picks the asset's content type and cannot say this one.
* `/.well-known/oauth-protected-resource` is served from
  `.well-known/oauth-protected-resource.json`, with the `application/json`
  type RFC 9728 requires, for the same reason.
* The hosted service's OpenID Provider discovery document is mirrored at
  `/.well-known/openid-configuration` and `/.well-known/oauth-authorization-server`,
  byte for byte from `https://portal.zoomies.sh` and refetched on each
  request, so the site's identity document is always the one the issuer
  serves. A portal that has no such document (404) leaves the not-found page
  standing; one that cannot be reached, does not answer within five seconds or
  answers 429 or 5xx gets a `502` with `Cache-Control: no-store`, because a
  404 is cached for hours and would turn an outage into "this domain has no
  OAuth metadata".
* Every answer carries the browser-hardening headers a static origin cannot
  set: `Strict-Transport-Security` (one year, deliberately without `preload`
  or `includeSubDomains`), `X-Content-Type-Options`, `X-Frame-Options`,
  `Referrer-Policy`, `Permissions-Policy` and a Content Security Policy. The
  policy is sent as `Content-Security-Policy-Report-Only` until the built site
  has been browsed with the console open and shown no violation; then
  `CSP_HEADER` in `worker.js` becomes `content-security-policy`.

The copies it serves are the ones `hooks/seo.py` writes beside the built site,
at every page's address plus `index.md` (the address the pages already declare
as their `rel="alternate"` link). Serving a copy is a lookup rather than an
HTML-to-Markdown conversion, and what an agent receives is the source the page
was written in.

## Why a worker

The assets pipeline cannot read an Accept header, cannot set arbitrary
response headers and cannot mirror another host's document, so the choice of
format and the discovery surface live in the worker in front of them. Serving
from the assets rather than an origin also takes the origin's availability
out of the site's: a build ships whole or not at all.

## Test

    node --test site-worker/worker.test.mjs

The tests run the handler against an in-memory asset binding, so nothing
leaves the machine. `.github/workflows/docs.yml` runs them on every site
change.

## Deploy

1. Create an API token with Workers edit rights, and the account id.
2. Deploy from this directory (the assets directory is `../site`, the docs
   build's output):

       CLOUDFLARE_API_TOKEN=... npx wrangler deploy

The zone's DNS records stay proxied; the worker route answers before any
origin would be consulted, and CI deploys the same way from the repository's
`CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` secrets.

## Verify

    curl -s https://zoomies.sh/quickstart/ \
      -H "Accept: text/markdown" | head -3
    curl -sI https://zoomies.sh/quickstart/ \
      -H "Accept: text/markdown" | grep -i "content-type"
    curl -sI https://zoomies.sh/ | grep -i "^link"
    curl -s https://zoomies.sh/.well-known/api-catalog
    curl -s https://zoomies.sh/.well-known/openid-configuration
    curl -sI https://zoomies.sh/.well-known/oauth-protected-resource | grep -i "content-type"

The second command should say `text/markdown; charset=utf-8`. The third should
name all four relations (`api-catalog`, `describedby`, `service-desc` and
`service-doc`). The fourth should be the linkset JSON, the fifth the portal's
discovery document (its `issuer` must be `https://portal.zoomies.sh`), and the
last should say `application/json`.
Browsers and crawlers that send no markdown header keep receiving the HTML,
with only the `Link` and `Vary: Accept` headers added (the Markdown copy
carries the same `Vary`, after whatever the assets already vary on).
