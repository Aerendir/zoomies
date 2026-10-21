<!--
  Sign in.

  What this renders is decided by `/meta`, which is safe to call before anyone
  is authenticated: a password form, an SSO button, both, or -- when
  authentication has been switched off in the configuration -- a plain statement
  of that fact rather than a form that would do nothing.

  This is the first screen anyone sees, and often the only one they see while
  something is wrong, so it does the small things properly: the cursor starts in
  the first empty field, caps lock is called out before it costs an attempt, the
  password can be revealed, and a failure says which kind of failure it was --
  wrong credentials, too many attempts, or a controller that cannot be reached
  at all. Those are three different problems and only one of them is the
  operator's fault.

  The page is the shared public frame: the brand on a dark stage beside the form,
  with the header and footer every page before sign in has. On a phone the stage
  becomes a strip above the form, so the button is still on the first screen.
-->
<script lang="ts">
  import Tooltip from '$lib/components/Tooltip.svelte';
  import { tick, untrack } from 'svelte';
  import { Eye, EyeOff, Server, TriangleAlert } from '@lucide/svelte';
  import {
    ApiError,
    confirmTwoStepEnrolment,
    oidcStartUrl,
    startTwoStepEnrolment,
    verifyTwoStepSignIn,
  } from '$lib/api/client';
  import type { SignInChallenge, TwoStepSignIn } from '$lib/api/types';
  import { toasts } from '$lib/state/toasts.svelte';
  import RecoveryCodes from '$lib/twostep/RecoveryCodes.svelte';
  import TwoStepEnrol from '$lib/twostep/TwoStepEnrol.svelte';
  import { authFailureText, sentence } from '$lib/errors';
  import { router } from '$lib/router';
  import { session } from '$lib/state/session.svelte';
  import Button from '$lib/components/Button.svelte';
  import Field from '$lib/components/Field.svelte';
  import IconButton from '$lib/components/IconButton.svelte';
  import Input from '$lib/components/Input.svelte';
  import SplitPage from '$lib/public/SplitPage.svelte';

  let username = $state('');
  let password = $state('');
  let touched = $state({ username: false, password: false });
  let submitting = $state(false);
  let failure = $state<ApiError | null>(null);
  /**
   * Why a single sign-on attempt was sent back here. The controller cannot
   * render a page of its own for a failed SSO callback, so it redirects to
   * /login with the reason in the query string; it is read once and then
   * dropped from the address, so a reload does not repeat a stale complaint.
   */
  let ssoFailure = $state(
    untrack(() => new URLSearchParams(location.search).get('error')?.trim() ?? ''),
  );
  /**
   * The break-glass door. With oidc.hide_password_login on, the page offers
   * single sign-on alone -- but the day the identity provider is what is
   * broken, an administrator still needs a way in, and /login?password is it.
   * The server refuses a password below administrator either way; this only
   * decides whether the form is drawn.
   */
  const passwordAsked = untrack(() => new URLSearchParams(location.search).has('password'));
  const passwordForm = $derived(!session.meta?.password_login_hidden || passwordAsked);
  /**
   * The MCP consent screen shows this form in place when nobody is signed
   * in, and single sign-on leaves the page, so it is told where to come back.
   */
  const returnTo = $derived(
    router.pathname === '/oauth/consent'
      ? `/oauth/consent?request=${encodeURIComponent(router.param('request'))}`
      : undefined,
  );
  let revealed = $state(false);
  let capsLock = $state(false);
  let usernameInput = $state<HTMLInputElement | null>(null);
  let passwordInput = $state<HTMLInputElement | null>(null);

  /*
    The second step. The password was right and the server answered 202: the
    pending sign-in is in a cookie this page cannot read, and all it knows is
    which step comes next -- a code, or setting up an authenticator because the
    instance requires one -- and for whom.
  */
  let pending = $state<SignInChallenge | null>(null);
  let code = $state('');
  let codeError = $state('');
  let codeInput = $state<HTMLInputElement | null>(null);
  /** The finished enrolment, held back until its recovery codes have been seen. */
  let enrolled = $state<TwoStepSignIn | null>(null);

  $effect(() => {
    if (pending?.two_step === 'verify' && codeInput) untrack(() => codeInput?.focus());
  });

  /** Back to the password, saying why. */
  function startAgain(cause?: ApiError): void {
    pending = null;
    enrolled = null;
    code = '';
    codeError = '';
    failure = cause ?? null;
    password = '';
    touched = { ...touched, password: false };
    void tick().then(() => passwordInput?.focus());
  }

  async function finish(result: TwoStepSignIn): Promise<void> {
    await session.completeTwoStep(result);
    if (result.recovery_code_used) {
      const left = result.recovery_codes_left ?? 0;
      toasts.success(
        'Signed in with a recovery code',
        `That code is spent; ${left} ${left === 1 ? 'is' : 'are'} left. Issue new ones from Settings, Account.`,
      );
    }
    router.navigate(destination);
  }

  async function submitCode(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (code.trim() === '') {
      codeError = 'Enter the code from your app, or a recovery code.';
      codeInput?.focus();
      return;
    }
    submitting = true;
    codeError = '';
    try {
      await finish(await verifyTwoStepSignIn({ code }));
    } catch (cause) {
      const err =
        cause instanceof ApiError
          ? cause
          : new ApiError({ status: 0, code: 'internal', message: 'Sign-in failed. Try again.' });
      // A refusal pointed at the field is a wrong or spent code: try the next
      // one. Anything else -- the five minutes are up, too many tries, the
      // limiter -- means the password has to be typed again.
      const wrongCode = err.fieldErrors().code;
      if (wrongCode) {
        codeError = sentence(wrongCode);
        code = '';
        void tick().then(() => codeInput?.focus());
      } else if (err.status === 429 || err.status === 401) {
        startAgain(err);
      } else {
        codeError = authFailureText(err);
      }
    } finally {
      submitting = false;
    }
  }

  async function confirmEnrolment(value: string): Promise<void> {
    try {
      enrolled = await confirmTwoStepEnrolment({ code: value });
    } catch (cause) {
      if (cause instanceof ApiError && !cause.fieldErrors().code && cause.status === 401) {
        startAgain(cause);
        return;
      }
      throw cause;
    }
  }

  /**
   * Where to go once signed in.
   *
   * Anything other than the overview is a deep link somebody followed while
   * signed out -- GitHub returning an operator to the App setup address with a
   * single-use code in it, most importantly -- and sending them to the overview
   * throws it away.
   */
  const destination = untrack(() =>
    location.pathname === '/login' ? '/' : location.pathname + location.search,
  );

  const meta = $derived(session.meta);

  let returning = $state(false);

  /** Back to the public view, as a visitor. */
  async function keepLooking(): Promise<void> {
    returning = true;
    try {
      if (!session.visitor) await session.refresh();
      router.navigate('/');
    } finally {
      returning = false;
    }
  }

  /**
   * Whether this is the shell's /login route rather than the sign-in screen.
   *
   * The router serves /login to somebody already in the product too -- that is
   * the only way the "no sign-in required" statement is ever seen -- and there
   * the page sits inside the navigation and the footer, which already say what
   * Zoomies is. The brand panel and the full-window layout are for the screen
   * that has nothing else on it.
   */
  const embedded = $derived(session.phase === 'ready' && router.pathname !== '/login');

  /**
   * The address being signed in to, as the browser reached it. Somebody with
   * accounts on a staging and a production controller should not have to read
   * the address bar to know which one they are about to type a password into.
   * The browser's own idea of the host is the one that is always right: the
   * configured external URL can be missing, can be a loopback address reached
   * through a tunnel, or can name an address this browser did not use -- and
   * it is this address the password is about to be sent to.
   */
  const host = location.host;

  $effect(() => {
    if (ssoFailure) router.setQuery({ error: null });
  });

  const usernameError = $derived(
    touched.username && username.trim() === '' ? 'Enter your username.' : undefined,
  );
  const passwordError = $derived(
    touched.password && password === '' ? 'Enter your password.' : undefined,
  );

  /**
   * What actually went wrong, in the operator's terms. A controller that never
   * answered and a password that was refused look identical in a generic
   * "sign-in failed", and they need completely different next steps.
   *
   * A 401 and a 403 are shown in the server's own words. It distinguishes a
   * wrong password from a disabled account and from an account that signs in
   * through SSO, and a 403 on this route is the origin check refusing the
   * request -- a proxy or external_url problem, which "wrong password" would
   * send the operator off to fix in entirely the wrong place.
   */
  const failureText = $derived.by(() => {
    if (!failure) return ssoFailure ? sentence(ssoFailure) : '';
    if (failure.status === 429) {
      return 'Too many sign-in attempts from this address. Wait a minute, then try again.';
    }
    return authFailureText(failure);
  });

  /*
    The cursor starts where there is something to type: a browser that has
    filled the username in should not make the operator tab past it.

    `placed` is a plain variable rather than state on purpose. The effect must
    fire once, when the fields first exist, and never again -- tracking it, or
    reading `username` reactively, would move the cursor out of the field
    somebody is typing in.
  */
  let placed = false;
  $effect(() => {
    if (placed || !usernameInput || meta?.auth_disabled) return;
    placed = true;
    // Inside the shell the route change moves focus to the page heading a
    // moment later, which blurred the field and marked it as left empty, so a
    // public view's visitor opening this page was greeted with "Enter your
    // username." before typing anything. The shell owns focus there.
    if (embedded) return;
    // Do not open a phone keyboard or scroll the form before the user chooses a field.
    if (matchMedia('(max-width: 960px), (pointer: coarse)').matches) return;
    untrack(() => (username.trim() === '' ? usernameInput : passwordInput))?.focus();
  });

  /**
   * Caps lock costs an attempt and, at the rate limit, a minute. The state is
   * only knowable from a key event, so it is read from every one the two
   * fields see and cleared when the password field is left.
   */
  function readCapsLock(event: KeyboardEvent): void {
    if (typeof event.getModifierState !== 'function') return;
    capsLock = event.getModifierState('CapsLock');
  }

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    touched = { username: true, password: true };
    if (username.trim() === '' || password === '') {
      // Derived from the model, not from the DOM: Svelte batches state into a
      // microtask, so a query for `aria-invalid="true"` here matches nothing on
      // the first submit of an empty form -- which is precisely the keyboard
      // user pressing Enter that this line exists for.
      (username.trim() === '' ? usernameInput : passwordInput)?.focus();
      return;
    }
    submitting = true;
    failure = null;
    ssoFailure = '';
    try {
      const next = await session.login(username.trim(), password);
      if (next) {
        pending = next;
        password = '';
        revealed = false;
        return;
      }
      router.navigate(destination);
    } catch (cause) {
      failure =
        cause instanceof ApiError
          ? cause
          : new ApiError({ status: 0, code: 'internal', message: 'Sign-in failed. Try again.' });
      // The password is cleared, so that is where the cursor belongs: retyping
      // it is the next thing to do whichever failure this was. The field is
      // untouched again with it -- "Enter your password." under a field this
      // page emptied itself is an accusation, and it would sit directly below
      // the banner that already said what went wrong.
      password = '';
      touched = { ...touched, password: false };
      revealed = false;
      // After the flush: the failure box appears above the form and the field
      // is emptied in the same update, and focus set before that lands on an
      // element the render is about to move.
      void tick().then(() => passwordInput?.focus());
    } finally {
      submitting = false;
    }
  }
