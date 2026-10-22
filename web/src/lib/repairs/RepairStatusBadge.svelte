<!--
  A repair's state as a badge: the tone and shape every status in Zoomies uses,
  and the words the history already uses, so the badge and the line beside it
  never disagree.
-->
<script lang="ts">
  import Badge from '$lib/components/Badge.svelte';
  import { repairStatus } from '$lib/status';

  let { state, size = 'md' }: { state: string; size?: 'sm' | 'md' } = $props();

  const status = $derived(repairStatus(state));
</script>

<Badge {status} {size} />
