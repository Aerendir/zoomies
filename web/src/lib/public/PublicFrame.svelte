<!--
  The frame every page before sign in shares: a header, the page, and a footer.

  Sign in, the first account, the consent screen and the status page were each
  their own layout, drawn from scratch, with a different amount of the brand on
  each. A visitor who meets two of them should feel they are in one product, and
  the way to do that is for them to be one frame with different content in it.

  The header carries the brand and the appearance choice and nothing else: there
  is no navigation here, because there is nowhere to navigate to until somebody
  has signed in. The footer is the dark panel the stage is, with what the
  project is, where it lives, who wrote it and which build this is. The links
  all leave the product, so each opens a new tab and says so.
-->
<script lang="ts">
  import { Globe } from '@lucide/svelte';
  import type { Snippet } from 'svelte';
  import LogoArt from '$lib/components/LogoArt.svelte';
  import GithubMark from '$lib/icons/GithubMark.svelte';
  import { DEVELOPER_NAME, DEVELOPER_URL, REPO_URL, SITE_HOST, SITE_URL } from '$lib/links';
  import { DEFAULT_LOGO_TREATMENT } from '$lib/state/logo-treatment';
  import ThemeSwitch from './ThemeSwitch.svelte';

  interface Props {
    /** The page. */
    children: Snippet;
    /** The build this controller is, if it has said. */
    version?: string;
    /** A line for the footer's left, in the pages' own voice. */
    note?: string;
  }
  const { children, version, note }: Props = $props();
</script>

<div class="public">
  <header class="bar">
    <!-- The small paw and wordmark carry no backdrop, so the treatment is moot here. -->
    <LogoArt variant="full" size={28} label="Zoomies" treatment={DEFAULT_LOGO_TREATMENT} />
  </header>

  <div class="content">{@render children()}</div>

  <!-- Drawn in the header's row, at its right, but read after the page: a person
       who arrived to sign in is on the first field, and the appearance choice
       is not what stands between them and it. -->
  <div class="appearance"><ThemeSwitch /></div>

  <footer class="foot">
    <p class="imprint">
      <span class="product">Zoomies</span>
      {#if version}<span class="version">{version}</span>{/if}
      {#if note}<span class="note">{note}</span>{/if}
    </p>
    <nav class="links" aria-label="About Zoomies">
      <a href={SITE_URL} target="_blank" rel="noopener noreferrer">
        <Globe size={15} aria-hidden="true" />
        {SITE_HOST}<span class="sr-only"> (opens in a new tab)</span>
      </a>
      <a href={REPO_URL} target="_blank" rel="noopener noreferrer">
        <GithubMark size={15} aria-hidden="true" />
        GitHub<span class="sr-only"> (opens in a new tab)</span>
      </a>
      <span class="credit">
        Developed by
        <a href={DEVELOPER_URL} target="_blank" rel="noopener noreferrer">
          {DEVELOPER_NAME}<span class="sr-only"> (opens in a new tab)</span>
        </a>
      </span>
    </nav>
  </footer>
</div>

<style>
  .public {
    /* One gutter and one measure for all three, so the logo, the stage and the
       footer share their edges, as they do on the portal's pages. */
    --gutter: var(--z-space-8);
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    min-height: 100vh;
    min-height: 100dvh;
    background: var(--z-surface);
    color: var(--z-text);
  }
  .bar,
  .appearance,
  .content,
  .foot {
    grid-column: 1;
    box-sizing: border-box;
    width: min(100% - 2 * var(--gutter), var(--z-width-public));
    margin-inline: auto;
  }
  .bar,
  .appearance {
    grid-row: 1;
    display: flex;
    align-items: center;
    padding-block: var(--z-space-4);
  }
  /* Both sit in the first row; the switch is the right-hand end of it. */
  .appearance {
    justify-content: flex-end;
    pointer-events: none;
  }
  .appearance :global(.switch) {
    pointer-events: auto;
  }
  .content {
    grid-row: 2;
    min-width: 0;
  }
  .foot {
    grid-row: 3;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--z-space-4) var(--z-space-8);
    margin-block: var(--z-space-8);
    padding: var(--z-space-8) var(--z-space-12);
    border: var(--z-border-width) solid var(--z-panel-border);
    border-radius: var(--z-radius-xl);
    background: var(--z-panel-bg);
    color: var(--z-panel-text);
  }
  .imprint {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--z-space-1) var(--z-space-3);
    margin: 0;
    font-size: var(--z-text-xs);
    color: var(--z-panel-text-subtle);
  }
  .product {
    font-weight: var(--z-weight-semibold);
    color: var(--z-panel-text-muted);
  }
  .version {
    font-family: var(--z-font-mono);
    overflow-wrap: anywhere;
  }
  .links {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-3) var(--z-space-5);
    font-size: var(--z-text-xs);
    color: var(--z-panel-text-muted);
  }
  .links a {
    display: inline-flex;
    align-items: center;
    gap: var(--z-space-2);
    color: inherit;
    text-decoration: none;
  }
  .links a:hover {
    color: var(--z-panel-text);
    text-decoration: underline;
  }
  .links a:focus-visible {
    outline: var(--z-focus-width) solid var(--z-brand-runner-blue);
    outline-offset: var(--z-focus-offset);
    border-radius: var(--z-radius-sm);
  }
  .credit {
    color: var(--z-panel-text-subtle);
  }
  .credit a {
    font-weight: var(--z-weight-semibold);
  }
  /* The status page loads no global stylesheet, so the visually hidden text the
     footer's links carry is defined here, where it is used. */
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
    border: 0;
  }
  @media (max-width: 800px) {
    .public {
      --gutter: var(--z-space-4);
    }
    .bar {
      padding-block: var(--z-space-4);
    }
    .foot {
      margin-block: var(--z-space-6);
      padding: var(--z-space-6);
    }
  }
</style>
