import type { Job, Schemas } from '../api/types';
import { faultDomain, faultLabel } from '../faults';
import { commitUrl, linksFromJobUrl, pullUrl, safeHttpUrl } from './links';

/*
  The case: what a repair is about, as the page draws it. There are two ways to
  know it. While a person is deciding, it is read from what Zoomies holds about
  the failing run and from what readiness found about the pull request. After a
  repair has started, it is read back from the snapshot the repair kept, because
  Zoomies prunes its job records and a pull request's title changes. Both give
  the same view, so the card that shows it does not care which it was handed.

  Everything here was written by someone who can open a pull request or edit a
  workflow. Text is passed through as text, for the page to draw as text, and an
  address is kept only if it is a plain web link.
*/

type Readiness = Schemas['EliReadiness'];
type Repair = Schemas['EliRepair'];

export interface CaseJob {
  /** Stable for the list: the Zoomies job id, or a stand-in when there is none. */
  key: string;
  name: string;
  step?: string;
  /** The failing step's number, which GitHub numbers from 1. */
  stepNumber?: number;
  url?: string;
  durationMs?: number;
  /** Whose the failure is: the workflow's, this fleet's, or not known. */
  domain: 'workflow' | 'fleet' | '';
  faultLabel?: string;
  fix?: string;
  /** The job the person opened to get here. */
  opened: boolean;
  githubJobId?: number;
}

export interface CaseView {
  repo: string;
  repoUrl?: string;
  title: string;
  /** Whether the pull request has been looked up, which is what decides if its title is known. */
  hasPull: boolean;
  pullNumber?: number;
  pullUrl?: string;
  author?: string;
  branch?: string;
  base?: string;
  sha?: string;
  commitUrl?: string;
  runLabel?: string;
  runUrl?: string;
  failedAt?: string;
  ran?: { pool?: string; runner?: string; poolId?: string; runnerId?: string };
  jobs: CaseJob[];
}

/** "CI #482 failed on fix/retry-502": what ran, when the pull request's own title is not known. */
function runTitle(workflow: string | undefined, runNumber: number | undefined, branch?: string) {
  if (!workflow && !runNumber) return '';
  const run = [workflow || 'Run', runNumber ? `#${runNumber}` : ''].filter(Boolean).join(' ');
  return branch ? `${run} failed on ${branch}` : `${run} failed`;
}

function runLabel(workflow: string | undefined, runNumber: number | undefined): string | undefined {
  if (workflow && runNumber) return `${workflow} #${runNumber}`;
  if (workflow) return workflow;
  return runNumber ? `Run #${runNumber}` : undefined;
}

/** The last words: what any repair has, for the case that knows nothing else. */
function plainTitle(repo: string, pullNumber: number | undefined): string {
  return pullNumber ? `Pull request #${pullNumber} in ${repo}` : `A repair in ${repo}`;
}

function liveJob(job: Job, index: number, openedJobId: string | undefined): CaseJob {
  const domain = faultDomain(job);
  const out: CaseJob = {
    key: job.id || (job.github_job_id ? `gh-${job.github_job_id}` : `job-${index}`),
    name: job.job_name ?? '',
    domain,
    opened: Boolean(openedJobId) && job.id === openedJobId,
  };
  if (job.failed_step?.name) {
    out.step = job.failed_step.name;
    if (job.failed_step.number) out.stepNumber = job.failed_step.number;
  }
  const url = safeHttpUrl(job.html_url);
  if (url) out.url = url;
  if (job.duration_ms) out.durationMs = job.duration_ms;
  const label = domain === 'fleet' ? faultLabel(job.fault_kind) : '';
  if (label) out.faultLabel = label;
  if (job.fault_fix) out.fix = job.fault_fix;
  if (job.github_job_id) out.githubJobId = job.github_job_id;
  return out;
}

/** The latest of the jobs' completion times, which is when the run's failure was settled. */
function lastCompleted(jobs: readonly Job[]): string | undefined {
  let best: string | undefined;
  let bestAt = -Infinity;
  for (const job of jobs) {
    const at = job.completed_at ? Date.parse(job.completed_at) : NaN;
    if (job.completed_at && at > bestAt) {
      best = job.completed_at;
      bestAt = at;
    }
  }
  return best;
}

