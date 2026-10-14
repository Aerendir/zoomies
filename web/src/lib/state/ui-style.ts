export type UIStyle = 'classic' | 'mission-control';

export const UI_STYLE_KEY = 'zoomies.ui-style';

export const UI_STYLE_OPTIONS: readonly { value: UIStyle; label: string; description: string }[] = [
  {
    value: 'classic',
    label: 'Classic',
    description: 'The familiar Zoomies palette and compact type.',
  },
  {
    value: 'mission-control',
    label: 'Mission Control',
    description: 'Navy surfaces, cool blue controls and roomier type. Same Zoomies personality.',
  },
];

/** Old and unknown preferences must never opt a browser into a new style. */
export function resolveUIStyle(value: unknown): UIStyle {
  return value === 'mission-control' ? value : 'classic';
}
