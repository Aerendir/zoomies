import type { Schemas, User } from '../api/types';

/*
  What the link form needs to know about a link it cannot write: the server
  refused it because the GitHub account is already linked to another Zoomies
  user, and a person holding two accounts after an instance move deserves to
  see whose row is in the way and one press to move the link, not a driver
  error and a hunt through the accounts page.
*/

export interface HeldLink {
  /** The holder's Zoomies account id, as the refusal carried in detail. */
  holderId: string;
  /** The holder by name when the page's user list has their row, else null. */
  user: User | null;
}

/**
 * The held-link refusal, as the page's data for it, or null for any other
 * refusal: the toast keeps those. The refusal's detail names the holder, and
 * the link row the page already holds is the fallback for a list drawn before
 * the link landed where it now is. The typed login decides which row the
 * page agrees with, compared without case, because the form holds what was
 * typed and the row what GitHub verified.
 */
export function heldLink(
  error: { code?: string; detail?: string },
  login: string,
  identities: readonly (Schemas['EliIdentity'] | undefined)[] | undefined,
  users: readonly User[],
): HeldLink | null {
  if (error?.code !== 'assistant.identity_held') return null;
  const wanted = login.trim().toLowerCase();
  const row = (identities ?? []).find(
    (i) => i !== undefined && i.github_login.toLowerCase() === wanted,
  );
  const holderId = error.detail || row?.user_id || '';
  return { holderId, user: users.find((u) => u.id === holderId) ?? null };
}

/**
 * The sentence the page shows for a held link: whose it is, and what moving
 * it does to them. An unknown holder keeps the generic sentence rather than
 * an empty at-sign.
 */
export function heldLinkMessage(held: HeldLink): string {
  const name = held.user?.username || held.holderId;
  if (!name) {
    return 'The GitHub account is already linked to another Zoomies user. Move the link to the person chosen above; their confirmation resets, so they must allow provider use again.';
  }
  return `@${name} already holds this GitHub account's link. Move the link to the person chosen above; their confirmation resets, so they must allow provider use again.`;
}
