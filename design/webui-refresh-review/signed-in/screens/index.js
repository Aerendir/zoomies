// The signed-in pages the review page offers, in the order the sidebar lists
// them. A screen is a module with pack 1's shape; see public/screens/index.js.
//
// The concept control offers Current (a capture of the page today) and the
// portal style, because the owner chose that look over three to choose between.
import overview from './overview.js';
import pools from './pools.js';
import poolDetail from './pool-detail.js';
import runners from './runners.js';
import runnerDetail from './runner-detail.js';
import queue from './queue.js';
import workflows from './workflows.js';
import jobs from './jobs.js';
import hosts from './hosts.js';
import hostDetail from './host-detail.js';
import providers from './providers.js';
import kennel from './kennel.js';
import repairs from './repairs.js';
import aiContext from './ai-context.js';
import usage from './usage.js';
import comparisons from './comparisons.js';

export const CONCEPTS = ['current', 'a'];
export const LABELS = { concept: { a: 'Portal style' } };

export const SCREENS = [
  overview,
  pools,
  poolDetail,
  runners,
  runnerDetail,
  queue,
  workflows,
  jobs,
  hosts,
  hostDetail,
  providers,
  usage,
  comparisons,
  kennel,
  repairs,
  aiContext,
];
