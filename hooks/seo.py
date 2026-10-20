"""Build-time SEO metadata for zoomies.sh.

Two things the site wants that MkDocs does not know by itself, both derived
from the repository rather than written by hand, so neither can go stale:

* **A real last-modified date per page.** MkDocs' own sitemap stamps every URL
  with the build date, which tells a crawler that the whole site changed every
  time anything did -- exactly the signal that makes freshness meaningless. The
  date a page was last edited is in git, so it is taken from there and put on
  ``page.meta.git_date`` for ``overrides/sitemap.xml`` and the sharing tags.

* **``llms.txt``**, the plain-text index an assistant fetches when it is asked
  about a project instead of crawling it. It is generated from the navigation,
  so a page added to ``mkdocs.yml`` appears in it without anyone remembering.

* **``/.well-known/security.txt``** (RFC 9116), where a researcher looks for how
  to report a vulnerability. It says what SECURITY.md says and points at it.
  ``Expires`` is required and must be under a year away, so a tracked file
  would quietly lapse; it is written at build, and the site is built on every
  push to main.

* **An API Catalog document** (RFC 9727), written as ``api-catalog.json`` and
  served at ``/.well-known/api-catalog`` by the site worker. It describes the
  controller's REST API with the machine description (the OpenAPI document in
  the repository), the human documentation, and the health endpoint of the
  live demo. Pages carry ``Link`` headers pointing at it and at ``llms.txt``.

* **The agent discovery documents**, the rest of what a machine reader asks
  for: ``/auth.md`` (copied from ``hooks/auth.md``), the MCP server card, the
  two product skills, each folder copied under ``agent-skills/`` with the
  digest of its ``SKILL.md``, the ai-catalog index pointing at all of it, the
  ``did.json`` its host identifier resolves to, the site's own OAuth
  protected-resource metadata, an A2A agent card for discovery, which names
  the MCP interface the product actually speaks and says that is what it
  speaks, and a ``poppy.json``. Each names something that is really serving.
  The authorization server's metadata is the hosted service's and is mirrored
  at the edge (``site-worker/``), and A2A protocol metadata stays with the
  deployments that run it; ``/auth.md`` links to the served one.

* **A Markdown copy of every page**, at the page's own address plus
  ``index.md``, and **``llms-full.txt``**, the readable pages in one file. The
  HTML is server-rendered and complete, but an assistant asked about a page
  pays for the navigation, the footer and 90 inline icons to read it, and
  GitHub Pages cannot answer ``Accept: text/markdown`` (the Cloudflare Worker
  in ``site-worker/`` answers it for the site, by serving these copies). The
  source is already Markdown, so the copy is the source with the parts only
  the theme understands taken out. ``llms-full.txt`` stops at a size an
  assistant can read in one go; the long reference pages are one link each
  rather than 185 KB in the middle.

This is a hook rather than a plugin because it needs nothing a plugin would
bring: the docs build stays at the three pinned dependencies in
``docs/requirements.txt``.

The git call is one process for the whole repository, not one per page. A
shallow checkout has no history to read, so the dates come back empty and every
page falls back to the build date -- which is why the site workflow checks out
with ``fetch-depth: 0``.
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
import posixpath
import re
import shutil
import subprocess
from datetime import datetime, timedelta, timezone

log = logging.getLogger("mkdocs.hooks.seo")

# Populated once per build by on_files, keyed by path relative to docs_dir.
_dates: dict[str, str] = {}
# The resolved navigation, kept from on_nav so llms.txt can be written from it.
_nav = None
# Each page's Markdown as written, keyed by path relative to docs_dir, kept from
# on_page_markdown for the per-page copies and llms-full.txt.
_sources: dict[str, tuple[object, str]] = {}

# A page larger than this is not copied into llms-full.txt, only linked. The
# comparisons and the short explainers are well under it. The quickstart is
# over it, as are many of the product pages, and the generated references
# (every setting, every API route, every problem code) are several times over,
# and one of them would crowd out the rest. What is left out is listed at the
# end of the file with a link to its own copy.
_FULL_PAGE_LIMIT = 15_000

_comment = re.compile(r"<!--.*?-->", re.DOTALL)
# The end tag is matched as widely as a browser matches it: `</script >` and
# `</script foo="bar">` both close the element, and a pattern that missed them
# would leave the script's text in the copy llms.txt carries.
_jsonld = re.compile(r"<script\b[^>]*>.*?</script[^<>]*>", re.DOTALL | re.IGNORECASE)
_snippet = re.compile(r"^--8<--.*\n?", re.MULTILINE)
_md_link = re.compile(r"(\]\()([^)\s#]+\.md)(#[^)\s]*)?(\))")
_only_light = re.compile(r"^!\[[^\n]*\]\([^)\n]*#only-light\)[^\n]*\n?", re.MULTILINE)
_image = re.compile(r"(!\[[^\]\n]*\]\()([^)\s#:]+)((?:#[^)\s]*)?\))")
_leading_h1 = re.compile(r"\A\s*# [^\n]*\n")


def _git_dates(docs_dir: str) -> dict[str, str]:
    """Map each file under docs_dir to the ISO date of its last commit."""
    cmd = [
        "git",
        "log",
        "--pretty=format:%x00%cI",
        "--name-only",
        "--no-renames",
        "--",
        ".",
    ]
    try:
        out = subprocess.run(
            cmd,
            cwd=docs_dir,
            capture_output=True,
            text=True,
            timeout=60,
            check=True,
        ).stdout
    except (OSError, subprocess.SubprocessError) as err:
        log.warning("no git history for the docs, so pages fall back to the build date: %s", err)
        return {}

    dates: dict[str, str] = {}
    current = ""
    for line in out.splitlines():
        if line.startswith("\x00"):
            # A commit header: everything until the next one was touched by it.
            current = line[1:11]
            continue
        path = line.strip()
        if not path:
            continue
        # git prints paths relative to the repository root even with a cwd, and
        # the newest commit touching a file is the first one seen.
        dates.setdefault(path, current)
    return dates


def on_files(files, config):
    """Read the history once, before any page is rendered."""
    global _dates
    root = os.path.relpath(config["docs_dir"], os.path.dirname(config["config_file_path"]))
    raw = _git_dates(config["docs_dir"])
    prefix = root.replace(os.sep, "/") + "/"
    _dates = {}
    for path, date in raw.items():
        key = path[len(prefix):] if path.startswith(prefix) else path
        _dates.setdefault(key, date)
    return files


def on_nav(nav, config, files):
    """Keep the resolved navigation for llms.txt."""
    global _nav
    _nav = nav
    return nav


def on_page_markdown(markdown, page, config, files):
    """Hand the page's own last-edited date to the templates.

    This runs while the pages are being read, which is before MkDocs renders
    the theme's static templates. sitemap.xml is one of those, and it reads
    ``page.meta.git_date`` for every page; set any later -- in
    ``on_page_context``, say -- the sitemap has already been written with the
    build date for every URL.
    """
    date = _dates.get(page.file.src_uri)
    if date:
        page.meta.setdefault("git_date", date)
    _sources[page.file.src_uri] = (page, markdown)
    return markdown


def _clean(markdown: str, src_uri: str, urls: dict[str, str]) -> str:
    """The page's Markdown without what only the theme understands.

    Comments are notes to a maintainer, the structured-data script is the FAQ
    saying its answers a second time, and an include line names a file in this
    repository. Links between pages are written as ``other.md`` so they work on
    GitHub and in the editor; here they become the page's address, because a
    copy read on its own has no neighbouring file to resolve them against.
    """
    text = _comment.sub("", markdown)
    text = _jsonld.sub("", text)
    text = _snippet.sub("", text)
    # Every screenshot is in the page twice, once for each colour scheme, and
    # only one is ever shown. A reader without a scheme needs just one of them.
    text = _only_light.sub("", text)

    def image(match: re.Match) -> str:
        path = posixpath.normpath(posixpath.join(posixpath.dirname(src_uri), match.group(2)))
        return f"{match.group(1)}{urls['']}{path}{match.group(3)}"

    text = _image.sub(image, text)

    def link(match: re.Match) -> str:
        target = posixpath.normpath(posixpath.join(posixpath.dirname(src_uri), match.group(2)))
        url = urls.get(target)
        if url is None:
            return match.group(0)
        return f"{match.group(1)}{url}{match.group(3) or ''}{match.group(4)}"

    text = _md_link.sub(link, text)
    return re.sub(r"\n{3,}", "\n\n", text).strip() + "\n"


def _documents(config) -> list[tuple[str, str, str, str]]:
    """(src_uri, canonical URL, title, Markdown) for every page, in navigation order.

    Pages the navigation does not name (the brand guide) follow it, so each page
    has a copy even when it is not a section of the site.
    """
    site_url = str(config["site_url"] or "").rstrip("/")
    urls = {src: f"{site_url}/{page.url}" for src, (page, _) in _sources.items()}
    # Not a page: the root, so an image path can be made absolute against it.
    urls[""] = f"{site_url}/"

    ordered: list[str] = []

    def walk(items):
        for item in items:
            if item.is_page:
                ordered.append(item.file.src_uri)
            elif item.is_section:
                walk(item.children)

    walk(_nav.items if _nav is not None else [])
    ordered += [src for src in _sources if src not in ordered]

    out = []
    for src in ordered:
        if src not in _sources:
            continue
        page, markdown = _sources[src]
        title = str(page.meta.get("title") or page.title)
        body = _leading_h1.sub("", _clean(markdown, src, urls), count=1).strip()
        out.append((src, urls[src], title, f"# {title}\n\nCanonical page: {urls[src]}\n\n{body}\n"))
    return out


def on_post_build(config):
    """Write llms.txt: the site's table of contents, in one plain-text file."""
    lines = [
        f"# {config['site_name']}",
        "",
        f"> {' '.join(str(config['site_description']).split())}",
        "",
        "Zoomies is free and open source under the AGPL-3.0. It is a single Go",
        "binary that watches a GitHub organisation or a repository on a personal",
        "account for queued Actions jobs. It starts one ephemeral runner per job",
        "on a machine you own and destroys it when the job finishes.",
        "SQLite for state, no Kubernetes and no database server,",
        "and a live web UI for the whole fleet. Runner images for Ubuntu, Debian,",
        "Fedora and Rocky Linux, on x86-64 and arm64. See the home page's",
        "qualification section for what has been tested and the current limits.",
        "",
        "## Documentation",
        "",
    ]

    site_url = str(config["site_url"] or "").rstrip("/")

    def walk(items):
        for item in items:
            if item.is_page:
                url = item.canonical_url or f"{site_url}/{item.url}"
                summary = " ".join(str(item.meta.get("description", "")).split())
                lines.append(f"- [{item.title}]({url})" + (f": {summary}" if summary else ""))
            elif item.is_section:
                lines.append("")
                lines.append(f"### {item.title}")
                lines.append("")
                walk(item.children)

    walk(_nav.items if _nav is not None else [])

    lines += [
        "",
        "## Source",
        "",
        f"- [Repository]({config['repo_url']}): the Go controller, the Svelte UI and these docs.",
        f"- [Full text]({site_url}/llms-full.txt): the readable pages in one file. Every page also has a Markdown copy at its address plus index.md, for example {site_url}/quickstart/index.md.",
        f"- [install.sh]({site_url}/install.sh): the one-line installer, byte-identical to the one in the repository.",
        f"- [install.ps1]({site_url}/install.ps1): the Windows installer, run from an elevated PowerShell window; byte-identical to the one in the repository.",
        f"- [OpenAPI document]({config['repo_url']}/blob/main/api/openapi.yaml): the REST contract both clients are generated from.",
        f"- [Catalog of problem codes and checks]({site_url}/catalog.json): every code Zoomies can raise, with what to do and how to see it worked, as JSON; its schema is beside it at catalog.schema.json.",
        "",
        f"Generated {datetime.now(timezone.utc).strftime('%Y-%m-%d')}.",
        "",
    ]

    with open(os.path.join(config["site_dir"], "llms.txt"), "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))

    _write_markdown(config, site_url)
    _write_security_txt(config, site_url)
    _write_auth_md(config)
    _write_agent_discovery(config, site_url)
    _write_api_catalog(config, site_url)

    # The catalog is generated into the Go package that embeds it, so the
    # binary and the site serve the same bytes; the site copies it rather than
    # keeping a second tracked file that could fall behind.
    catalog_dir = os.path.join(os.path.dirname(os.path.abspath(config["docs_dir"])), "internal", "catalog")
    for name, published in (("catalog.json", "catalog.json"), ("schema.json", "catalog.schema.json")):
        shutil.copyfile(os.path.join(catalog_dir, name), os.path.join(config["site_dir"], published))


def _write_markdown(config, site_url: str) -> None:
    """Write each page's Markdown copy and llms-full.txt into the built site."""
    documents = _documents(config)
    full = [
        f"# {config['site_name']}: the documentation in one file",
        "",
        "These are the pages of " + site_url + "/ as Markdown, in the order of its navigation.",
        "The index is " + site_url + "/llms.txt. A page too long to include here is listed at the end,",
        "with a link to its own Markdown copy.",
        "",
    ]
    long_pages = []
    for src, url, title, text in documents:
        page = _sources[src][0]
        target = os.path.join(config["site_dir"], page.url.replace("/", os.sep), "index.md")
        os.makedirs(os.path.dirname(target), exist_ok=True)
        with open(target, "w", encoding="utf-8") as fh:
            fh.write(text)
        if len(text) > _FULL_PAGE_LIMIT:
            long_pages.append((title, url))
        else:
            full += ["---", "", text]

    if long_pages:
        full += ["---", "", "## Pages too long to include here", ""]
        full += [f"- [{title}]({url}index.md)" for title, url in long_pages]
        full.append("")

    with open(os.path.join(config["site_dir"], "llms-full.txt"), "w", encoding="utf-8") as fh:
        fh.write("\n".join(full))


def _write_security_txt(config, site_url: str) -> None:
    """Write /.well-known/security.txt, which says where to report a vulnerability."""
    repo = str(config["repo_url"]).rstrip("/")
    expires = (datetime.now(timezone.utc) + timedelta(days=330)).strftime("%Y-%m-%dT00:00:00.000Z")
    lines = [
        "# Report a vulnerability as a private security advisory, not a public issue.",
        f"Contact: {repo}/security/advisories/new",
        f"Expires: {expires}",
        "Preferred-Languages: en",
        f"Policy: {repo}/blob/main/SECURITY.md",
        f"Canonical: {site_url}/.well-known/security.txt",
        "",
    ]
    target = os.path.join(config["site_dir"], ".well-known")
    os.makedirs(target, exist_ok=True)
    with open(os.path.join(target, "security.txt"), "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))


# The live demo instance, where every interface the docs describe is actually
# serving. The discovery documents below name it because a machine client
# reading the site wants a deployment it can reach, not a promise.
_DEMO = "https://demo.zoomies.sh"


def _write_api_catalog(config, site_url: str) -> None:
    """Write api-catalog.json, the API Catalog document of RFC 9727.

    It is written with a .json name rather than straight into .well-known/
    because GitHub Pages guesses a file's content type from its extension, and
    an extensionless file would be served as application/octet-stream where the
    RFC asks for application/linkset+json. The site worker serves the well-known
    address by fetching this file and setting the type itself.

    The API itself runs on each deployment, not on this site, so the catalog
    anchors at the page that documents the API surface and links the OpenAPI
    document in the repository: the two things a machine client can fetch
    today. The status relation breaks that rule on purpose: health belongs to
    a deployment, and the demo is the one whose health a machine client can
    read. Should the demo go away, the link answers 404, which is the honest
    answer for a health check, not a promise that has stopped being kept.
    """
    repo = str(config["repo_url"]).rstrip("/")
    catalog = {
        "linkset": [
            {
                "anchor": f"{site_url}/api-surface/",
                "service-desc": [
                    {"href": f"{repo}/raw/main/api/openapi.yaml", "type": "application/yaml"},
                ],
                "service-doc": [
                    {"href": f"{site_url}/api-surface/", "type": "text/html"},
                ],
                "status": [
                    {"href": f"{_DEMO}/readyz", "type": "application/json"},
                ],
            },
        ],
    }
    with open(os.path.join(config["site_dir"], "api-catalog.json"), "w", encoding="utf-8") as fh:
        json.dump(catalog, fh, indent=2)
        fh.write("\n")


def _write_auth_md(config) -> None:
    """Copy /auth.md into the site root.

    The document is tracked as hooks/auth.md, beside the hook that ships it,
    because a file under docs/.well-known would never reach the site (mkdocs
    passes over names beginning with a dot) and a page in docs/ would be
    rendered as HTML where the convention asks for Markdown at the root.
    """
    root = os.path.dirname(os.path.abspath(config["config_file_path"]))
    shutil.copyfile(os.path.join(root, "hooks", "auth.md"), os.path.join(config["site_dir"], "auth.md"))


def _write_agent_discovery(config, site_url: str) -> None:
    """Write the discovery documents for what the product actually offers.

    Everything here names something a machine client can reach: the skills the
    repository installs (each folder copied byte for byte, with the digest of
    its SKILL.md), the MCP
    interface the controller serves at /mcp with the demo instance standing in
    as the reachable example, and the index under which a registry can find
    all of it. The documents are written into .well-known/ here because mkdocs
    passes over dot directories, and as .json files because Pages would serve
    an extensionless name as application/octet-stream.

    No document claims something is running unless it is: A2A metadata belongs
    to a deployment, not to the site that documents it, and publishing it here
    would be discovery metadata pointing at nothing. The OAuth protected-resource
    document is written here because the site declares it for itself; the
    authorization server it names is the hosted service's, whose own documents
    the edge worker mirrors. auth.md says where the real chain lives.
    """
    root = os.path.dirname(os.path.abspath(config["config_file_path"]))
    well = os.path.join(config["site_dir"], ".well-known")
    os.makedirs(well, exist_ok=True)

    # The release the header shows, from hooks/source.py's on_config, so the
    # card names the version the skill and scripts were generated from. A
    # build with no network has no facts and the card carries a placeholder.
    version = (config.get("extra") or {}).get("source_facts", {}).get("version") or "0.0.0"

    skills: list[dict] = []
    for name in ("zoomies", "zoomies-kennel"):
        folder = os.path.join(root, "skills", name)
        with open(os.path.join(folder, "SKILL.md"), "rb") as fh:
            body = fh.read()
        # The whole folder, not SKILL.md alone: the skill keeps its detail in
        # files beside it and reads them by relative link, and those links
        # answered 404 when only SKILL.md was published. The index still names
        # SKILL.md, and its digest is of that file.
        shutil.copytree(
            folder,
            os.path.join(well, "agent-skills", name),
            dirs_exist_ok=True,
            ignore=shutil.ignore_patterns(".*", "__pycache__"),
        )
        front = re.search(r"^description: (.+)$", body.decode("utf-8"), re.MULTILINE)
        if front is None:
            raise ValueError(f"skills/{name}/SKILL.md has no front-matter description for the discovery index")
        skills.append({
            "name": name,
            "type": "skill-md",
            "description": front.group(1).strip(),
            "url": f"{site_url}/.well-known/agent-skills/{name}/SKILL.md",
            "digest": "sha256:" + hashlib.sha256(body).hexdigest(),
        })

    with open(os.path.join(well, "agent-skills", "index.json"), "w", encoding="utf-8") as fh:
        json.dump({
            "$schema": "https://schemas.agentskills.io/discovery/0.2.0/schema.json",
            "skills": skills,
        }, fh, indent=2)
        fh.write("\n")

    card_dir = os.path.join(well, "mcp")
    os.makedirs(card_dir, exist_ok=True)
    with open(os.path.join(card_dir, "server-card.json"), "w", encoding="utf-8") as fh:
        json.dump({
            "serverInfo": {
                "name": "zoomies",
                "title": "Zoomies",
                "version": version,
                "description": (
                    "The MCP interface of a self-hosted GitHub Actions runner fleet controller: "
                    "why jobs queue, wait, fail or run, what the fleet's hosts and pools look like, "
                    "and the changes an operator confirms before anything happens. The controller "
                    "serves it at /mcp on every deployment; the endpoint here is the live demo."
                ),
            },
            "transport": {
                "protocol": "streamable-http",
                "endpoint": f"{_DEMO}/mcp",
            },
            "capabilities": {
                "tools": {"listChanged": False},
                "resources": {"subscribe": False, "listChanged": False},
                "prompts": {"listChanged": False},
            },
            "authorization": {
                "type": "oauth2",
                "protectedResourceMetadata": f"{_DEMO}/.well-known/oauth-protected-resource",
                "authorizationServerMetadata": f"{_DEMO}/.well-known/oauth-authorization-server",
                "scopesSupported": ["mcp:read", "mcp:operate"],
                "bearerMethods": ["header"],
            },
            "documentation": f"{site_url}/connect-claude/",
        }, fh, indent=2)
        fh.write("\n")

    with open(os.path.join(well, "agent-card.json"), "w", encoding="utf-8") as fh:
        json.dump({
            "name": "Zoomies",
            "description": (
                "The agent surface of Zoomies, a self-hosted GitHub Actions runner fleet "
                "controller: read the fleet's jobs, hosts, pools and runners, explain why "
                "work queued, waited, failed or scaled, and act on it where a person "
                "confirms. The url is the live demo deployment's MCP endpoint, and the "
                "wire protocol it speaks is MCP over Streamable HTTP rather than A2A; "
                "this card is for discovery and says so."
            ),
            "url": f"{_DEMO}/mcp",
            "version": version,
            "provider": {"organization": "Zoomies", "url": "https://zoomies.sh"},
            "documentationUrl": f"{site_url}/connect-claude/",
            "capabilities": {"streaming": False, "pushNotifications": False},
            "defaultInputModes": ["application/json"],
            "defaultOutputModes": ["application/json"],
            "supportedInterfaces": [
                {"url": f"{_DEMO}/mcp", "transport": "MCP"},
            ],
            "skills": [
                {
                    "id": "fleet-inspection",
                    "name": "Fleet inspection",
                    "description": "Read the fleet's jobs, hosts, pools and runners, and explain why work queued, waited, failed or scaled.",
                    "tags": ["github-actions", "diagnostics", "fleet"],
                },
                {
                    "id": "fleet-operation",
                    "name": "Fleet operation",
                    "description": "Propose and, with the person's confirmation, apply operator changes: remedies, runner drains, host throttles and host edits, each audited.",
                    "tags": ["operations", "fleet"],
                },
                {
                    "id": "workflow-advice",
                    "name": "Workflow advice",
                    "description": "Read a repository's workflow files and say what the fleet would run them on, and what changed between versions.",
                    "tags": ["github-actions", "workflows"],
                },
            ],
            # An OAuth 2.0 authorization server with RFC 8414 metadata, as the MCP
            # server card above says, and not an OpenID provider: it issues no ID
            # tokens and publishes no /.well-known/openid-configuration (see
            # handleAuthorizationServer in internal/api), so an openIdConnect
            # scheme would send a client to the demo's HTML page.
            "securitySchemes": {
                "demoOAuth": {
                    "type": "oauth2",
                    "description": "Sign in to the demo with the authorization code flow and PKCE.",
                    "oauth2MetadataUrl": f"{_DEMO}/.well-known/oauth-authorization-server",
                    "flows": {
                        "authorizationCode": {
                            "authorizationUrl": f"{_DEMO}/oauth/authorize",
                            "tokenUrl": f"{_DEMO}/oauth/token",
                            "refreshUrl": f"{_DEMO}/oauth/token",
                            "scopes": {
                                "mcp:read": "Read the fleet. The viewer role.",
                                "mcp:operate": "Act on the fleet, each action audited. The operator role, which includes mcp:read.",
                            },
                        }
                    },
                }
            },
            "security": [{"demoOAuth": ["mcp:read", "mcp:operate"]}],
        }, fh, indent=2)
        fh.write("\n")

    # The catalog names its host by a did:web identifier, which resolves to
    # /.well-known/did.json on that domain. The document says who the DID is and
    # nothing more: no keys and no services, because nothing here signs anything
    # a key would be checked against. It exists so the identifier the catalog
    # claims resolves, not to claim more.
    host_did = "did:web:zoomies.sh"
    with open(os.path.join(well, "did.json"), "w", encoding="utf-8") as fh:
        json.dump({"@context": ["https://www.w3.org/ns/did/v1"], "id": host_did}, fh, indent=2)
        fh.write("\n")

    with open(os.path.join(well, "ai-catalog.json"), "w", encoding="utf-8") as fh:
        json.dump({
            "specVersion": "1.0",
            "host": {"displayName": "Zoomies", "identifier": host_did},
            "entries": [
                {
                    "identifier": "urn:air:zoomies.sh:server:fleet-mcp",
                    "displayName": "The Zoomies MCP server",
                    "type": "application/mcp-server-card+json",
                    "url": f"{site_url}/.well-known/mcp/server-card.json",
                    "representativeQueries": [
                        "why did my GitHub Actions job queue for so long",
                        "what is wrong with my self-hosted runner fleet",
                        "which pools and hosts are running my CI",
                        "diagnose a failed Actions job from its logs",
                    ],
                },
                {
                    "identifier": "urn:air:zoomies.sh:api:rest",
                    "displayName": "The Zoomies REST interface",
                    "type": "text/html",
                    "url": f"{site_url}/api-surface/",
                    "representativeQueries": [
                        "which REST routes does the fleet controller expose",
                        "what does GET /api/v1/hosts return",
                        "how do I authenticate to the Zoomies API",
                    ],
                },
                {
                    "identifier": "urn:air:zoomies.sh:skills:product",
                    "displayName": "The Zoomies skills",
                    "type": "application/json",
                    "url": f"{site_url}/.well-known/agent-skills/index.json",
                    "representativeQueries": [
                        "which skill should a coding agent install to run the fleet",
                        "check GitHub Actions workflow files locally against a ruleset",
                        "look after a self-hosted runner fleet from the terminal",
                    ],
                },
                {
                    "identifier": "urn:air:zoomies.sh:reference:problem-codes",
                    "displayName": "The problem-code catalog",
                    "type": "application/json",
                    "url": f"{site_url}/catalog.json",
                    "representativeQueries": [
                        "what does problem code bind.public_no_tls mean",
                        "what should I do when a host stops reporting healthy",
                        "which problem codes can the controller raise",
                    ],
                },
            ],
        }, fh, indent=2)
        fh.write("\n")

    # The resource metadata the site declares about itself (RFC 9728): the
    # resource is this site, the authorization server is the hosted service's
    # provider, and the agent_auth block is the same one the provider's
    # metadata carries. The mirrors (site-worker/) serve the provider's two
    # documents; this one is written here, because the resource is the site's
    # to declare and a mirror's bytes must name the mirror's host. It is a
    # .json file for the reason every document here is, and the worker serves
    # it at the extensionless well-known address with the application/json
    # that RFC 9728 requires and Pages would not give it.
    portal = "https://portal.zoomies.sh"
    with open(os.path.join(well, "oauth-protected-resource.json"), "w", encoding="utf-8") as fh:
        json.dump({
            "resource": f"{site_url}/",
            "authorization_servers": [portal],
            "scopes_supported": ["mcp:read"],
            "bearer_methods_supported": ["header"],
            "resource_name": "Zoomies",
            "resource_documentation": f"{site_url}/auth.md",
            "agent_auth": {
                "skill": f"{portal}/auth.md",
                "register_uri": f"{portal}/oauth/register",
                "registration_methods": [{
                    "type": "dynamic_client_registration",
                    "documented_at": f"{portal}/auth.md",
                    "grant_types": ["authorization_code", "refresh_token"],
                    "consent": "a staff member of the client's choosing approves the connection on the portal, after the second factor",
                }],
                "identity_types_supported": [],
            },
        }, fh, indent=2)
        fh.write("\n")

    with open(os.path.join(well, "poppy.json"), "w", encoding="utf-8") as fh:
        json.dump({
            "protocol_version": "0.1",
            "organization": {"name": "Zoomies", "domain": "zoomies.sh"},
            "web": {},
        }, fh, indent=2)
        fh.write("\n")
