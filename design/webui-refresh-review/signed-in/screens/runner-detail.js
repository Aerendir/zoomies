// A runner: what it is doing now, where it is, and what went wrong if it failed.
import { FLEET } from '/signed-in/fixture.js';
import { callout, button, esc } from '/review/ui.js';
import { card, clip, facts, relative, steps } from '/signed-in/kit.js';
import { detailScreen } from './detail.js';
import { jobPill, runnerPill } from './list.js';

function view(id) {
  return () => {
    const runner = FLEET.runners.find((r) => r.id === id);
    const job = FLEET.jobs.find((j) => j.id === runner.current_job_id);
    return {
      title: runner.name,
      eyebrow: 'Runner',
      subtitle: `In ${runner.pool_name}, on ${runner.host_name}.`,
      actions: runnerPill(runner.state) + button({ label: 'Drain', variant: 'secondary', href: '#' }),
      body:
        (runner.message
          ? callout({ tone: 'danger', icon: 'triangle-alert', html: `<strong>It failed.</strong> ${esc(runner.message)}` })
          : '') +
        `<div class="k-grid wide">` +
        card({
          icon: 'workflow',
          title: 'Current job',
          html: job
            ? `<div class="k-stack">${clip(job.repo)}${clip(`${job.workflow} / ${job.job_name}`)}</div>${jobPill(job)}` +
              facts([['Branch', `<code>${esc(job.head_branch)}</code>`], ['Started', esc(relative(job.started_at))]])
            : `<p class="k-empty-text">No job. It ${runner.state === 'failed' ? 'never registered' : 'is waiting for one'}.</p>`,
        }) +
        card({
          icon: 'server',
          title: 'Details',
          html: facts([
            ['Pool', esc(runner.pool_name)],
            ['Host', esc(runner.host_name)],
            ['Jobs handled', esc(runner.jobs_handled)],
            ['Version', esc(runner.runner_version)],
            ['Created', esc(relative(runner.created_at))],
            ['Labels', clip(runner.labels.join(', '))],
          ]),
        }) +
        `</div>` +
        card({
          icon: 'compass',
          title: 'Its life',
          html: steps([
            { title: 'Started', text: 'A container was created on the host for this one job.' },
            { title: runner.state === 'failed' ? 'Failed before registering' : 'Registered with GitHub', text: runner.state === 'failed' ? 'It exited before GitHub saw it, so no job reached it.' : 'GitHub can now hand it a job.' },
            { title: 'Destroyed when the job ends', text: 'Nothing it did is visible to the next job.' },
          ]),
        }),
    };
  };
}

export default detailScreen({
  id: 'runner-detail',
  page: 'runners',
  title: 'Runner detail',
  noun: 'runner',
  back: { label: 'Runners', screen: 'runners' },
  currentAlt: 'Today’s runner page: a lifecycle strip, a job panel and a long list of fields.',
  variants: {
    busy: { label: 'Busy with a job', view: view('run_04') },
    failed: { label: 'Failed to start', view: view('run_09') },
  },
});
