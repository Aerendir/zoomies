<!--
  What the repair page says about a repository that repairs itself, above the
  decision it bears on: that the repository does this on its own, and, for an
  administrator, whether Eli already tried this failure or why it has not started.

  A person about to press Fix with Eli on such a failure is otherwise one press
  away from repeating an attempt Eli already made, or waiting for one it will not
  start. The wording is chosen in automatic.ts, because what may be said depends
  on who is reading.
-->
<script lang="ts">
  import type { AutomaticNote } from './automatic';

  interface Props {
    note: AutomaticNote;
  }

  let { note }: Props = $props();

  const headingId = $props.id();
</script>

<section class="note" aria-labelledby={headingId}>
  <h3 id={headingId}>{note.title}</h3>
  <p>{note.body}</p>
  {#if note.attemptId}
    <a href="/repairs/{encodeURIComponent(note.attemptId)}">View the attempt</a>
  {/if}
</section>

<style>
  .note {
    margin-bottom: var(--z-space-4);
    padding: var(--z-space-4) var(--z-space-5);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    background: var(--z-surface-sunken);
  }
  h3 {
    margin: 0;
    font-size: var(--z-text-sm);
    font-weight: var(--z-weight-semibold);
    line-height: var(--z-leading-sm);
  }
  p {
    margin: var(--z-space-2) 0 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
    overflow-wrap: anywhere;
  }
  a {
    display: inline-block;
    margin-top: var(--z-space-2);
    color: var(--z-accent);
    font-size: var(--z-text-sm);
  }
</style>
