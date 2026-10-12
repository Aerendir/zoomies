/**
 * When a tooltip shows, and when it goes: one set of rules for every bubble in
 * the product.
 *
 * Each `Tooltip` used to keep its own flags and its own listeners, which is how
 * two of them end up open at once and how a sweep down the sidebar costs a
 * wait per entry. Here there is one controller, so "only one bubble is open"
 * is a property of the code rather than a hope, and the timings are written
 * once.
 *
 * The controller is plain TypeScript with an injectable clock: the numbers are
 * the behaviour, and a test that has to sleep for them is a test nobody runs.
 */
import { layers } from './keys';

/** How long the pointer rests on a trigger before the first bubble shows. */
export const OPEN_DELAY = 250;
/**
 * After a bubble has shown, and while one is open, moving to a neighbour shows
 * its bubble at once for this long. It is what makes a sweep down the sidebar
 * feel instant after the first entry.
 */
export const WARM = 300;
/** How long a bubble outlives the pointer leaving, so the pointer can cross onto it. */
export const GRACE = 100;

/** `none` is the opt-out for things people scrub across, like the matrix squares. */
export type Wait = 'default' | 'none';

/**
 * The wait a rich card takes. They opened the instant the pointer touched them
 * before the shared shell, which made a card flash over anything the pointer
 * crossed on its way elsewhere. Set this to `'none'` to give them that back.
 */
export const RICH_CARD_WAIT: Wait = 'default';

/** What the controller drives. A `Tooltip` instance is one of these. */
export interface Bubble {
  show(): void;
  hide(): void;
}

export interface Clock {
  now(): number;
  setTimeout(fn: () => void, ms: number): unknown;
  clearTimeout(id: unknown): void;
}

export const browserClock: Clock = {
  now: () => Date.now(),
  setTimeout: (fn, ms) => setTimeout(fn, ms),
  clearTimeout: (id) => clearTimeout(id as ReturnType<typeof setTimeout>),
};

interface Holds {
  /** The pointer is on the trigger. */
  hover: boolean;
  /** The pointer is on the bubble. */
  inside: boolean;
  /** Keyboard focus is on the trigger. */
  focus: boolean;
  /**
   * Its owner opened it (a tap, through the bound `open`). It stays until focus
   * leaves or a press puts it away, because a finger lifting is a pointer
   * leaving and must not take it with it.
   */
  pinned: boolean;
  opening: unknown;
  closing: unknown;
}

export interface TooltipController {
  /** The bubble showing now, if any. */
  readonly current: Bubble | null;
  hover(bubble: Bubble, wait?: Wait): void;
  unhover(bubble: Bubble): void;
  bubbleEnter(bubble: Bubble): void;
  bubbleLeave(bubble: Bubble): void;
  focus(bubble: Bubble): void;
  blur(bubble: Bubble): void;
  /** Put one bubble away now, and forget any wait it was in. */
  dismiss(bubble: Bubble): void;
  /** Put whatever is showing away. Returns whether there was anything to put away. */
  dismissCurrent(): boolean;
  /** A bubble its owner opened by itself: it takes the place of whatever was showing. */
  adopt(bubble: Bubble): void;
  /** A bubble its owner closed by itself. */
  forget(bubble: Bubble): void;
  /** A bubble that is going away for good. */
  release(bubble: Bubble): void;
}

export interface ControllerHooks {
  /** Called when the first bubble opens and again when the last one has closed. */
  onActive?: (active: boolean) => void;
}

