<!-- Copy: served verbatim at https://zoomies.sh/auth.md by hooks/seo.py. It
     describes what a person, or an agent acting for one, can pass at a Zoomies
     deployment. Keep it true to internal/api and internal/auth, and check both
     when something here names a scope or an endpoint. -->

# auth.md

How an agent authenticates against a Zoomies deployment, and what needs an
account to begin with.

## Flows

- **Consent first**: every credential an agent gets here was approved by a
  person, on a page. A controller's consent page is its ordinary sign-in,
  which asks for a second factor only of an account that has two-step
  verification turned on (or where the instance requires it) and never of a
  single-sign-on account, whose provider owns that step. Registering a client
  takes no sign-in, but a registered client holds no credential until a person
  approves it, so no credential is self-issued or anonymous.
- **Agent as an OAuth client** (the controller's MCP): dynamic client
  registration, an authorization-code flow with PKCE, a consent page between
  the request and the grant. The full walk is in
  [Registration](#registration-a-complete-flow).
- **API tokens** (the REST interface): a bearer token that anybody signed in
  creates for themselves, with a role no higher than their own, optionally
  narrowed by scopes and optionally expiring.
- In the auth.md vocabulary: `identity_assertion` and its ID-JAG and
  verified-email forms are not enabled, `anonymous` is not enabled, and
  `service_auth` is replaced by the consent ceremony above. Nothing here
  issues a credential an agent can claim without its person's own decision.

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

The controller is its own authorization server and resource server. The
walkthrough below uses `https://<your controller>` for the deployment's
address; the [public demo](https://demo.zoomies.sh/) serves the whole chain
live.

## Scopes

- `mcp:read`: read the fleet the way a viewer may.
- `mcp:operate`: what an operator may change, implying `mcp:read`.
- The REST interface's tokens may carry scopes, written `resource:verb` (for
  example `hosts:read`, or `pools:*` for every verb on pools). A scope only
  narrows the token's role; with none, a token has whatever its role allows.

## Registration: a complete flow

The controller registers agents as OAuth clients, and the person who will be
acting decides on the controller's consent page before the client can do
anything. An agent that wants its own way in must walk the person through it;
there is no self-issued credential.

Dynamic client registration is part of the metadata while the deployment's
`security.mcp_open_registration` setting is on, and the default is on; an
operator who turns it off adds the MCP client by hand first.

1. **Discover** (two hops):

   ```
   GET https://<your controller>/.well-known/oauth-protected-resource/mcp
   ```

   ```json
   {"resource": "https://<your controller>/mcp",
    "authorization_servers": ["https://<your controller>"],
    "scopes_supported": ["mcp:read", "mcp:operate"],
    "bearer_methods_supported": ["header"],
    "resource_name": "Zoomies",
    "resource_documentation": "https://zoomies.sh/connect-claude/"}
   ```

   The controller is the authorization server in that answer. It publishes
   OAuth authorization-server metadata at
   `https://<your controller>/.well-known/oauth-authorization-server`. It
   issues no ID tokens and publishes no signing keys, so it has no OpenID
   Connect discovery document.

2. **Register the client** (dynamic client registration, RFC 7591):

   ```http
   POST https://<your controller>/oauth/register
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
   While registration is off, the operator adds the client first and the same
   request answers 403.

3. **Ask for a sign-in, and the person decides**:

   ```http
   GET https://<your controller>/oauth/authorize?response_type=code
       &client_id=<id>&redirect_uri=<uri>&state=<state>
       &code_challenge=<s256 challenge>&code_challenge_method=S256
   ```

   The controller redirects a browser to its consent page. The person signs
   in with the deployment's own sign-in (password and a second factor where
   the account or the instance asks for one, or single sign-on), reads what
   the client named and what the connection may see, and approves. An
   approval returns to the client's address with `code`; a denial is
   `error=access_denied`. This page is the whole security decision, and it is
   the part the controller does not offer to automation: no anonymous or
   self-issued registration exists, because a token here acts as one named
   person, and that person consents with the second factor behind them where
   one applies.

4. **Exchange**:

   ```http
   POST https://<your controller>/oauth/token
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

5. **Act, and end it**: `POST https://<your controller>/mcp`, Bearer in hand,
   every call audited under the person's name. Revoke at
   `https://<your controller>/oauth/revoke` (RFC 7009); the person can also
   end the connection from their own account, which stops the token at once.
   A 401 from the endpoint carries the discovery pointer in its
   `WWW-Authenticate` header, so a closed door still shows where the lock is.

[The connection walkthrough](https://zoomies.sh/connect-claude/) does the same
walk by hand, one screen at a time.

## Credentials for the REST interface

Anybody signed in to the controller creates API tokens for themselves, on
**Settings → API tokens** or with `zoomies tokens create`. A token has a role (a
viewer's unless another is asked for, and never above the creator's own) and
may carry scopes in `resource:verb` form that narrow it to part of that role;
with none, it has whatever the role allows. It may be given an expiry. Every
request presents it as a bearer token:

    Authorization: Bearer <token>

The token answers as its role, narrowed by its scopes if it has any, never
more; a route the role or the scopes do not cover is refused, and the refusal
says which one is missing. There is no agent account to create: the token is
granted by a person with an account on the instance, the instance never learns
anything else about an agent, and revoking it disconnects the agent at once.
