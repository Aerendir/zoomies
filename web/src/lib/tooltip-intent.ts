/**
 * Hover that means something: wait for the pointer to move over the trigger.
 *
 * When the page changes under a pointer that is resting (a sheet rising where
 * the finger-sized "More" button was, a list re-rendering) the browser fires
 * enter events for whatever is now beneath it, with no pointer move behind
 * them. A real arrival is a pointer move whose target is inside the trigger.
 * The listener is installed before that move, so it still counts when enter
 * arrives after it.
 *
 * Returns the function that stops listening.
 */
export function whenPointerMoves(el: Element, arm: (event: PointerEvent) => void): () => void {
  const moved: EventListener = (event) => arm(event as PointerEvent);
  el.addEventListener('pointermove', moved);
  return () => el.removeEventListener('pointermove', moved);
}
