// The negotiation rules and the handler's decisions, against an in-memory
// origin. The Markdown copies the worker serves are written by hooks/seo.py,
// so a change to their address or their shape fails here rather than at the
// edge. Plain node:test, so no dependency beyond the binary runs it.

import test from "node:test";
import assert from "node:assert/strict";

import worker, {
  discoveryLinks,
  markdownPath,
  tokenEstimate,
  wantsMarkdown,
} from "./worker.js";

const HTML = "<!doctype html><html><body><p>the page</p></body></html>";
const MARKDOWN = "# The page\n\nIts body as Markdown.\n";

// `seen` collects the address of every fetch the worker makes, so a test can
// say which hosts it was sent to as well as what it answered.
function originFor(paths, seen = []) {
  return async (input) => {
    const url = new URL(typeof input === "string" ? input : input.url ?? input.href);
    seen.push(url.href);
    // Keys are "/path" for this site's origin, or "host/path" for an
    // upstream the mirror fetches by name.
    const found = paths.get(url.host + url.pathname) ?? paths.get(url.pathname);
    if (found === undefined) {
      return new Response(HTML, {
        status: 404,
        headers: { "content-type": "text/html; charset=utf-8" },
      });
    }
    return new Response(found.body, {
      status: found.status ?? 200,
      headers: found.headers ?? { "content-type": found.type },
    });
  };
}

async function handle(url, paths, init = {}, seen = []) {
  const previous = globalThis.fetch;
  globalThis.fetch = originFor(new Map([...paths]), seen);
  try {
    return await worker.fetch(new Request(url, init));
  } finally {
    globalThis.fetch = previous;
  }
}

test("a request that asks for markdown gets the page's copy", async () => {
  const response = await handle(
    "https://zoomies.sh/quickstart/",
    new Map([
      [
        "/quickstart/index.md",
        { type: "text/markdown; charset=utf-8", body: MARKDOWN },
      ],
    ]),
    { headers: { accept: "text/markdown" } },
  );

  assert.equal(response.status, 200);
  assert.equal(
    response.headers.get("content-type"),
    "text/markdown; charset=utf-8",
  );
  assert.equal(response.headers.get("vary"), "Accept");
  assert.equal(
    response.headers.get("x-markdown-tokens"),
    String(Math.ceil(MARKDOWN.length / 4)),
  );
  assert.equal(await response.text(), MARKDOWN);
});

test("the site root's markdown copy is served for the bare host", async () => {
  const response = await handle(
    "https://zoomies.sh",
    new Map([
      ["/index.md", { type: "text/markdown; charset=utf-8", body: MARKDOWN }],
    ]),
    { headers: { accept: "text/markdown" } },
  );

  assert.equal(response.status, 200);
  assert.equal(await response.text(), MARKDOWN);
});

test("a browser's accept header gets the HTML the origin serves", async () => {
  const response = await handle(
    "https://zoomies.sh/quickstart/",
    new Map([
      ["/quickstart/", { type: "text/html; charset=utf-8", body: HTML }],
    ]),
    {
      headers: {
        accept:
          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
      },
    },
  );

  assert.equal(response.status, 200);
  assert.equal(
    response.headers.get("content-type"),
    "text/html; charset=utf-8",
  );
  assert.equal(await response.text(), HTML);
});

test("an address with no markdown copy falls back to the origin's answer", async () => {
  const response = await handle("https://zoomies.sh/not-a-page/", new Map([]), {
    accept: "text/markdown",
  });

  assert.equal(response.status, 404);
  assert.equal(
    response.headers.get("content-type"),
    "text/html; charset=utf-8",
  );
});

test("a file that is a format of its own is never replaced with markdown", async () => {
  const catalog = '{"version": 1}';
  const response = await handle(
    "https://zoomies.sh/catalog.json",
    new Map([["/catalog.json", { type: "application/json", body: catalog }]]),
    { headers: { accept: "text/markdown" } },
  );

  assert.equal(response.status, 200);
  assert.equal(response.headers.get("content-type"), "application/json");
  assert.equal(await response.text(), catalog);
});

