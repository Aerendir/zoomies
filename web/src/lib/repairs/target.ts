/** Only GitHub PR addresses are accepted, never arbitrary fetch destinations. */
export function parseRepairTarget(value: string): { repo: string; pull_number: number } | null {
  try {
    const url = new URL(value.trim());
    if (
      url.protocol !== 'https:' ||
      url.hostname !== 'github.com' ||
      url.username ||
      url.password ||
      url.port
    )
      return null;
    const match = /^\/([^/]+)\/([^/]+)\/pull\/([1-9]\d*)\/?$/.exec(url.pathname);
    if (!match || !Number.isSafeInteger(Number(match[3]))) return null;
    return { repo: `${match[1]}/${match[2]}`, pull_number: Number(match[3]) };
  } catch {
    return null;
  }
}
