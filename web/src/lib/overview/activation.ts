import type { Installation } from '../api/types';

/** A repository name never becomes an arbitrary external URL. */
export function activationRepository(value: string, installation: Installation): string | null {
  const repo = value.trim();
  if (!/^[A-Za-z0-9][A-Za-z0-9-]*\/[A-Za-z0-9_.-]+$/.test(repo)) return null;
  if (['.', '..'].includes(repo.split('/')[1] ?? '')) return null;
  const target = installation.target?.toLowerCase();
  if (!target) return null;
  if (installation.target_type === 'repo') return repo.toLowerCase() === target ? repo : null;
  return repo.split('/')[0]?.toLowerCase() === target ? repo : null;
}

export function activationActionsUrl(repo: string, installation: Installation): string | null {
  try {
    const base = new URL(
      installation.web_url || (installation.enterprise ? '' : 'https://github.com'),
    );
    if (base.protocol !== 'https:' || base.username || base.password) return null;
    return `${base.origin}/${repo}/actions/workflows/zoomies-test.yml`;
  } catch {
    return null;
  }
}
