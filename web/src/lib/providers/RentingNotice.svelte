<!--
  Why nothing is being rented, and the one click that fixes the usual cause.

  It renders nothing unless a switch is the reason, so a page mounts it without
  asking first. The sentences are the server's, shown as given. When the click
  is refused (an environment-pinned setting is the usual case) the server's own
  message stays beside the button and the notice stays, because the cause is
  still there and the message says where it lives.
-->
<script lang="ts">
  import { ApiError, updateSettings } from '$lib/api/client';
  import type { ProviderRenting } from '$lib/api/types';
  import { pluralise } from '$lib/format';
  import { toasts } from '$lib/state/toasts.svelte';
  import Button from '$lib/components/Button.svelte';
  import Field from '$lib/components/Field.svelte';
  import Input from '$lib/components/Input.svelte';
  import { clampCeiling, refusalText, rentingNotice, settingsFor } from './renting';

  interface Props {
    renting: ProviderRenting | null;
    canAdmin: boolean;
    /** Told once the settings are saved, so the page reads the new verdict. */
    onchanged: () => void;
  }

  let { renting, canAdmin, onchanged }: Props = $props();

  const notice = $derived(rentingNotice(renting, canAdmin));
  const uid = $props.id();
  const headingId = `renting-notice-${uid}`;

  // Empty until the operator types, so the field follows the suggestion as the
  // view refreshes instead of freezing the first number it saw.
  let typed = $state<string | null>(null);
  let saving = $state(false);
  let failure = $state('');

  const ceilingText = $derived(typed ?? String(notice?.action?.ceiling ?? 1));

  async function apply(): Promise<void> {
    if (!notice?.action) return;
    const ceiling = clampCeiling(Number(ceilingText), notice.action.ceiling);
    saving = true;
    failure = '';
    try {
      await updateSettings(settingsFor(notice, ceiling));
      toasts.success(
        notice.id === 'off'
          ? 'Renting is on'
          : `The fleet may rent ${pluralise(ceiling, 'machine')} at once`,
      );
      typed = null;
      onchanged();
    } catch (cause) {
      failure =
        cause instanceof ApiError
          ? refusalText(cause.message, cause.errors)
          : 'The change was not saved. Check the connection and try again.';
    } finally {
      saving = false;
    }
  }
</script>

{#if notice}
  <section class="notice" aria-labelledby={headingId}>
    <h2 id={headingId}>{notice.title}</h2>
    <p>{notice.body}</p>
    <p class="fix">{notice.fix}</p>

    {#if notice.pools.length > 0}
      <ul class="pools">
        {#each notice.pools as pool (pool.pool_id)}
          <li>
            <span class="name">{pool.pool_name}</span>
            <span class="queued">{pluralise(pool.queued, 'job')} queued</span>
            <span class="verdict">{pool.summary}</span>
          </li>
        {/each}
      </ul>
    {/if}

    {#if notice.action}
      <div class="act">
        <Field label="Machines at once">
          {#snippet children({ id, describedBy, invalid })}
            <Input
              {id}
              {describedBy}
              {invalid}
              type="number"
              min={1}
              step={1}
              bind:value={() => ceilingText, (next) => (typed = next)}
            />
          {/snippet}
        </Field>
        <Button variant="primary" loading={saving} onclick={apply}>{notice.action.label}</Button>
        {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      </div>
    {:else}
      <p class="hint">{notice.hint}</p>
    {/if}
  </section>
{/if}

<style>
  /* A pure explanation, so it takes the accent surface and no status colour. */
  .notice {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-3);
    padding: var(--z-space-4) var(--z-space-5);
    border: var(--z-border-width) solid var(--z-accent-border);
    border-radius: var(--z-radius-md);
    background: var(--z-accent-subtle);
    color: var(--z-text);
  }
  h2 {
    margin: 0;
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
  }
  p {
    margin: 0;
    font-size: var(--z-text-sm);
  }
  .fix,
  .hint {
    color: var(--z-text-muted);
  }
  .pools {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--z-space-2);
    font-size: var(--z-text-sm);
  }
  .pools li {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-1) var(--z-space-3);
  }
  .name {
    font-weight: var(--z-weight-semibold);
  }
  .queued {
    color: var(--z-text-muted);
  }
  .act {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--z-space-3);
  }
  .failure {
    flex-basis: 100%;
    color: var(--z-danger);
  }
</style>
