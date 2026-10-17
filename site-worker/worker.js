// The edge of zoomies.sh: content negotiation for agents.
//
// The site is served by GitHub Pages, which cannot read an Accept header, so a
// request that asks for text/markdown is answered here instead of at the
// origin. Every page's Markdown copy is built beside the site (hooks/seo.py
// writes one at the page's address plus index.md), so serving one is a lookup,
// not a conversion: an agent receives the source the page was written in,
// without the navigation, footer and inline icons the HTML carries.
//
// HTML stays the default. Only an explicit text/markdown entry in Accept gets
// Markdown; anything else, including a browser's */*, is passed through to the
// origin untouched.
//
// Every page answer also carries a Link header (RFC 8288) with the relations
// an agent follows to the machine-readable resources: the API Catalog
// (RFC 9727), the plain-text index, and the REST API's description and
// documentation. GitHub Pages cannot set arbitrary response headers, so the
// header is added here.

const MARKDOWN_TYPE = "text/markdown";

// The OpenAPI document lives in the repository, not on this site: the API runs
// on each deployment, so the authoritative machine description is the one in
// the source tree it is generated from.
const OPENAPI_URL = "https://raw.githubusercontent.com/eyupio/zoomies/main/api/openapi.yaml";

export function discoveryLinks() {
  return [
    '</.well-known/api-catalog>; rel="api-catalog"',
    `</llms.txt>; rel="describedby"`,
    `<${OPENAPI_URL}>; rel="service-desc"; type="application/yaml"`,
    '</api-surface/>; rel="service-doc"; type="text/html"',
  ].join(", ");
}

// An estimate, not a count: tokens are the model's unit and four characters is
// the usual order of magnitude for English prose. The header is advisory, and
// a close number is worth more than a tokenizer dependency at the edge.
export function tokenEstimate(text) {
  return Math.ceil(text.length / 4);
}

// Only an explicit text/markdown entry counts, and only when its q does not
// refuse it. A prefix test would match a browser's */* and take the choice of
// format away from the person reading the page.
export function wantsMarkdown(accept) {
  if (!accept) {
    return false;
  }
  for (const entry of accept.split(",")) {
    const [type, ...parameters] = entry.trim().split(";");
    if (type.trim().toLowerCase() !== MARKDOWN_TYPE) {
      continue;
    }
    for (const parameter of parameters) {
      const [name, value] = parameter.trim().split("=", 2);
      if (name.trim().toLowerCase() === "q" && Number(value) === 0) {
        return false;
      }
    }
    return true;
  }
  return false;
}

// The Markdown copy's address, or null where the URL is a file of its own
// kind: an image, the catalog's JSON, a script. Those are not pages, and no
// copy of them exists to find. Nor is a pathname that begins with two slashes
// (the URL parser also reads "/\" that way): resolved against the site it names
// another host.
export function markdownPath(pathname) {
  if (pathname.startsWith("//")) {
    return null;
  }
  if (pathname.endsWith("/")) {
    return pathname + "index.md";
  }
  const file = pathname.split("/").pop();
  if (file.includes(".")) {
    return pathname.endsWith(".html") ? pathname.replace(/\.html$/, ".md") : null;
  }
  return pathname + "/index.md";
}

// The address of a file on this site, or null when the path would leave it.
//
// The path comes from the request, so it is the requester's to choose, and
// "//host/p" resolved against an origin is a reference to host, not a path on
// the origin: new URL(path, origin) would send the worker to any server the
// requester names and have it answer with that server's reply. Concatenation
// cannot change the authority, and the parse that follows is the check that
// it did not.
function onThisSite(origin, path) {
  if (path.startsWith("//")) {
    return null;
  }
  const address = origin + path;
  return new URL(address).origin === origin ? address : null;
}

// The headers of a fetched answer that a response built here may carry. The
// body is re-sent as this site's own, so the cookies, policies and CORS grants
// of whatever answered the fetch are not the worker's to pass on; what is left
// is what lets a cache and a conditional request treat the copy as the file it
// is.
const RELAYED_HEADERS = ["cache-control", "etag", "last-modified"];

function relayedHeaders(upstream) {
  const headers = new Headers();
  for (const name of RELAYED_HEADERS) {
    const value = upstream.headers.get(name);
    if (value !== null) {
      headers.set(name, value);
    }
  }
  return headers;
}

// The Markdown copy as a response, or null when this address has none, in
// which case the origin's answer (including a real 404) stands.
//
// The Vary header is set even though GitHub Pages does not: the worker, not
// the origin, chooses the format, so a cache in front of the origin has to
// know that the choice depends on Accept.
async function markdownResponse(request, url) {
  const path = markdownPath(url.pathname);
  if (path === null) {
    return null;
  }
  const target = onThisSite(url.origin, path);
  if (target === null) {
    return null;
  }
  const copy = await fetch(target, {
    headers: { accept: MARKDOWN_TYPE },
    redirect: "follow",
  });
  if (!copy.ok) {
    return null;
  }
  const body = await copy.text();
  const headers = relayedHeaders(copy);
  headers.set("content-type", MARKDOWN_TYPE + "; charset=utf-8");
  headers.set("vary", "Accept");
  headers.set("x-markdown-tokens", String(tokenEstimate(body)));
  headers.set("link", discoveryLinks());
  return new Response(body, { status: copy.status, headers });
}

