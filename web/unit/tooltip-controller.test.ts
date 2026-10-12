import { test } from 'node:test';
import assert from 'node:assert/strict';
import { focusWasPlaced, layers, placeFocus } from '../src/lib/keys';
import {
  GRACE,
  OPEN_DELAY,
  WARM,
  createAppTooltips,
  createTooltipController,
  type Bubble,
  type Clock,
} from '../src/lib/tooltip';

/**
 * A clock the test winds by hand. Real timers would make every one of these
 * a sleep, and the numbers under test are the product: 250, 300 and 100.
 */
function fakeClock(): Clock & { advance(ms: number): void } {
  let now = 1_000;
  let nextId = 1;
  const timers = new Map<number, { at: number; fn: () => void }>();
  return {
    now: () => now,
    setTimeout(fn, ms) {
      const id = nextId++;
      timers.set(id, { at: now + ms, fn });
      return id;
    },
    clearTimeout(id) {
      timers.delete(id as number);
    },
    advance(ms) {
      const end = now + ms;
      for (;;) {
        const due = [...timers.entries()]
          .filter(([, t]) => t.at <= end)
          .sort((a, b) => a[1].at - b[1].at)[0];
        if (!due) break;
        timers.delete(due[0]);
        now = due[1].at;
        due[1].fn();
      }
      now = end;
    },
  };
}

/** A bubble that remembers whether it is showing, and how often it was told to. */
function bubble(name: string): Bubble & { name: string; open: boolean; shown: number } {
  const b = {
    name,
    open: false,
    shown: 0,
    show() {
      b.open = true;
      b.shown++;
    },
    hide() {
      b.open = false;
    },
  };
  return b;
}

function setup() {
  const clock = fakeClock();
  const ctl = createTooltipController(clock);
  return { clock, ctl, a: bubble('a'), b: bubble('b'), c: bubble('c') };
}

test('the first bubble waits out the open delay before it shows', () => {
  const { clock, ctl, a } = setup();
  ctl.hover(a);
  clock.advance(OPEN_DELAY - 1);
  assert.equal(a.open, false, 'nothing yet one millisecond early');
  clock.advance(1);
  assert.equal(a.open, true);
  assert.equal(ctl.current, a);
});

test('leaving before the wait is over cancels the bubble altogether', () => {
  // A pointer crossing the page on its way somewhere else must not leave a
  // trail of help behind it.
  const { clock, ctl, a } = setup();
  ctl.hover(a);
  clock.advance(OPEN_DELAY - 50);
  ctl.unhover(a);
  clock.advance(1_000);
  assert.equal(a.open, false);
  assert.equal(a.shown, 0);
});

test('moving to a neighbour while one is open shows it at once', () => {
  // This is the sidebar sweep: the first entry costs a wait, the next seven
  // cost nothing.
  const { clock, ctl, a, b } = setup();
  ctl.hover(a);
  clock.advance(OPEN_DELAY);
  ctl.unhover(a);
  clock.advance(10);
  ctl.hover(b);
  assert.equal(b.open, true, 'no wait for the neighbour');
  assert.equal(a.open, false, 'and the first one is gone with it');
});

test('a neighbour is still instant for the warm window after the last one closed', () => {
  const { clock, ctl, a, b, c } = setup();
  ctl.hover(a);
  clock.advance(OPEN_DELAY);
  ctl.unhover(a);
  clock.advance(GRACE); // closes here
  assert.equal(a.open, false);
  clock.advance(WARM - 1);
  ctl.hover(b);
  assert.equal(b.open, true, 'one millisecond inside the window');
  ctl.unhover(b);
  clock.advance(GRACE + WARM + 1);
  assert.equal(b.open, false);
  ctl.hover(c);
  clock.advance(OPEN_DELAY - 1);
  assert.equal(c.open, false, 'a pointer that rested elsewhere pays the wait again');
  clock.advance(1);
  assert.equal(c.open, true);
});

test('closing waits out the grace so the pointer can cross onto the bubble', () => {
  const { clock, ctl, a } = setup();
  ctl.hover(a);
  clock.advance(OPEN_DELAY);
  ctl.unhover(a);
  clock.advance(GRACE - 1);
  assert.equal(a.open, true, 'still there a millisecond before the grace ends');
  clock.advance(1);
  assert.equal(a.open, false);
});

test('coming back to the trigger inside the grace keeps it open', () => {
  const { clock, ctl, a } = setup();
  ctl.hover(a);
  clock.advance(OPEN_DELAY);
  ctl.unhover(a);
  clock.advance(GRACE - 20);
  ctl.hover(a);
  clock.advance(1_000);
  assert.equal(a.open, true);
  assert.equal(a.shown, 1, 'and it was not shown a second time');
});

