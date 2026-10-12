/**
 * Hover that means something: wait for the pointer to move over the trigger.
 *
 * When the page changes under a pointer that is resting (a sheet rising where
 * the finger-sized "More" button was, a list re-rendering) the browser fires
 * enter events for whatever is now beneath it, with no pointer move behind
 * them. A real arrival is an enter followed at once by a move, so the wait
 * starts on the move. Without this a bubble the person never reached for
 * opened over the sheet they had just opened, and took the first Escape that
 * was meant for the sheet.
 *
 * Returns the function that gives the wait up (the pointer left or pressed).
 */
export function whenPointerMoves(el: Element, arm: () => void): () => void {
  const moved = (): void => {
    el.removeEventListener('pointermove', moved);
    arm();
  };
  el.addEventListener('pointermove', moved);
  return () => el.removeEventListener('pointermove', moved);
}
