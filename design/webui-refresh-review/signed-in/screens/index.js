// The signed-in pages the review page offers, in the order the sidebar lists
// them. A screen is a module with pack 1's shape; see public/screens/index.js.
//
// The concept control offers Current (a capture of the page today) and the
// portal style, because the owner chose that look over three to choose between.
import overview from './overview.js';

export const CONCEPTS = ['current', 'a'];
export const LABELS = { concept: { a: 'Portal style' } };

export const SCREENS = [overview];
