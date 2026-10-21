/**
 * localStorage that never throws.
 *
 * Private-browsing modes throw on access rather than returning null, and a page
 * that will not load in a private window is a page someone cannot demo. This is
 * the whole of it, with no imports, so the status page can read the theme
 * without taking the preference store and the API client along.
 */
export const storage = {
  get(key: string): string | null {
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  },
  set(key: string, value: string): void {
    try {
      localStorage.setItem(key, value);
    } catch {
      // Nothing to do and nothing worth telling the operator: the session simply
      // does not persist.
    }
  },
  remove(key: string): void {
    try {
      localStorage.removeItem(key);
    } catch {
      /* as above */
    }
  },
};
