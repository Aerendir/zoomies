/**
 * Which style and colour mode the gallery is showing.
 *
 * The gallery is a development tool, so it does not touch the operator's saved
 * preferences: opening it must not change what Settings, Appearance does on
 * the next real page. It reads the combination from the query string, which
 * also makes a combination something a test or a capture command can name:
 *
 *   /gallery.html?style=mission-control&mode=dark
 *
 * Style validation stays in `$lib/state/ui-style` so the gallery and the
 * product cannot disagree about what a valid style is. Mode is the same three
 * values `ThemeChoice` has, resolved here because the gallery never persists
 * one.
 */
import { resolveUIStyle, type UIStyle } from '$lib/state/ui-style';

/** The three colour modes, spelled as `ThemeChoice` spells them. */
export type GalleryMode = 'system' | 'light' | 'dark';

const MODES: readonly GalleryMode[] = ['system', 'light', 'dark'];

export interface GalleryChoice {
  style: UIStyle;
  mode: GalleryMode;
}

/**
 * An unknown mode, a missing one or an unsupported one is System: the gallery
 * should still open with something sensible rather than fail to render.
 */
export function resolveMode(value: unknown): GalleryMode {
  return MODES.find((mode) => mode === value) ?? 'system';
}

/** The combination a URL's query string asks for. */
export function readChoice(search: string): GalleryChoice {
  const params = new URLSearchParams(search);
  return {
    style: resolveUIStyle(params.get('style')),
    mode: resolveMode(params.get('mode')),
  };
}

/**
 * Put the combination on the document root.
 *
 * Removing an attribute is not the same as setting the default one: Classic
 * and System are the absence of an attribute, exactly as they are in index.html,
 * so the token cascade follows the operating system again.
 */
export function applyChoice({ style, mode }: GalleryChoice): void {
  const root = document.documentElement;
  if (style === 'classic') root.removeAttribute('data-ui-style');
  else root.setAttribute('data-ui-style', style);
  if (mode === 'system') root.removeAttribute('data-theme');
  else root.setAttribute('data-theme', mode);
}

/** The query string that names a combination, for the switcher and for links. */
export function choiceQuery({ style, mode }: GalleryChoice): string {
  const params = new URLSearchParams({ style, mode });
  return `?${params.toString()}`;
}
