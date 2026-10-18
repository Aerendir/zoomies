/*
  Addresses on a repair page. Every one of them came from somewhere that is not
  this build: a job's address from GitHub, a pull request's from whoever opened
  it. So a link is only ever an http or https address, and the repository, run
  and commit addresses are read off the host the job's address names rather than
  written as github.com, which would send a GitHub Enterprise Server user to the
  wrong site.
*/

const JOB_PATH = /^\/([^/]+)\/([^/]+)\/actions\/runs\/(\d+)\/job\/(\d+)\/?$/;
const COMMIT = /^[0-9a-f]{7,64}$/i;

const trimSlash = (value: string): string => value.replace(/\/+$/, '');

/**
 * The address if it is a web link, otherwise nothing. A scheme that runs script
 * or names a local file, an address with credentials in it, and anything that is
 * not an absolute address at all are all refused.
 */
export function safeHttpUrl(value: string | null | undefined): string | undefined {
  if (!value) return undefined;
  let url: URL;
  try {
    url = new URL(value.trim());
  } catch {
    return undefined;
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return undefined;
  if (!url.hostname || url.username || url.password) return undefined;
  return url.href;
}

/**
 * The job, its run and its repository, from the job's own address. An address
 * that is a web link but not a job page gives the job alone: it is still a place
 * to send somebody, and nothing more can be said about it.
 */
export function linksFromJobUrl(jobUrl: string | null | undefined): {
  job?: string;
  run?: string;
  repo?: string;
} {
  const job = safeHttpUrl(jobUrl);
  if (!job) return {};
  const url = new URL(job);
  const match = JOB_PATH.exec(url.pathname);
  if (!match) return { job };
  const repo = `${url.origin}/${match[1]}/${match[2]}`;
  return { job, run: `${repo}/actions/runs/${match[3]}`, repo };
}

/** A commit's page, built on the repository's address. Nothing without both. */
export function commitUrl(repoUrl: string | undefined, sha: string): string | undefined {
  const repo = safeHttpUrl(repoUrl);
  if (!repo || !COMMIT.test(sha)) return undefined;
  return `${trimSlash(repo)}/commit/${sha}`;
}

/** A pull request's page, built on the repository's address. Nothing without it. */
export function pullUrl(repoUrl: string | undefined, number: number): string | undefined {
  const repo = safeHttpUrl(repoUrl);
  if (!repo || !Number.isSafeInteger(number) || number < 1) return undefined;
  return `${trimSlash(repo)}/pull/${number}`;
}
