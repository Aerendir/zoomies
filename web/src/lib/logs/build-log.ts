/** The stock image marks already-masked workflow output before relaying it. */
const PREFIX = 'ZOOMIES_BUILD_LOG ';
const VERSION = 'ZOOMIES_BUILD_LOG_V1';

/**
 * Where the viewer writes that the stream reopened.
 *
 * The relay sends its backlog again from the top on every reopen, so the lines
 * after this one may repeat lines above it. The raw tab shows the marker as it
 * is; the build log keeps only prefixed lines and would have dropped it, leaving
 * the repeat in the scrollback and in the saved file with nothing to explain it.
 * One definition, so what the viewer writes and what the build log recognises
 * cannot drift apart.
 */
const RECONNECT_MARKER = '-- reconnected to the log stream --';
const RECONNECT_LINE = `\x1b[2m${RECONNECT_MARKER}\x1b[0m`;
export const RECONNECT_NOTICE = `\r\n${RECONNECT_LINE}\r\n`;

export class BuildLog {
  #partial = '';
  #lines: string[] = [];
  #limit: number;
  supported = false;
  lineCount = 0;

  constructor(limit: number) {
    this.#limit = limit;
  }

  push(chunk: string): string {
    const lines = (this.#partial + chunk).split('\n');
    this.#partial = lines.pop() ?? '';
    const output: string[] = [];
    for (const raw of lines) {
      const line = raw.replace(/\r$/, '');
      if (line === VERSION) this.supported = true;
      if (line === RECONNECT_LINE) {
        // A seam separates something from something. Before any build output
        // there is nothing above it, and a lone seam would hide the waiting
        // message and switch on Save log for a file of one line about the
        // connection.
        if (this.#lines.length > 0 || output.length > 0)
          output.push(`##[debug]${RECONNECT_MARKER}`);
        continue;
      }
      if (/^\[log stream fell behind, \d+ bytes dropped\]$/.test(line)) {
        output.push(`##[warning]${line}. Open GitHub for the complete build log.`);
      }
      if (line.startsWith("zoomies: could not open this runner's log stream: ")) {
        output.push(`##[error]${line}`);
      }
      if (!line.startsWith(PREFIX)) continue;
      this.supported = true;
      output.push(line.slice(PREFIX.length));
    }
    this.lineCount += output.length;
    for (const line of output) this.#lines.push(line);
    if (this.#lines.length > this.#limit) {
      this.#lines.splice(0, this.#lines.length - this.#limit);
    }
    return output.length ? output.join('\n') + '\n' : '';
  }

  replay(): string {
    return this.#lines.length ? this.#lines.join('\n') + '\n' : '';
  }

  clear(): void {
    this.#lines = [];
    this.lineCount = 0;
  }
}

/** Runner control tags become readable headings and severity labels. */
export function formatBuildLog(text: string, colour = true): string {
  return text.replace(
    /^##\[(group|endgroup|command|error|warning|notice|debug)\](.*)$/gm,
    (_tag, kind: string, message: string) => {
      if (kind === 'endgroup') return '';
      const label = kind === 'error' ? 'Error: ' : kind === 'warning' ? 'Warning: ' : '';
      const plain = kind === 'command' ? `$ ${message}` : label + message;
      if (!colour) return plain;
      const code =
        kind === 'group'
          ? '1'
          : kind === 'error'
            ? '31'
            : kind === 'warning'
              ? '33'
              : kind === 'command' || kind === 'debug'
                ? '2'
                : '0';
      return `\x1b[${code}m${plain}\x1b[0m`;
    },
  );
}
