import { storage } from './prefs.svelte';
import { syncBrowserChrome, theme } from './theme.svelte';
import { resolveUIStyle, UI_STYLE_KEY, type UIStyle } from './ui-style';

class VisualStyle {
  #choice = $state<UIStyle>(resolveUIStyle(storage.get(UI_STYLE_KEY)));

  get choice(): UIStyle {
    return this.#choice;
  }

  set(value: UIStyle): void {
    const choice = resolveUIStyle(value);
    this.#choice = choice;
    if (typeof document === 'undefined') return;
    if (choice === 'classic') {
      document.documentElement.removeAttribute('data-ui-style');
      storage.remove(UI_STYLE_KEY);
    } else {
      document.documentElement.setAttribute('data-ui-style', choice);
      storage.set(UI_STYLE_KEY, choice);
    }
    // Only root tokens change: editors, the fleet cache and Eli keep their state.
    syncBrowserChrome(theme.choice);
  }
}

export const visualStyle = new VisualStyle();