test("a HEAD request for markdown answers with the headers and no body", async () => {
  const response = await handle(
    "https://zoomies.sh/quickstart/",
    new Map([
      [
        "/quickstart/index.md",
        { type: "text/markdown; charset=utf-8", body: MARKDOWN },
      ],
    ]),
    {
      headers: { accept: "text/markdown" },
      method: "HEAD",
    },
  );

  assert.equal(response.status, 200);
  assert.equal(
    response.headers.get("content-type"),
    "text/markdown; charset=utf-8",
  );
  assert.equal(await response.text(), "");
});

// A path that begins with two slashes (and "/\", which the URL parser reads the
// same way) is a reference to another host once it is resolved against the
// site: "//attacker.example/p/index.md" names attacker.example. The worker
// builds the address of a page's Markdown copy from the request path, so
// without a guard anyone could point it at a server of their own and have this
// site hand back that server's status, body and headers as zoomies.sh.
const ATTACKER = "attacker.example";
const ATTACKER_COPY = {
  body: "# owned\n",
  headers: {
    "content-type": "text/markdown",
    "set-cookie": "session=attacker; Domain=zoomies.sh; Path=/",
    "content-security-policy": "default-src *",
  },
};

for (const [name, address] of [
  ["two slashes", `https://zoomies.sh//${ATTACKER}/p/`],
  ["a slash and a backslash", `https://zoomies.sh/\\${ATTACKER}/p/`],
  ["three slashes", `https://zoomies.sh///${ATTACKER}/p/`],
  ["dot segments that collapse to two slashes", `https://zoomies.sh/a/..//${ATTACKER}/p/`],
  ["two slashes and no closing slash", `https://zoomies.sh//${ATTACKER}/p`],
  ["two slashes and an .html name", `https://zoomies.sh//${ATTACKER}/p.html`],
]) {
  test(`a request path of ${name} never sends the worker to another host`, async () => {
    const seen = [];
    const response = await handle(
      address,
      new Map([
        [`${ATTACKER}/p/index.md`, ATTACKER_COPY],
        [`${ATTACKER}/p.md`, ATTACKER_COPY],
      ]),
      { headers: { accept: "text/markdown" } },
      seen,
    );

    assert.deepEqual(
      seen.filter((href) => new URL(href).host !== "zoomies.sh"),
      [],
      "the worker fetched a host other than the site's",
    );
    // The origin's own answer for an address it does not have.
    assert.equal(response.status, 404);
    assert.equal(response.headers.get("set-cookie"), null);
    assert.notEqual(await response.text(), ATTACKER_COPY.body);
  });
}

// Even from the site's own origin, a copy is relayed with a short list of
// headers rather than all of them: a cookie, a content security policy or a
// CORS grant that came from whatever answered the fetch must never be set on
// zoomies.sh by this worker.
test("a markdown copy is answered with only the headers the worker means to relay", async () => {
  const response = await handle(
    "https://zoomies.sh/quickstart/",
    new Map([
      [
        "/quickstart/index.md",
        {
          body: MARKDOWN,
          headers: {
            "content-type": "text/markdown; charset=utf-8",
            "cache-control": "max-age=600",
            etag: '"5f3a"',
            "last-modified": "Tue, 13 Oct 2026 09:00:00 GMT",
            "set-cookie": "session=1; Domain=zoomies.sh; Path=/",
            "content-security-policy": "default-src 'none'",
            "access-control-allow-origin": "https://attacker.example",
            "access-control-allow-credentials": "true",
            "x-served-by": "cache-lhr-1",
          },
        },
      ],
    ]),
    { headers: { accept: "text/markdown" } },
  );

  assert.equal(response.status, 200);
  assert.equal(response.headers.get("cache-control"), "max-age=600");
  assert.equal(response.headers.get("etag"), '"5f3a"');
  assert.equal(
    response.headers.get("last-modified"),
    "Tue, 13 Oct 2026 09:00:00 GMT",
  );
  for (const name of [
    "set-cookie",
    "content-security-policy",
    "access-control-allow-origin",
    "access-control-allow-credentials",
    "x-served-by",
  ]) {
    assert.equal(response.headers.get(name), null, `${name} was relayed`);
  }
});