test('entering the bubble cancels the close, and leaving it starts the grace again', () => {
  const { clock, ctl, a } = setup();
  ctl.hover(a);
  clock.advance(OPEN_DELAY);
  ctl.unhover(a);
  clock.advance(GRACE - 10);
  ctl.bubbleEnter(a);
  clock.advance(5_000);
  assert.equal(a.open, true, 'a pointer resting on the bubble keeps it');
  ctl.bubbleLeave(a);
  clock.advance(GRACE - 1);
  assert.equal(a.open, true);
  clock.advance(1);
  assert.equal(a.open, false);
});

test('keyboard focus shows the bubble immediately and blur takes it away', () => {
  const { clock, ctl, a } = setup();
  ctl.focus(a);
  assert.equal(a.open, true, 'no timer ran');
  ctl.blur(a);
  assert.equal(a.open, false, 'and blur is as immediate');
  clock.advance(1_000);
  assert.equal(a.open, false);
});

test('a focused trigger keeps its bubble when the pointer drifts off it', () => {
  const { clock, ctl, a } = setup();
  ctl.focus(a);
  ctl.hover(a);
  ctl.unhover(a);
  clock.advance(1_000);
  assert.equal(a.open, true, 'the keyboard is still there');
  ctl.blur(a);
  assert.equal(a.open, false);
});

test('blur leaves the bubble up while the pointer is still on the trigger', () => {
  const { clock, ctl, a } = setup();
  ctl.hover(a);
  clock.advance(OPEN_DELAY);
  ctl.focus(a);
  ctl.blur(a);
  assert.equal(a.open, true);
  ctl.unhover(a);
  clock.advance(GRACE);
  assert.equal(a.open, false);
});

test('an opt-out opens at once even from cold, which is what the matrix squares do', () => {
  const { ctl, a } = setup();
  ctl.hover(a, 'none');
  assert.equal(a.open, true);
});

test('a pointer press hides the bubble and cancels one that has not shown yet', () => {
  const { clock, ctl, a, b } = setup();
  ctl.hover(a);
  clock.advance(OPEN_DELAY);
  assert.equal(ctl.dismissCurrent(), true);
  assert.equal(a.open, false);
  // A click made inside the wait must not be answered by help arriving late.
  ctl.hover(b);
  clock.advance(OPEN_DELAY - 50);
  ctl.dismiss(b);
  clock.advance(1_000);
  assert.equal(b.open, false);
});

test('escape closes the open bubble and says so, and says nothing when there is none', () => {
  // The caller stops the event only when this returns true, so an Escape that
  // belongs to a menu or a dialog is never swallowed by a tooltip that is not
  // there.
  const { clock, ctl, a } = setup();
  assert.equal(ctl.dismissCurrent(), false);
  ctl.hover(a);
  clock.advance(OPEN_DELAY);
  assert.equal(ctl.dismissCurrent(), true);
  assert.equal(a.open, false);
  assert.equal(ctl.dismissCurrent(), false, 'the second press is the dialog');
});

test('only one bubble is ever showing, whatever the pointer and the keyboard do', () => {
  const { clock, ctl, a, b, c } = setup();
  const all = [a, b, c];
  // A fixed pseudo-random walk, so a failure can be replayed.
  let seed = 7;
  const next = (n: number) => {
    seed = (seed * 1103515245 + 12345) & 0x7fffffff;
    return seed % n;
  };
  for (let step = 0; step < 2_000; step++) {
    const target = all[next(3)]!;
    switch (next(9)) {
      case 0:
        ctl.hover(target);
        break;
      case 1:
        ctl.hover(target, 'none');
        break;
      case 2:
        ctl.unhover(target);
        break;
      case 3:
        ctl.focus(target);
        break;
      case 4:
        ctl.blur(target);
        break;
      case 5:
        ctl.bubbleEnter(target);
        break;
      case 6:
        ctl.bubbleLeave(target);
        break;
      case 7:
        ctl.dismissCurrent();
        break;
      default:
        clock.advance(next(400));
    }
    const open = all.filter((x) => x.open);
    assert.ok(open.length <= 1, `step ${step}: ${open.map((x) => x.name)} are all open`);
    assert.equal(ctl.current, open[0] ?? null, `step ${step}: the controller agrees`);
  }
});

