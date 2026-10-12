<script lang="ts">
  import Tooltip from './Tooltip.svelte';

  interface Props {
    checked?: boolean;
    /** Mixed state, for a "select all" that only covers some rows. */
    indeterminate?: boolean;
    label?: string;
    description?: string;
    id?: string;
    name?: string;
    value?: string;
    disabled?: boolean;
    /**
     * Why it cannot be ticked now, and what to do about it. With `disabled`, the
     * box is marked `aria-disabled` rather than `disabled`, so it can still be
     * pointed at and focused and the reason can be reached.
     */
    reason?: string;
    describedBy?: string;
    ariaLabel?: string;
    onchange?: (checked: boolean) => void;
    class?: string;
  }

  let {
    checked = $bindable(false),
    indeterminate = false,
    label,
    description,
    id: providedId,
    name,
    value,
    disabled = false,
    reason,
    describedBy,
    ariaLabel,
    onchange,
    class: className = '',
  }: Props = $props();

  const uid = $props.id();
  const id = $derived(providedId ?? `checkbox-${uid}`);
  const descriptionId = $derived(description ? `${id}-description` : undefined);
  let element = $state<HTMLInputElement | null>(null);
  /** Off, with a reason to give: refused rather than disabled, so it can still be reached. */
  const refused = $derived(disabled && Boolean(reason));

  $effect(() => {
    if (element) element.indeterminate = indeterminate;
  });
</script>

<div class="row {className}">
  <!--
    The reason is in the box's name when there is one (a row says why it cannot be
    selected), so the tooltip does not read it out again.
  -->
  <Tooltip bare {reason}>
    <input
      bind:this={element}
      bind:checked
      {id}
      {name}
      {value}
      disabled={disabled && !refused}
      aria-disabled={refused ? 'true' : undefined}
      class:refused
      type="checkbox"
      aria-label={ariaLabel ?? (label ? undefined : 'Select')}
      aria-describedby={[describedBy, descriptionId].filter(Boolean).join(' ') || undefined}
      onclick={(event) => {
        // aria-disabled stops nothing by itself: the box would tick.
        if (refused) event.preventDefault();
      }}
      onchange={() => onchange?.(checked)}
    />
  </Tooltip>
  {#if label}
    <div class="text">
      <label for={id}>{label}</label>
      {#if description}<p id={descriptionId}>{description}</p>{/if}
    </div>
  {/if}
</div>

<style>
  .row {
    display: flex;
    align-items: flex-start;
    gap: var(--z-space-2);
  }
  input {
    flex: none;
    width: var(--z-control-box);
    height: var(--z-control-box);
    margin: var(--z-nudge-3) 0 0;
    accent-color: var(--z-accent);
    cursor: pointer;
  }
  input:disabled,
  input.refused {
    cursor: not-allowed;
    opacity: 0.5;
  }
  .text {
    display: flex;
    flex-direction: column;
    gap: var(--z-nudge-2);
  }
  label {
    font-size: var(--z-text-base);
    color: var(--z-text);
    cursor: pointer;
  }
  /* A label that still invites a click is how a disabled row reads as broken
     rather than as a state, so it stops inviting one. */
  .row:has(input:disabled) label,
  .row:has(input.refused) label {
    color: var(--z-text-muted);
    cursor: not-allowed;
  }
  p {
    margin: 0;
    font-size: var(--z-text-xs);
    color: var(--z-text-muted);
  }
</style>
