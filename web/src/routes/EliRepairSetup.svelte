<!--
  Repair setup: the account behind a person's repairs, the repository policies and
  the form that links an account. It was a fold-out in the middle of the repairs
  page, and is a page of its own so the hub is about repairs and this is about
  whether they can happen.
-->
<script lang="ts">
  import Button from '$lib/components/Button.svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import { router } from '$lib/router';
  import RepairSetupPanel from '$lib/repairs/RepairSetupPanel.svelte';

  let panel = $state<RepairSetupPanel | null>(null);
</script>

<PageHeader
  title="Repair setup"
  subtitle="Link your GitHub account, and choose which repositories Eli may repair and when."
  breadcrumb={[{ label: 'Eli repairs', href: '/repairs' }, { label: 'Setup' }]}
  onrefresh={() => panel?.reload()}
>
  <Button href="/settings/assistant">Model providers</Button>
</PageHeader>

<RepairSetupPanel bind:this={panel} initialRepo={router.query.get('repo') ?? ''} />