test('a bubble opened by its owner is adopted, and closes whatever was showing', () => {
  // The rich cards on a touch screen open from a tap through their bound
  // `open`, bypassing the pointer rules. They still have to be the only one.
  const { ctl, a, b } = setup();
  ctl.focus(a);
  b.show();
  ctl.adopt(b);
  assert.equal(a.open, false);
  assert.equal(ctl.current, b);
  b.hide();
  ctl.forget(b);
  assert.equal(ctl.current, null);
});

test('a bubble its owner opened stays through a pointer leaving, until focus goes or something is pressed', () => {
  // A tap on a touch screen opens a rich card through its bound `open`, and a
  // finger lifting is a pointer leaving. The card must not go with it.
  const { clock, ctl, a } = setup();
  a.show();
  ctl.adopt(a);
  ctl.hover(a);
  ctl.unhover(a);
  clock.advance(5_000);
  assert.equal(a.open, true, 'a finger lifting is not a reason to close');
  ctl.blur(a);
  assert.equal(a.open, false, 'focus leaving is');
});

test('a pinned bubble can still be put away by a press', () => {
  const { ctl, a } = setup();
  a.show();
  ctl.adopt(a);
  assert.equal(ctl.dismissCurrent(), true);
  assert.equal(a.open, false);
});

/* -- the keyboard and the layer stack ------------------------------------------ */

/** An Escape the way the browser makes one, minus the DOM Node does not have. */
function escape(): Event & { key: string; stopped: boolean } {
  const event = new Event('keydown', { cancelable: true }) as Event & {
    key: string;
    stopped: boolean;
  };
  event.key = 'Escape';
  event.stopped = false;
  event.stopPropagation = () => {
    event.stopped = true;
  };
  return event;
}

test('escape stops where it is heard only while a bubble is open', () => {
  const clock = fakeClock();
  const target = new EventTarget();
  const { controller } = createAppTooltips(clock, target);
  const a = bubble('a');

  const idle = escape();
  target.dispatchEvent(idle);
  assert.equal(idle.stopped, false, 'with nothing open the key goes on to whoever wants it');

  controller.focus(a);
  const open = escape();
  target.dispatchEvent(open);
  assert.equal(open.stopped, true);
  assert.equal(open.defaultPrevented, true);
  assert.equal(a.open, false);

  const after = escape();
  target.dispatchEvent(after);
  assert.equal(after.stopped, false, 'and the next press is not the tooltip’s');
});

test('a pointer press anywhere puts the bubble away', () => {
  const clock = fakeClock();
  const target = new EventTarget();
  const { controller } = createAppTooltips(clock, target);
  const a = bubble('a');
  controller.focus(a);
  target.dispatchEvent(new Event('pointerdown'));
  assert.equal(a.open, false);
});

test('an open bubble is the top of the layer stack but never takes the keyboard', () => {
  // Shortcuts stand down under any overlay. A tooltip is not one: with this
  // wrong, resting the pointer on the sidebar would switch `g p` and `r` off.
  const clock = fakeClock();
  const { controller } = createAppTooltips(clock, new EventTarget());
  const a = bubble('a');
  const before = layers.size;
  controller.focus(a);
  assert.equal(layers.top()?.kind, 'tooltip');
  assert.equal(layers.size, before, 'it does not count');
  assert.equal(layers.closeTop(), true, 'but the stack can still close it');
  assert.equal(a.open, false);
  assert.equal(layers.top()?.kind, undefined);
});

test('a tooltip over a menu closes before the menu does', () => {
  const clock = fakeClock();
  const { controller } = createAppTooltips(clock, new EventTarget());
  const a = bubble('a');
  let menuOpen = true;
  const menu = layers.push('dropdown', () => (menuOpen = false));
  try {
    controller.focus(a);
    assert.equal(layers.size, 1, 'only the menu counts');
    layers.closeTop();
    assert.equal(a.open, false, 'the bubble goes first');
    assert.equal(menuOpen, true, 'the menu is untouched');
    layers.closeTop();
    assert.equal(menuOpen, false, 'and the next Escape is the menu’s');
  } finally {
    layers.remove(menu);
  }
});

test('focus an overlay places itself is not somebody reaching for a control', () => {
  // A dialog opened from the keyboard puts focus on its first control. A bubble
  // that opened on that would take the first Escape, which belongs to the dialog.
  assert.equal(focusWasPlaced(), false, 'nothing has been placed yet');
  let focused = false;
  placeFocus({ focus: () => (focused = true) } as unknown as HTMLElement);
  assert.equal(focused, true, 'the element is still focused');
  assert.equal(focusWasPlaced(), true, 'and the tooltip is told it was placed');
});
