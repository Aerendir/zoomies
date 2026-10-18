<!--
  A page that has nothing to show for its address says so on one card: what is
  missing, why that may be, and the one way on. It is where a repair, an offer or
  a target is not there, so a reader is never left on a blank page.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    heading: string;
    /** The explanation, as text. */
    children: Snippet;
    /** The way on, usually one button. */
    action?: Snippet;
  }

  let { heading, children, action }: Props = $props();

  const headingId = $props.id();
</script>

<section class="notice" aria-labelledby={headingId}>
  <h2 id={headingId}>{heading}</h2>
  <p>{@render children()}</p>
  {#if action}{@render action()}{/if}
</section>

<style>
  .notice {
    display: grid;
    justify-items: start;
    gap: var(--z-space-3);
    max-width: var(--z-measure-prose);
    padding: var(--z-space-6);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    background: var(--z-surface);
  }
  h2 {
    margin: 0;
    font-size: var(--z-text-lg);
    font-weight: var(--z-weight-semibold);
  }
  p {
    margin: 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
  }
</style>
