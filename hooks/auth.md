<!-- Copy: served verbatim at https://zoomies.sh/auth.md by hooks/seo.py. It
     describes what a person, or an agent acting for one, can pass at a Zoomies
     deployment and at the hosted service's identity. Keep it true to
     internal/api and internal/auth (the fleet's half) and to the portal's
     discovery documents (the hosted half), and check both when something here
     names a scope or an endpoint. -->

# auth.md

How an agent authenticates against a Zoomies deployment, and what needs an
account to begin with.

## Flows

- **Consent first**: every credential an agent gets here was approved by a
  person, on a page, with the second factor behind them. There is no
  self-issued or anonymous registration on either half of the product.
- **Agent as an OAuth client** (the hosted service's staff console, and the
  self-hosted controller's MCP): dynamic client registration, an
  authorization-code flow with PKCE, a consent page between the request and
  the grant. The full walk is in [Registration](#registration-a-complete-flow).
- **Operator-issued bearer tokens** (the self-hosted REST interface): a
  long-lived token with explicit scopes, created by an operator.
- In the auth.md vocabulary: `identity_assertion` and its ID-JAG and
  verified-email forms are not enabled, `anonymous` is not enabled, and
  `service_auth` is replaced by the consent ceremony above. The
  `identity_types_supported` list is empty in both documents that carry it,
  and nothing here issues a credential an agent can claim without its
  person's own decision.

## What needs no credentials

This site is public documentation. Every page, the sitemap, llms.txt,
llms-full.txt, the Markdown copy beside every page and the problem-code
catalog at catalog.json answer any request, and the robots.txt content
signals say yes to all three uses.

## What does

A fleet is each operator's own deployment of the single `zoomies` binary, so
the interfaces an agent drives live at the deployment's own address, not here.
There are two of them, and both are documented in full:

- The REST interface, under `/api/v1`, in
  [the API surface](https://zoomies.sh/api-surface/), generated from
  `api/openapi.yaml` in the repository.
- The MCP interface, at `/mcp` on every controller, over Streamable HTTP: the
  tools an agent calls to ask about and, where the person confirms, act on the
  fleet. For details see [connecting an agent](https://zoomies.sh/connect-claude/).

The hosted service (the portal that operates instances for a team) runs the
one identity the site can name: an OpenID-style provider at
`https://portal.zoomies.sh`, whose discovery documents this site mirrors at
its own addresses. The walkthrough below registers an agent against it.

## Scopes

- `mcp:read`: read the fleet the way a viewer may. What the hosted service's
  staff console issues; the staff member's own role at the portal caps every
  call.
- `mcp:operate`: on a self-hosted controller, what an operator may change,
  implying `mcp:read`.
- The REST interface's tokens carry explicit scopes in `resource.verb` form.

## Registration: a complete flow

The portal registers agents as OAuth clients, and the person who will be
acting decides on the portal's page before the client can do anything. An
agent that wants its own way in must walk the person through it; there is no
self-issued credential.

1. **Discover** (two hops):

   ```
   GET https://zoomies.sh/.well-known/oauth-protected-resource
   ```

   ```json
   {"resource": "https://zoomies.sh/",
    "authorization_servers": ["https://portal.zoomies.sh"],
    "scopes_supported": ["mcp:read"], "bearer_methods_supported": ["header"],
    "resource_documentation": "https://zoomies.sh/auth.md",
    "agent_auth": {"skill": "https://portal.zoomies.sh/auth.md",
                   "register_uri": "https://portal.zoomies.sh/oauth/register",
                   "registration_methods": [{"type": "dynamic_client_registration",
                                             "grant_types": ["authorization_code", "refresh_token"],
                                             "consent": "a staff member approves the connection, after the second factor"}],
                   "identity_types_supported": []}}
   ```

   The `agent_auth` block names the hosted service's own copy of this
   walkthrough and its registration endpoint. The provider's two discovery
   documents are mirrored here from its own addresses (the same bytes);
   the resource metadata is this site's own, because the resource is the
   site's to declare.

2. **Register the client** (dynamic client registration, RFC 7591):

   ```http
   POST https://portal.zoomies.sh/oauth/register
   Content-Type: application/json

   {"client_name": "the agent's name",
    "redirect_uris": ["https://the.agent/callback"],
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"]}
   ```

   ```json
   {"client_id": "<the id>", "redirect_uris": ["https://the.agent/callback"],
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"], "token_endpoint_auth_method": "none",
    "scope": "mcp:read"}
   ```

   Clients are public: no secret is sent anywhere, and PKCE (S256) binds
   every code to the client that asked. `invalid_client_metadata`,
   `invalid_redirect_uri` and `too_many_requests` arrive in RFC 7591's shape.

3. **Ask for a sign-in, and the person decides**:

   ```http
   GET https://portal.zoomies.sh/oauth/authorize?response_type=code
       &client_id=<id>&redirect_uri=<uri>&state=<state>
       &code_challenge=<s256 challenge>&code_challenge_method=S256
   ```

   The portal redirects a browser to its consent page. The staff member signs
   in to the portal (password and a second factor, or a passkey), reads what
   the client named and what the connection may see, and approves. An
   approval returns to the client's address with `code`; a denial is
   `error=access_denied`. This page is the whole security decision, and it is
   the part the portal does not offer to automation: no anonymous or
   self-issued registration exists, because a token here acts as one named
   person, and that person consents with the second factor behind them.

4. **Exchange**:

   ```http
   POST https://portal.zoomies.sh/oauth/token
   Content-Type: application/x-www-form-urlencoded

   grant_type=authorization_code&code=<code>&client_id=<id>
   &redirect_uri=<uri>&code_verifier=<the pkce verifier>
   ```

   ```json
   {"access_token": "<secret>", "token_type": "Bearer", "expires_in": 3600,
    "refresh_token": "<secret>", "scope": "mcp:read"}
   ```

   A used or wrong code is `invalid_grant`, and the walk starts again at
   step 3. Refresh (`grant_type=refresh_token`) returns a new pair and
   retires the old.

5. **Act, and end it**: `POST https://portal.zoomies.sh/api/admin/mcp`, Bearer
   in hand, every call audited under the staff member's name. Revoke at
   `https://portal.zoomies.sh/oauth/revoke` (RFC 7009); the staff member can
   also end the connection from their own account, which stops the token at
   once. A 401 from the endpoint carries the discovery pointer in its
   `WWW-Authenticate` header, so a closed door still shows where the lock is.

### On a self-hosted controller

The same shape, at the deployment's own address, and no mirror: the
controller is its own authorization server and resource server. The scope
names differ (`mcp:read`, `mcp:operate`), and dynamic client registration is
part of the metadata only while the deployment's
`security.mcp_open_registration` setting is on; the default is off, and the
operator adds the MCP client by hand first. The [public demo](https://demo.zoomies.sh/)
serves the whole chain live:

```
curl -s https://demo.zoomies.sh/.well-known/oauth-protected-resource
curl -s https://demo.zoomies.sh/.well-known/oauth-authorization-server
```

[The connection walkthrough](https://zoomies.sh/connect-claude/) does the
same walk by hand, one screen at a time.

## Credentials for the REST interface

An operator creates long-lived API tokens in the controller's settings. Each
token carries an explicit scope list in `resource.verb` form (what a viewer
may read, what an operator may change), and every request presents it as a
bearer token:

    Authorization: Bearer <token>

The token answers as the role its scopes give it, never more; a route the
scope does not cover is refused, and the refusal says which one is missing.
There is no agent account to create: the token is granted by the person who
runs the instance, the instance never learns anything else about an agent,
and revoking it disconnects the agent at once.
