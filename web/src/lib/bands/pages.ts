/**
 * The pages that open on a band, in the order Appearance lists them. A page not
 * listed here has no band to switch off.
 */
export const BAND_PAGES = [
  { key: 'overview', label: 'Overview' },
  { key: 'runners', label: 'Runners' },
  { key: 'pools', label: 'Pools' },
  { key: 'queue', label: 'Queue' },
  { key: 'jobs', label: 'Jobs' },
  { key: 'workflows', label: 'Workflows' },
  { key: 'hosts', label: 'Hosts' },
  { key: 'providers', label: 'Providers' },
] as const;

export type BandPage = (typeof BAND_PAGES)[number]['key'];

export function isBandPage(value: unknown): value is BandPage {
  return BAND_PAGES.some((page) => page.key === value);
}
