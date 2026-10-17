import { dogPhase } from '../mascot/dog-motion';

/** The gestures StandardStatusIcon draws. Every state keeps at least one
 *  living motion, so a stalled runner never looks like a broken image; the
 *  terminal three move slowly and sadly rather than freezing. */
export type DogMotion =
  'trot' | 'dash' | 'sit' | 'sniff' | 'wave' | 'strain' | 'settle' | 'low' | 'ghost' | 'scan';

/** The glyph in the icon's corner, which stays when motion is off. */
export type DogCue =
  | 'work'
  | 'boost'
  | 'maximum'
  | 'ready'
  | 'search'
  | 'hello'
  | 'pause'
  | 'rest'
  | 'error'
  | 'gone'
  | 'question';

const STATES = [
  'busy',
  'zoomies',
  'maximum_zoomies',
  'idle',
  'provisioning',
  'registering',
  'throttled',
  'draining',
  'failed',
  'removed',
  'unknown',
] as const;
export type StandardMotionState = (typeof STATES)[number];

const MOTIONS: Record<StandardMotionState, { motion: DogMotion; cue: DogCue; duration: number }> = {
  busy: { motion: 'trot', cue: 'work', duration: 1.3 },
  zoomies: { motion: 'dash', cue: 'boost', duration: 0.8 },
  maximum_zoomies: { motion: 'dash', cue: 'maximum', duration: 0.5 },
  idle: { motion: 'sit', cue: 'ready', duration: 6 },
  provisioning: { motion: 'sniff', cue: 'search', duration: 3.6 },
  registering: { motion: 'wave', cue: 'hello', duration: 3.2 },
  throttled: { motion: 'strain', cue: 'pause', duration: 2.2 },
  draining: { motion: 'settle', cue: 'rest', duration: 5 },
  failed: { motion: 'low', cue: 'error', duration: 6.5 },
  removed: { motion: 'ghost', cue: 'gone', duration: 5 },
  unknown: { motion: 'scan', cue: 'question', duration: 5 },
};

/** Status cues survive reduced motion; failures move slowly, never happily. */
export function standardMotion(state: string, seed: string) {
  const known = STATES.includes(state as StandardMotionState)
    ? (state as StandardMotionState)
    : 'unknown';
  const animation = MOTIONS[known];
  const phase = dogPhase(seed);
  const duration = animation.duration + phase * 0.15;
  return {
    state: known,
    ...animation,
    duration,
    // A negative delay starts each runner partway through its lap, so a page
    // of busy dogs never bobs in unison.
    phase: -phase * duration,
  };
}