export function createTooltipController(
  clock: Clock = browserClock,
  hooks: ControllerHooks = {},
): TooltipController {
  let current: Bubble | null = null;
  /** Until when a neighbour of a bubble that has just closed still opens at once. */
  let warmUntil = -Infinity;
  const holds = new Map<Bubble, Holds>();

  function of(bubble: Bubble): Holds {
    let h = holds.get(bubble);
    if (!h) {
      h = {
        hover: false,
        inside: false,
        focus: false,
        pinned: false,
        opening: null,
        closing: null,
      };
      holds.set(bubble, h);
    }
    return h;
  }

  function cancelOpening(bubble: Bubble): void {
    const h = of(bubble);
    if (h.opening !== null) clock.clearTimeout(h.opening);
    h.opening = null;
  }

  function cancelClosing(bubble: Bubble): void {
    const h = of(bubble);
    if (h.closing !== null) clock.clearTimeout(h.closing);
    h.closing = null;
  }

  function isWarm(): boolean {
    return current !== null || clock.now() < warmUntil;
  }

  function setCurrent(next: Bubble | null): void {
    const was = current !== null;
    current = next;
    if (was !== (next !== null)) hooks.onActive?.(next !== null);
  }

  /**
   * Whichever bubble takes over, the other waits are for something the person
   * has moved on from: a pointer that rested on A and a Tab that has since
   * reached B. A's timer must not come round later and put A's help over B.
   */
  function cancelOtherOpenings(except: Bubble): void {
    for (const other of holds.keys()) {
      if (other !== except) cancelOpening(other);
    }
  }

  function open(bubble: Bubble): void {
    cancelOtherOpenings(bubble);
    cancelOpening(bubble);
    cancelClosing(bubble);
    if (current === bubble) return;
    const other = current;
    if (other) {
      cancelClosing(other);
      cancelOpening(other);
      // Hidden before the new one is shown, so there is never a frame with two.
      other.hide();
    }
    setCurrent(bubble);
    bubble.show();
  }

  function close(bubble: Bubble): void {
    cancelOpening(bubble);
    cancelClosing(bubble);
    if (current !== bubble) return;
    warmUntil = clock.now() + WARM;
    of(bubble).pinned = false;
    setCurrent(null);
    bubble.hide();
  }

  function held(h: Holds): boolean {
    return h.hover || h.inside || h.focus || h.pinned;
  }

  /** Close after the grace, unless something is still holding it open. */
  function settle(bubble: Bubble): void {
    const h = of(bubble);
    if (current !== bubble || held(h) || h.closing !== null) return;
    h.closing = clock.setTimeout(() => {
      h.closing = null;
      if (!held(h)) close(bubble);
    }, GRACE);
  }

  return {
    get current() {
      return current;
    },
    hover(bubble, wait = 'default') {
      const h = of(bubble);
      h.hover = true;
      cancelClosing(bubble);
      if (current === bubble) return;
      if (wait === 'none' || isWarm()) {
        open(bubble);
        return;
      }
      cancelOpening(bubble);
      h.opening = clock.setTimeout(() => {
        h.opening = null;
        if (h.hover) open(bubble);
      }, OPEN_DELAY);
    },
    unhover(bubble) {
      of(bubble).hover = false;
      cancelOpening(bubble);
      settle(bubble);
    },
    bubbleEnter(bubble) {
      of(bubble).inside = true;
      cancelClosing(bubble);
    },
    bubbleLeave(bubble) {
      of(bubble).inside = false;
      settle(bubble);
    },
    focus(bubble) {
      of(bubble).focus = true;
      open(bubble);
    },
    blur(bubble) {
      const h = of(bubble);
      h.focus = false;
      h.pinned = false;
      if (h.hover || h.inside) return;
      close(bubble);
    },
    dismiss(bubble) {
      close(bubble);
    },
    dismissCurrent() {
      if (!current) return false;
      close(current);
      return true;
    },
    adopt(bubble) {
      cancelOtherOpenings(bubble);
      cancelOpening(bubble);
      cancelClosing(bubble);
      of(bubble).pinned = true;
      if (current === bubble) return;
      const other = current;
      if (other) {
        cancelClosing(other);
        cancelOpening(other);
        other.hide();
      }
      setCurrent(bubble);
    },
    forget(bubble) {
      cancelOpening(bubble);
      cancelClosing(bubble);
      of(bubble).pinned = false;
      if (current !== bubble) return;
      warmUntil = clock.now() + WARM;
      setCurrent(null);
    },
    release(bubble) {
      cancelOpening(bubble);
      cancelClosing(bubble);
      holds.delete(bubble);
      if (current === bubble) {
        warmUntil = clock.now() + WARM;
        setCurrent(null);
      }
    },
  };
}

/* -- the keyboard, the pointer and the layer stack ----------------------------- */

/**
 * The controller, joined to the parts of the page it has to answer to while a
 * bubble is up.
 *
 * Escape is heard in the capture phase, ahead of the menu or the dialog the
 * bubble is sitting over, and is stopped only when it closed a bubble: a menu
 * item's reason closes first and the next press closes the menu. The listener
 * exists only while a bubble does.
 *
 * The bubble is also on the layer stack, so `layers.closeTop()` agrees with
 * the listener about what Escape closes first. `layers.size` does not count it
 * (see keys.ts), so it never switches a global shortcut off.
 */
export function createAppTooltips(
  clock: Clock = browserClock,
  target: EventTarget | null = typeof document === 'undefined' ? null : document,
): { controller: TooltipController } {
  let layer: number | null = null;
  let detach: (() => void) | null = null;

  const controller: TooltipController = createTooltipController(clock, {
    onActive(active) {
      if (active) {
        layer = layers.push('tooltip', () => void controller.dismissCurrent());
        if (!target) return;
        const onKey = (event: Event) => {
          if ((event as KeyboardEvent).key !== 'Escape') return;
          if (!controller.dismissCurrent()) return;
          event.stopPropagation();
          event.preventDefault();
        };
        // A press anywhere puts the bubble away: on its trigger, on the bubble
        // itself, or somewhere else entirely.
        const onPointer = () => void controller.dismissCurrent();
        target.addEventListener('keydown', onKey, true);
        target.addEventListener('pointerdown', onPointer, true);
        detach = () => {
          target.removeEventListener('keydown', onKey, true);
          target.removeEventListener('pointerdown', onPointer, true);
        };
      } else {
        if (layer !== null) layers.remove(layer);
        layer = null;
        detach?.();
        detach = null;
      }
    },
  });
  return { controller };
}

/** The one controller every `Tooltip` in the page answers to. */
export const tooltips: TooltipController = createAppTooltips().controller;
