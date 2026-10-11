<script lang="ts">
  import PageHeader from '$lib/components/PageHeader.svelte';
  import EliRepairs from '$lib/settings/EliRepairs.svelte';
  import Button from '$lib/components/Button.svelte';
  import { router } from '$lib/router';
</script>

<PageHeader
  title="Eli repairs"
  subtitle="Catch failed PR checks, review repair offers and follow the result."
>
  <Button href="/settings/assistant">Model providers</Button>
</PageHeader>
<EliRepairs
  initialRepo={router.query.get('repo') ?? ''}
  initialPull={Number(router.query.get('pull') ?? 0)}
  initialRun={Number(router.query.get('run') ?? 0)}
/>
