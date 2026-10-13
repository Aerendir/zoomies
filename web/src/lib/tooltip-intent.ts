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
  const doc = el.ownerDocument;
  const moved = (event: PointerEvent): void => {
    if (!event.composedPath().includes(el)) return;
    doc.removeEventListener('pointermove', moved, true);
    arm();
  };
  doc.addEventListener('pointermove', moved, true);
  return () => doc.removeEventListener('pointermove', moved, true);
}
