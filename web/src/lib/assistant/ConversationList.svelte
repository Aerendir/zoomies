<!--
  The kept conversations, in place of the thread: pick one up where it was
  left, or let it go. They are the person's own, listed newest first, and
  kept for the window the administrator set or until deleted here.
-->
<script lang="ts">
  import { ArrowLeft, Trash2 } from '@lucide/svelte';
  import Button from '$lib/components/Button.svelte';
  import IconButton from '$lib/components/IconButton.svelte';
  import RelativeTime from '$lib/components/RelativeTime.svelte';
  import { eli } from './eli.svelte';
</script>

<section class="history" aria-labelledby="eli-history-heading">
  <header>
    <Button variant="ghost" size="sm" icon={ArrowLeft} onclick={() => eli.closeHistory()}
      >Back</Button
    >
    <h3 id="eli-history-heading">Past conversations</h3>
  </header>
  {#if eli.historyError}
    <p class="notice" role="alert">{eli.historyError}</p>
  {/if}
  {#if eli.conversations.length === 0}
    <p class="empty">
      Nothing kept yet. A conversation is kept once Eli has answered, under your account, so a
      reload or another device finds it; it is removed when you delete it here or when it expires.
    </p>
  {:else}
    <ul aria-label="Past conversations">
      {#each eli.conversations as kept (kept.id)}
        <li class:current={kept.id === eli.conversation.id}>
          <button type="button" class="resume" onclick={() => void eli.resume(kept.id)}>
            <span class="title">{kept.title}</span>
            <span class="meta">
              {kept.message_count}
              {kept.message_count === 1 ? 'message' : 'messages'} ·
              <RelativeTime value={kept.updated_at} plain />
            </span>
          </button>
          <IconButton
            icon={Trash2}
            size="sm"
            label="Delete: {kept.title}"
            onclick={() => void eli.remove(kept.id)}
          />
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .history {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-3);
    height: 100%;
    overflow: auto;
    padding: var(--z-space-3);
  }
  header {
    display: flex;
    align-items: center;
    gap: var(--z-space-2);
  }
  h3 {
    margin: 0;
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
  }
  .notice,
  .empty {
    margin: 0;
    font-size: var(--z-text-sm);
    color: var(--z-text-muted);
  }
  ul {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-1);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--z-space-2);
    border-radius: var(--z-radius-md);
  }
  li.current {
    background: var(--z-surface-sunken);
  }
  .resume {
    display: flex;
    flex: 1;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--z-nudge-1);
    min-width: 0;
    padding: var(--z-space-2);
    font: inherit;
    text-align: left;
    color: inherit;
    background: none;
    border: 0;
    border-radius: var(--z-radius-md);
    cursor: pointer;
  }
  .resume:hover {
    background: var(--z-surface-sunken);
  }
  .title {
    overflow: hidden;
    width: 100%;
    font-size: var(--z-text-sm);
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .meta {
    font-size: var(--z-text-xs);
    color: var(--z-text-muted);
  }
</style>
