import { dogPhase } from '../mascot/dog-motion';

/** Preserve the Cute avatar's existing cadence when Standard changes style. */
export function cuteMotion(state: string, seed: string) {
  const fraction = dogPhase(seed);
  const walking = state === 'busy';
  const stride =
    (state === 'maximum_zoomies' ? 0.56 : state === 'zoomies' ? 0.8 : walking ? 1.6 : 0.64) +
    fraction * (walking ? 0.12 : 0.02);
  return {
    walking,
    spinning: state === 'maximum_zoomies' || state === 'zoomies',
    stride,
    phase: -fraction * stride,
  };
}

/** The status tone each state borrows for its collar tag and props. */
type Tone = 'idle' | 'busy' | 'pending' | 'draining' | 'danger' | 'neutral';

export interface CutePose {
  pose: 'stand' | 'sit';
  ears: 'up' | 'droop' | 'flat';
  eyes: 'open' | 'happy' | 'worried' | 'half';
  mouth: 'smile' | 'frown' | 'o' | 'flat' | 'pant';
  brows: 'none' | 'determined' | 'sad' | 'worried';
  props: string;
  tone: Tone;
}

const POSES: Record<string, CutePose> = {
  idle: {
    pose: 'sit',
    ears: 'up',
    eyes: 'open',
    mouth: 'smile',
    brows: 'none',
    props: '',
    tone: 'idle',
  },
  busy: {
    pose: 'stand',
    ears: 'up',
    eyes: 'open',
    mouth: 'smile',
    brows: 'determined',
    props: '',
    tone: 'busy',
  },
  provisioning: {
    pose: 'stand',
    ears: 'up',
    eyes: 'open',
    mouth: 'o',
    brows: 'none',
    props: 'trail',
    tone: 'pending',
  },
  registering: {
    pose: 'sit',
    ears: 'up',
    eyes: 'open',
    mouth: 'o',
    brows: 'none',
    props: 'ring',
    tone: 'pending',
  },
  draining: {
    pose: 'sit',
    ears: 'droop',
    eyes: 'half',
    mouth: 'flat',
    brows: 'sad',
    props: 'leash',
    tone: 'draining',
  },
  failed: {
    pose: 'sit',
    ears: 'droop',
    eyes: 'worried',
    mouth: 'frown',
    brows: 'sad',
    props: '',
    tone: 'danger',
  },
  removed: {
    pose: 'sit',
    ears: 'droop',
    eyes: 'happy',
    mouth: 'smile',
    brows: 'none',
    props: '',
    tone: 'neutral',
  },
  unknown: {
    pose: 'sit',
    ears: 'flat',
    eyes: 'worried',
    mouth: 'o',
    brows: 'worried',
    props: '',
    tone: 'neutral',
  },
  throttled: {
    pose: 'stand',
    ears: 'flat',
    eyes: 'half',
    mouth: 'flat',
    brows: 'worried',
    props: 'leash',
    tone: 'pending',
  },
  zoomies: {
    pose: 'stand',
    ears: 'up',
    eyes: 'open',
    mouth: 'pant',
    brows: 'none',
    props: 'dust speed',
    tone: 'busy',
  },
  maximum_zoomies: {
    pose: 'stand',
    ears: 'up',
    eyes: 'open',
    mouth: 'pant',
    brows: 'none',
    props: 'dust speed sparkle',
    tone: 'busy',
  },
};

const FALLBACK: CutePose = {
  pose: 'sit',
  ears: 'flat',
  eyes: 'worried',
  mouth: 'o',
  brows: 'worried',
  props: '',
  tone: 'neutral',
};

/**
 * The pose each state holds: posture, ears, eyes, mouth, brows and props.
 * Posture and ears carry the state at grid size, so the four sitting states
 * separate by ear shape and the tag's status tone rather than by detail that
 * only survives at 96px.
 */
export function cutePose(state: string): CutePose {
  return POSES[state] ?? FALLBACK;
}
