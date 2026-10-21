// Migrate: rewrite a repository's workflows to run on this fleet.
//
// Three steps, in the portal's numerals, because that is what it is: pick, read
// the change, open the pull request. Nothing is changed until the last one, and
// the page says so where the button is.
import { button, field } from '/review/ui.js';
import { card, stage, steps } from '/signed-in/kit.js';
import { pageScreen } from './list.js';

export default pageScreen({
  page: 'migrate',
  title: 'Migrate',
  eyebrow: 'GitHub',
  subtitle: 'Move a repository’s workflows onto this fleet.',
  noun: 'migration',
  normalLabel: 'Pick a repository',
  currentAlt: 'Today’s Migrate page: a repository picker and a long explanation of what will change.',
  empty: {
    icon: 'git-pull-request-arrow',
    title: 'No repository to migrate',
    text: 'Connect a GitHub account first, so Zoomies has repositories to read.',
    action: 'Connect an account',
  },
  body: () =>
    stage({
      eyebrow: 'Migrate',
      headline: 'Change one line,',
      accent: 'not the workflow.',
      lede: 'Zoomies rewrites the runs-on line of each job and opens a pull request. You read it and merge it, or you do not.',
    }) +
    `<div class="k-grid wide">` +
    card({
      icon: 'compass',
      title: 'How it goes',
      html: steps([
        { title: 'Pick a repository', text: 'Zoomies reads its workflows and nothing else.' },
        { title: 'Read the change', text: 'Every runs-on line it would rewrite, side by side with the one it has now.' },
        { title: 'Open the pull request', text: 'Only now is anything written to GitHub.' },
      ], 0),
    }) +
    card({
      icon: 'search',
      title: 'Pick a repository',
      html:
        `<form class="k-form" novalidate>${field({ name: 'migrate-repo', label: 'Repository', value: 'acme-corp/api', size: 'md', hint: 'As owner/name.' })}` +
        `<div class="k-form-actions">${button({ label: 'Read its workflows', variant: 'primary', iconAfter: 'arrow-right', type: 'submit' })}</div></form>`,
    }) +
    `</div>`,
});
