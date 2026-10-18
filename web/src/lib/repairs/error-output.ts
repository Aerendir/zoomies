/*
  A failed job's error lines, made fit to show. They are what a program printed
  to a terminal, so they arrive with the colour codes it wrote for one. Those are
  noise on a page, and any other control character is a way to confuse a reader
  or to hide text, so they go too. The text itself is never altered: it is drawn
  as text by the component, whatever markup it holds.
*/

const ESC = String.fromCharCode(27);

// A control sequence introducer: ESC [ parameters intermediates final.
const SEQUENCE = new RegExp(`${ESC}\\[[0-9;?]*[ -/]*[@-~]`, 'g');

// Everything below a space except the tab, and DEL.
// eslint-disable-next-line no-control-regex
const CONTROL = /[\u0000-\u0008\u000a-\u001f\u007f]/g;

/** The lines with escape sequences and control characters removed, and empty ones dropped. */
export function cleanLines(lines: readonly string[]): string[] {
  return lines
    .map((line) => line.replace(SEQUENCE, '').replace(CONTROL, ''))
    .filter((line) => line.trim() !== '');
}