export function caseFromLive(input: {
  repo: string;
  jobs: readonly Job[];
  readiness: Readiness | null;
  openedJobId?: string;
}): CaseView {
  const { repo, jobs, readiness, openedJobId } = input;
  const first = jobs[0];
  const links = linksFromJobUrl(first?.html_url);
  const pull = readiness?.pull;
  const pullNumber =
    pull?.number || (readiness && readiness.pull_number > 0 ? readiness.pull_number : undefined);
  const sha = readiness?.head_sha || first?.head_sha || undefined;
  const branch = pull?.branch || first?.head_branch || undefined;
  const ranOn = jobs.find((j) => j.pool_name || j.runner_name || j.pool_id || j.runner_id);

  const view: CaseView = {
    repo,
    title:
      pull?.title ||
      runTitle(first?.workflow, first?.run_number, branch) ||
      plainTitle(repo, pullNumber),
    hasPull: Boolean(pull),
    jobs: jobs.map((job, index) => liveJob(job, index, openedJobId)),
  };
  if (links.repo) view.repoUrl = links.repo;
  if (pullNumber) view.pullNumber = pullNumber;
  const pullLink = safeHttpUrl(pull?.url) ?? pullUrl(links.repo, pullNumber ?? 0);
  if (pullLink) view.pullUrl = pullLink;
  if (pull?.author) view.author = pull.author;
  if (branch) view.branch = branch;
  if (pull?.base) view.base = pull.base;
  if (sha) {
    view.sha = sha;
    const commit = commitUrl(links.repo, sha);
    if (commit) view.commitUrl = commit;
  }
  const label = runLabel(first?.workflow, first?.run_number);
  if (label) view.runLabel = label;
  if (links.run) view.runUrl = links.run;
  const failedAt = lastCompleted(jobs);
  if (failedAt) view.failedAt = failedAt;
  if (ranOn) {
    view.ran = {};
    if (ranOn.pool_name) view.ran.pool = ranOn.pool_name;
    if (ranOn.runner_name) view.ran.runner = ranOn.runner_name;
    if (ranOn.pool_id) view.ran.poolId = ranOn.pool_id;
    if (ranOn.runner_id) view.ran.runnerId = ranOn.runner_id;
  }
  return view;
}

const RUN_PAGE = /^\/([^/]+)\/([^/]+)\/actions\/runs\/\d+\/?$/;
const PULL_PAGE = /^\/([^/]+)\/([^/]+)\/pull\/\d+\/?$/;

/** The repository's address, read off a run's or a pull request's when no job gave it. */
function repoFromPage(page: string | undefined, pattern: RegExp): string | undefined {
  if (!page) return undefined;
  const url = new URL(page);
  const match = pattern.exec(url.pathname);
  return match ? `${url.origin}/${match[1]}/${match[2]}` : undefined;
}

export function caseFromSnapshot(repair: Repair): CaseView {
  const kept = repair.details?.case;
  const jobs = kept?.jobs ?? [];
  const runLink = safeHttpUrl(kept?.run_url);
  const pullLink = safeHttpUrl(kept?.pull_url);
  const repoUrl =
    linksFromJobUrl(jobs[0]?.url).repo ??
    repoFromPage(runLink, RUN_PAGE) ??
    repoFromPage(pullLink, PULL_PAGE);
  const pullNumber = repair.pull_number > 0 ? repair.pull_number : undefined;

  const view: CaseView = {
    repo: repair.repo,
    title:
      kept?.pull_title ||
      runTitle(kept?.workflow, kept?.run_number, kept?.branch) ||
      plainTitle(repair.repo, pullNumber),
    hasPull: Boolean(kept?.pull_title || kept?.pull_url),
    jobs: jobs.map((job, index): CaseJob => {
      const out: CaseJob = { key: `job-${index}`, name: job.name, domain: '', opened: false };
      if (job.step) out.step = job.step;
      const url = safeHttpUrl(job.url);
      if (url) out.url = url;
      if (job.duration_ms) out.durationMs = job.duration_ms;
      return out;
    }),
  };
  if (repoUrl) view.repoUrl = repoUrl;
  if (pullNumber) view.pullNumber = pullNumber;
  const pullAt = pullLink ?? pullUrl(repoUrl, pullNumber ?? 0);
  if (pullAt) view.pullUrl = pullAt;
  if (kept?.author) view.author = kept.author;
  if (kept?.branch) view.branch = kept.branch;
  if (repair.head_sha) {
    view.sha = repair.head_sha;
    const commit = commitUrl(repoUrl, repair.head_sha);
    if (commit) view.commitUrl = commit;
  }
  const label = runLabel(kept?.workflow, kept?.run_number);
  if (label) view.runLabel = label;
  if (runLink) view.runUrl = runLink;
  return view;
}