</script>

{#snippet column()}
  <div class="form-column">
    {#if !embedded}
      <Tooltip bare text="The Zoomies instance at this address">
        <p class="instance">
          <Server size={12} aria-hidden="true" />
          <span class="sr-only">This instance:</span>
          <span class="host">{host}</span>
        </p>
      </Tooltip>
    {/if}

    {#if meta?.auth_disabled}
      <h1>No sign-in required</h1>
      <p class="lede">
        Authentication is switched off in this instance's configuration, so there is nothing to sign
        in to. Anyone who can reach this address has full access.
      </p>
      <Button variant="primary" size="lg" full href="/">Continue to the dashboard</Button>
      <p class="note">
        Turn authentication back on in the configuration file before this instance is reachable by
        anyone you do not trust.
      </p>
    {:else if pending?.two_step === 'verify'}
      <h1>Enter your code</h1>
      <p class="lede">
        Open your authenticator app and type the code it shows for Zoomies. A recovery code works
        too.
      </p>
      <form onsubmit={submitCode} novalidate>
        <Field
          label="Code"
          hint="Signing in as {pending.username}. This step expires after five minutes."
          error={codeError || undefined}
        >
          {#snippet children({ id, describedBy, invalid })}
            <Input
              bind:value={code}
              bind:element={codeInput}
              {id}
              {describedBy}
              {invalid}
              size="lg"
              name="code"
              inputmode="text"
              autocomplete="one-time-code"
              autocapitalize="none"
              spellcheck={false}
              mono
              disabled={submitting}
            />
          {/snippet}
        </Field>
        <Button type="submit" variant="primary" size="lg" full loading={submitting}>
          Verify and sign in
        </Button>
      </form>
      <p class="note">
        Lost your phone and your recovery codes? An administrator can reset two-step verification
        for you. <button type="button" class="linkish" onclick={() => startAgain()}
          >Start again</button
        >
      </p>
    {:else if pending?.two_step === 'enrol'}
      {#if enrolled?.recovery_codes}
        <h1>Save your recovery codes</h1>
        <p class="lede">You are signed in. Keep these before you go any further.</p>
        <div class="stack">
          <RecoveryCodes codes={enrolled.recovery_codes} account={pending.username} />
          <Button variant="primary" size="lg" full onclick={() => enrolled && finish(enrolled)}>
            I have saved them, continue
          </Button>
        </div>
      {:else}
        <h1>Set up two-step verification</h1>
        <p class="lede">
          This instance asks for a code from an authenticator app after your password. Set one up to
          finish signing in as {pending.username}.
        </p>
        <TwoStepEnrol
          load={startTwoStepEnrolment}
          confirm={confirmEnrolment}
          confirmLabel="Turn on and sign in"
          size="lg"
        />
        <p class="note">
          <button type="button" class="linkish" onclick={() => startAgain()}>Start again</button>
        </p>
      {/if}
    {:else}
      <h1>Sign in to Zoomies</h1>
      <p class="lede">Manage your runner fleet.</p>

      {#if failureText}
        <p class="failure" role="alert">
          <TriangleAlert size={15} aria-hidden="true" />
          <span>{failureText}</span>
        </p>
      {/if}

      {#if meta?.oidc_enabled}
        <Button href={oidcStartUrl(returnTo)} variant="primary" size="lg" full
          >{meta.oidc_label ?? 'Sign in with SSO'}</Button
        >
        {#if passwordForm}
          <div class="divider"><span>or use a local account</span></div>
        {/if}
        {#if !passwordForm}
          <p class="note">
            An administrator who cannot use single sign-on can
            <a href="/login?password">sign in with a password</a>.
          </p>
        {/if}
      {/if}

      {#if passwordForm}
        <form onsubmit={submit} novalidate>
          <Field label="Username" error={usernameError}>
            {#snippet children({ id, describedBy, invalid })}
              <Input
                bind:value={username}
                bind:element={usernameInput}
                {id}
                {describedBy}
                {invalid}
                size="lg"
                name="username"
                autocomplete="username"
                autocapitalize="none"
                spellcheck={false}
                disabled={submitting}
                onkeydown={readCapsLock}
                onblur={() => (touched = { ...touched, username: true })}
              />
            {/snippet}
          </Field>

          <!-- The caps-lock warning goes in `notice`, not `hint`: hint is the
             branch Field drops the moment there is an error, which is exactly
             when caps lock is most likely to be the reason for one.

             The hint holds the row an error takes, so "Enter your password."
             appearing on blur cannot move the button out from under a pointer
             on its way to it. It used to read "the one you chose when this
             instance was set up", which is true of the first administrator
             and of nobody else: every other password was set by an
             administrator, and an administrator is also who can reset it. -->
          <Field
            label="Password"
            hint="Forgotten it? An administrator can reset it."
            error={passwordError}
            notice={capsLock ? 'Caps lock is on.' : undefined}
          >
            {#snippet children({ id, describedBy, invalid })}
              <Input
                bind:value={password}
                bind:element={passwordInput}
                {id}
                {describedBy}
                {invalid}
                size="lg"
                type={revealed ? 'text' : 'password'}
                name="password"
                autocomplete="current-password"
                disabled={submitting}
                onkeydown={readCapsLock}
                onblur={() => {
                  touched = { ...touched, password: true };
                  capsLock = false;
                }}
              >
                {#snippet trailing()}
                  <IconButton
                    icon={revealed ? EyeOff : Eye}
                    label={revealed ? 'Hide password' : 'Show password'}
                    pressed={revealed}
                    disabled={submitting}
                    onclick={() => {
                      revealed = !revealed;
                      passwordInput?.focus();
                    }}
                  />
                {/snippet}
              </Input>
            {/snippet}
          </Field>

          <Button
            type="submit"
            variant={meta?.oidc_enabled ? 'secondary' : 'primary'}
            size="lg"
            full
            loading={submitting}
          >
            Sign in
          </Button>
        </form>
      {/if}

      {#if meta?.public_view}
        <!--
          A visitor who opened this page and changed their mind, or somebody
          who just signed out of a public fleet, can go back to looking. The
          session is asked again rather than assumed: it is what turns this
          browser back into a visitor.
        -->
        <Button
          variant="ghost"
          size="lg"
          full
          loading={returning}
          onclick={() => void keepLooking()}
        >
          Continue without signing in
        </Button>
      {/if}

      <details class="account-help">
        <summary>Need help signing in?</summary>
        <p>
          <strong>Forgotten your password?</strong> An administrator of this instance can reset it.
        </p>
        <p><strong>Need an account?</strong> Ask an administrator of this instance to add you.</p>
      </details>
    {/if}
  </div>
{/snippet}

{#if embedded}
  <div class="embedded">{@render column()}</div>
{:else}
  <SplitPage
    eyebrow="Your infrastructure. Your control."
    headline={['A fresh runner.', 'For every job.']}
    lede="GitHub Actions runners on machines you own. Managed from one place."
    version={meta?.version}
    note={meta?.auth_disabled
      ? undefined
      : 'No account? Ask an administrator of this instance to add you.'}
  >
    {@render column()}
  </SplitPage>
{/if}

<style>
  .form-column {
    display: flex;
    flex-direction: column;
    width: 100%;
    max-width: var(--z-width-dialog-sm);
    min-width: 0;
  }
  .instance {
    display: flex;
    align-items: center;
    gap: var(--z-space-2);
    margin: 0 0 var(--z-space-6);
    font-size: var(--z-text-xs);
    color: var(--z-text-muted);
  }
  .instance :global(svg) {
    flex: none;
  }
  .host {
    overflow-wrap: anywhere;
    min-width: 0;
  }
  h1 {
    margin: 0;
    font-size: var(--z-text-2xl);
    line-height: var(--z-leading-2xl);
    font-weight: var(--z-weight-semibold);
    letter-spacing: var(--z-tracking-tight);
    color: var(--z-text);
  }
  .lede {
    margin: var(--z-space-1) 0 var(--z-space-6);
    font-size: var(--z-text-base);
    line-height: var(--z-leading-base);
    color: var(--z-text-muted);
    text-wrap: pretty;
  }
  form {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-4);
  }
  form :global(.btn) {
    margin-top: var(--z-space-1);
  }
  .failure {
    display: flex;
    align-items: flex-start;
    gap: var(--z-space-2);
    margin: 0 0 var(--z-space-5);
    padding: var(--z-space-3);
    border: var(--z-border-width) solid var(--z-danger-border);
    border-radius: var(--z-radius-md);
    background: var(--z-danger-subtle);
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
    color: var(--z-text);
  }
  .failure span {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .failure :global(svg) {
    flex: none;
    margin-top: var(--z-nudge-3);
    color: var(--z-danger);
  }
  .divider {
    display: flex;
    align-items: center;
    gap: var(--z-space-3);
    margin: var(--z-space-5) 0;
    color: var(--z-text-subtle);
    font-size: var(--z-text-xs);
  }
  .divider::before,
  .divider::after {
    content: '';
    flex: 1;
    height: var(--z-border-width);
    background: var(--z-border);
  }
  .note {
    margin: var(--z-space-4) 0 0;
    font-size: var(--z-text-xs);
    line-height: var(--z-leading-xs);
    color: var(--z-text-muted);
  }
  .stack {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-5);
  }
  /* "Start again" reads as a link in a sentence, and is a button because it
     changes this page rather than going anywhere. */
  .linkish {
    padding: 0;
    border: 0;
    background: none;
    color: var(--z-accent);
    font: inherit;
    text-decoration: underline;
    text-underline-offset: var(--z-underline-offset);
    cursor: pointer;
  }
  .linkish:hover {
    color: var(--z-accent-hover);
  }
  .linkish:focus-visible {
    outline: var(--z-focus-width) solid var(--z-focus-colour);
    outline-offset: var(--z-focus-offset);
    border-radius: var(--z-radius-sm);
  }

  .account-help {
    border-top: var(--z-border-width) solid var(--z-border);
    margin-top: var(--z-space-6);
    padding-top: var(--z-space-4);
    color: var(--z-text-muted);
    font-size: var(--z-text-xs);
    line-height: var(--z-leading-base);
  }
  .account-help summary {
    cursor: pointer;
    min-height: var(--z-control-touch);
    display: flex;
    align-items: center;
    justify-content: space-between;
    list-style: none;
  }
  .account-help summary::-webkit-details-marker {
    display: none;
  }
  .account-help summary::after {
    content: '+';
    font-size: var(--z-text-lg);
  }
  .account-help[open] summary::after {
    content: '−';
  }
  .account-help summary:focus-visible {
    outline: var(--z-focus-width) solid var(--z-focus-colour);
    outline-offset: var(--z-focus-offset);
    border-radius: var(--z-radius-sm);
  }
  .account-help p {
    margin: var(--z-space-3) 0 0;
  }
</style>
