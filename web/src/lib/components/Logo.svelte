<!-- The logo, drawn with the backdrop the operator chose in Appearance.

     All the drawing is in LogoArt.svelte, which is told which backdrop to use.
     This only asks the preference, so that every caller in the app keeps
     writing <Logo /> and a page without preferences can use LogoArt directly. -->
<script lang="ts">
  import { prefs } from '../state/prefs.svelte';
  import type { LogoTreatment } from '../state/logo-treatment';
  import LogoArt from './LogoArt.svelte';

  interface Props {
    variant?: 'mark' | 'wordmark' | 'full' | 'lockup';
    size?: number;
    label?: string;
    /** Appearance previews can show an option without changing the saved choice. */
    treatment?: LogoTreatment;
    class?: string;
  }
  const { treatment, ...rest }: Props = $props();
</script>

<LogoArt {...rest} treatment={treatment ?? prefs.logoTreatment} />