test("markdown is refused only when an entry's quality refuses it", () => {
  assert.equal(wantsMarkdown("text/markdown"), true);
  assert.equal(wantsMarkdown("TEXT/Markdown"), true);
  assert.equal(wantsMarkdown("text/markdown; q=0.5"), true);
  assert.equal(wantsMarkdown(" text/markdown , text/html"), true);
  assert.equal(wantsMarkdown("text/markdown;q=0"), false);
  assert.equal(wantsMarkdown("text/markdown; Q=0, text/html;q=0.9"), false);
  assert.equal(wantsMarkdown("text/html,*/*;q=0.8"), false);
  assert.equal(wantsMarkdown(""), false);
  assert.equal(wantsMarkdown(null), false);
});

test("the copy's address follows the built site's shape", () => {
  assert.equal(markdownPath("/"), "/index.md");
  assert.equal(markdownPath("/quickstart/"), "/quickstart/index.md");
  assert.equal(markdownPath("/quickstart"), "/quickstart/index.md");
  assert.equal(markdownPath("/index.html"), "/index.md");
  assert.equal(markdownPath("/404.html"), "/404.md");
  assert.equal(markdownPath("/catalog.json"), null);
  assert.equal(markdownPath("/install.sh"), null);
  assert.equal(markdownPath("/screenshots/ui.webp"), null);
  assert.equal(markdownPath("/llms.txt"), null);
  // Not a path on this site at all: resolved against it, it names another host.
  assert.equal(markdownPath("//attacker.example/p/"), null);
  assert.equal(markdownPath("///attacker.example/p"), null);
});

test("the token estimate rounds up the parts of a token", () => {
  assert.equal(tokenEstimate(""), 0);
  assert.equal(tokenEstimate("abcd"), 1);
  assert.equal(tokenEstimate("abcde"), 2);
});

test("the discovery links name all four relations a scanner looks for", () => {
  const links = discoveryLinks();
  for (const relation of [
    "api-catalog",
    "describedby",
    "service-desc",
    "service-doc",
  ]) {
    assert.match(links, new RegExp(`rel="${relation}"`), relation);
  }
});

test("an HTML page answer carries the discovery links", async () => {
  const response = await handle(
    "https://zoomies.sh/",
    new Map([["/", { type: "text/html; charset=utf-8", body: HTML }]]),
    { headers: { accept: "text/html" } },
  );

  assert.equal(response.status, 200);
  assert.match(response.headers.get("link"), /rel="api-catalog"/);
  assert.match(response.headers.get("link"), /rel="service-desc"/);
});

test("a markdown copy carries the discovery links too", async () => {
  const response = await handle(
    "https://zoomies.sh/",
    new Map([
      ["/index.md", { type: "text/markdown; charset=utf-8", body: MARKDOWN }],
    ]),
    { headers: { accept: "text/markdown" } },
  );

  assert.match(response.headers.get("link"), /rel="describedby"/);
});

test("a file that is not a page wears no discovery links", async () => {
  const response = await handle(
    "https://zoomies.sh/install.sh",
    new Map([
      ["/install.sh", { type: "text/x-shellscript", body: "#!/bin/sh\n" }],
    ]),
    {},
  );

  assert.equal(response.headers.get("link"), null);
});

test("the well-known api-catalog address serves the RFC 9727 document", async () => {
  const catalog = '{"linkset": []}';
  const response = await handle(
    "https://zoomies.sh/.well-known/api-catalog",
    new Map([
      ["/api-catalog.json", { type: "application/json", body: catalog }],
    ]),
    {},
  );

  assert.equal(response.status, 200);
  assert.equal(
    response.headers.get("content-type"),
    'application/linkset+json; profile="https://www.rfc-editor.org/info/rfc9727"',
  );
  assert.match(response.headers.get("link"), /rel="api-catalog"/);
  assert.equal(await response.text(), catalog);
});

