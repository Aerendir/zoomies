<!--
  Pick a different pull request: paste its address and the repair page moves to
  it. The address is read with the same parser the rest of the repair pages use,
  which accepts a GitHub pull request address and nothing else, so a pasted
  string can never name somewhere the controller would be asked to fetch.
-->
<script lang="ts">
  import Button from '$lib/components/Button.svelte';
  import Dialog from '$lib/components/Dialog.svelte';
  import Field from '$lib/components/Field.svelte';
  import Input from '$lib/components/Input.svelte';
  import { router } from '$lib/router';
  import { parseRepairTarget } from './target';
  import { targetHref } from './workspace';

  let { open = $bindable(false) }: { open: boolean } = $props();

  let value = $state('');
  let error = $state('');

  const FORM = 'repair-target-form';

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    const target = parseRepairTarget(value);
    if (!target) {
      error = 'Paste a GitHub pull request URL, such as https://github.com/owner/repo/pull/123.';
      return;
    }
    error = '';
    value = '';
    open = false;
    router.navigate(targetHref({ repo: target.repo, pull: target.pull_number }));
  }

  // A dialog opened again starts clean, not with the last mistake still on it.
  $effect(() => {
    if (!open) error = '';
  });
</script>

<Dialog
  bind:open
  title="Repair a different PR"
  description="Paste the pull request's address. Eli checks it before anything is spent."
>
  <form id={FORM} onsubmit={submit}>
    <Field label="Pull request" hint="For example https://github.com/owner/repo/pull/123." {error}>
      {#snippet children({ id, describedBy, invalid })}
        <Input
          {id}
          {describedBy}
          {invalid}
          bind:value
          placeholder="https://github.com/owner/repo/pull/123"
          autocomplete="off"
          spellcheck={false}
        />
      {/snippet}
    </Field>
  </form>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
    <Button variant="primary" type="submit" form={FORM}>Check this PR</Button>
  {/snippet}
</Dialog>
