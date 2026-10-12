<!--
  The line a public view's visitor reads under the top bar.

  A visitor arrived from a link, often the project's own site, and the first
  question is what they are looking at and whether touching it does anything.
  The answer is short, it stays put while they look around, and it holds the
  one thing they can do about it. Accent rather than a status colour: this is
  information about the view, not about the state of anything in the fleet.
-->
<script lang="ts">
  import { Eye } from '@lucide/svelte';
  import { signIn } from './signout';
</script>

<p class="visitor" role="note">
  <Eye size={14} aria-hidden="true" />
  <span>You are looking at a public, read-only view of this fleet.</span>
  <button type="button" class="signin" onclick={signIn}>Sign in</button>
</p>

<style>
  .visitor {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--z-space-2);
    margin: 0;
    padding: var(--z-space-2) var(--z-space-4);
    border-bottom: var(--z-border-width) solid var(--z-accent-border);
    background: var(--z-accent-subtle);
    color: var(--z-text);
    font-size: var(--z-text-sm);
  }
  .visitor :global(svg) {
    color: var(--z-accent);
    flex: none;
  }
  .signin {
    margin-left: auto;
    padding: 0;
    border: 0;
    background: none;
    color: var(--z-accent);
    font: inherit;
    font-weight: var(--z-weight-medium);
    cursor: pointer;
    text-decoration: underline;
  }
  .signin:hover {
    color: var(--z-accent-hover);
  }
</style>