test("a HEAD request for the api-catalog answers with headers and no body", async () => {
  const catalog = '{"linkset": []}';
  const response = await handle(
    "https://zoomies.sh/.well-known/api-catalog",
    new Map([
      ["/api-catalog.json", { type: "application/json", body: catalog }],
    ]),
    { method: "HEAD" },
  );

  assert.equal(response.status, 200);
  assert.equal(await response.text(), "");
});

test("the identity provider's discovery document is mirrored from the portal", async () => {
  const discovery = JSON.stringify({
    issuer: "https://portal.zoomies.sh",
    authorization_endpoint: "https://portal.zoomies.sh/authorize",
    token_endpoint: "https://portal.zoomies.sh/token",
    jwks_uri: "https://portal.zoomies.sh/jwks.json",
    grant_types_supported: ["authorization_code"],
    response_types_supported: ["code"],
  });
  const response = await handle(
    "https://zoomies.sh/.well-known/openid-configuration",
    new Map([
      ["portal.zoomies.sh/.well-known/openid-configuration", { type: "application/json", body: discovery }],
    ]),
    { headers: { accept: "application/json" } },
  );

  assert.equal(response.status, 200);
  assert.equal(response.headers.get("content-type"), "application/json");
  assert.equal(await response.text(), discovery);
});

test("the authorization-server address serves the provider's own metadata document", async () => {
  const asDoc = JSON.stringify({
    issuer: "https://portal.zoomies.sh",
    token_endpoint: "https://portal.zoomies.sh/oauth/token",
    agent_auth: { skill: "https://portal.zoomies.sh/auth.md", register_uri: "https://portal.zoomies.sh/oauth/register" },
  });
  const response = await handle(
    "https://zoomies.sh/.well-known/oauth-authorization-server",
    new Map([
      ["/.well-known/oauth-authorization-server", { type: "application/json", body: asDoc }],
    ]),
    {},
  );

  assert.equal(response.status, 200);
  assert.equal(response.headers.get("content-type"), "application/json");
  const served = JSON.parse(await response.text());
  assert.notEqual(served.agent_auth, undefined);
  assert.equal(served.token_endpoint, "https://portal.zoomies.sh/oauth/token");
});

test("a HEAD request for the discovery document answers with no body", async () => {
  const discovery = '{"issuer": "https://portal.zoomies.sh"}';
  const response = await handle(
    "https://zoomies.sh/.well-known/openid-configuration",
    new Map([
      ["portal.zoomies.sh/.well-known/openid-configuration", { type: "application/json", body: discovery }],
    ]),
    { method: "HEAD" },
  );

  assert.equal(response.status, 200);
  assert.equal(await response.text(), "");
});

test("when the provider cannot be reached, the origin's answer stands", async () => {
  const response = await handle(
    "https://zoomies.sh/.well-known/openid-configuration",
    new Map([]),
    {},
  );

  assert.equal(response.status, 404);
  assert.equal(response.headers.get("content-type"), "text/html; charset=utf-8");
});

const PRM = JSON.stringify({
  resource: "https://portal.zoomies.sh/api/admin/mcp",
  authorization_servers: ["https://portal.zoomies.sh"],
  scopes_supported: ["mcp:read"],
  bearer_methods_supported: ["header"],
  agent_auth: { skill: "https://portal.zoomies.sh/auth.md", register_uri: "https://portal.zoomies.sh/oauth/register" },
});

test("the generated resource metadata passes through from the origin untouched", async () => {
  const response = await handle("https://zoomies.sh/.well-known/oauth-protected-resource", new Map([
    ["/.well-known/oauth-protected-resource", { type: "application/json", body: PRM }],
  ]), { headers: { accept: "application/json" } });
  assert.equal(response.status, 200);
  assert.equal(await response.text(), PRM);
});