// The API Catalog document (RFC 9727) at its well-known address. The origin
// stores it as api-catalog.json, because GitHub Pages picks a content type
// from the extension and would serve an extensionless file as
// application/octet-stream; the type the RFC asks for is set here.
async function apiCatalogResponse(url) {
  const source = await fetch(new URL("/api-catalog.json", url.origin), { redirect: "follow" });
  if (!source.ok) {
    return null;
  }
  const body = await source.text();
  const headers = new Headers(source.headers);
  headers.set("content-type", 'application/linkset+json; profile="https://www.rfc-editor.org/info/rfc9727"');
  headers.set("link", discoveryLinks());
  return new Response(body, { status: source.status, headers });
}

// The identity documents this site can back truthfully are the hosted
// service's own: the portal runs an OpenID Provider whose discovery names a
// real issuer, real endpoints and signing keys, and its authorization-server
// metadata carries the agent_auth block. The mirrors serve each document at
// the site's address for it, exactly as the portal serves it, refetched on
// every request, so a rotation or an added block can never leave a stale copy
// here. Where discovery "should" live is the issuer's own well-known address,
// and the documents say that is where they live: a mirror changes nothing
// inside their bytes, and a client that insists on fetching at the issuer
// itself is pointed there by everything in them. The resource's own metadata
// is not mirrored: it is the site's to declare, and hooks/seo.py writes it.
const ISSUER_ORIGIN = "https://portal.zoomies.sh";
const MIRRORS = [
  { from: "/.well-known/openid-configuration",
    at: ["/.well-known/openid-configuration"] },
  { from: "/.well-known/oauth-authorization-server",
    at: ["/.well-known/oauth-authorization-server"] },
];

// How long the provider has to answer. The documents are small and nearly
// static, so one that has not arrived in this long is not coming, and a client
// waiting on discovery is better told so than held until the platform gives up.
const MIRROR_TIMEOUT_MS = 5000;

// What a client is told when the provider cannot be asked. It is not the
// origin's 404: that says this site has no such document and is cached for
// hours, so an outage at the provider would read as an absence long after the
// provider came back.
function providerUnavailable() {
  return new Response(
    "The identity provider at portal.zoomies.sh is not serving this document " +
      "right now. Nothing was cached, so try again in a minute.\n",
    {
      status: 502,
      headers: {
        "content-type": "text/plain; charset=utf-8",
        "cache-control": "no-store",
      },
    },
  );
}

// The provider's document, null where the provider says it has none (the
// origin's answer then stands), or a 502 where it could not be asked or did not
// answer well: a refused connection, a timeout, a 429 or a 5xx.
async function mirrorResponse(from) {
  let upstream;
  let body;
  try {
    upstream = await fetch(new URL(from, ISSUER_ORIGIN), {
      headers: { accept: "application/json" },
      signal: AbortSignal.timeout(MIRROR_TIMEOUT_MS),
    });
    if (upstream.status === 404) {
      return null;
    }
    if (!upstream.ok) {
      return providerUnavailable();
    }
    body = await upstream.text();
  } catch {
    return providerUnavailable();
  }
  const headers = new Headers(upstream.headers);
  headers.set("content-type", "application/json");
  return new Response(body, { status: upstream.status, headers });
}

export default {
  async fetch(request) {
    if (request.method === "GET" || request.method === "HEAD") {
      const url = new URL(request.url);
      const mirror = MIRRORS.find((m) => m.at.includes(url.pathname));
      if (mirror) {
        const mirrored = await mirrorResponse(mirror.from);
        if (mirrored !== null) {
          return request.method === "HEAD" ? new Response(null, mirrored) : mirrored;
        }
      }
      if (url.pathname === "/.well-known/api-catalog" || url.pathname === "/.well-known/api-catalog/") {
        const catalog = await apiCatalogResponse(url);
        if (catalog !== null) {
          return request.method === "HEAD" ? new Response(null, catalog) : catalog;
        }
      }
      if (wantsMarkdown(request.headers.get("accept"))) {
        const markdown = await markdownResponse(request, url);
        if (markdown !== null) {
          return request.method === "HEAD" ? new Response(null, markdown) : markdown;
        }
      }
      const origin = await fetch(request);
      // A page answer announces where the machine-readable resources are. The
      // markdownPath test is the worker's existing idea of what a page is: it
      // keeps install.sh and the images from wearing links that are not
      // about them.
      if (origin.ok && markdownPath(url.pathname) !== null) {
        const response = new Response(origin.body, origin);
        response.headers.set("link", discoveryLinks());
        return response;
      }
      return origin;
    }
    // A same-zone fetch from a Worker goes to the origin directly, so the HTML
    // answer reaches GitHub Pages exactly as it always did.
    return fetch(request);
  },
};
