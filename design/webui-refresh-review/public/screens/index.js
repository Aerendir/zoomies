// The screens the review page offers, in the order they appear. A screen is a
// module with this shape:
//
//   id        the hash and file-name key, such as 'signin'
//   title     what the Screen control calls it
//   states    state ids, the first being the default
//   labels    optional: a human label for a state id (otherwise it is spelt out)
//   currentAlt  optional: what the capture of today's page shows, for a screen reader
//   render(concept, state)   markup for concept 'a', 'b' or 'c', as a string
//
// Adding a screen is one import and one line here.
import signin from './signin.js';
import firstAccount from './first-account.js';
import connectApp from './connect-app.js';
import fleetStatus from './fleet-status.js';
import notFound from './not-found.js';

export const SCREENS = [signin, firstAccount, connectApp, fleetStatus, notFound];
