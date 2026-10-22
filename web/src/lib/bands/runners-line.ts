/**
 * What the Runners page's band says: how many runners are live and how they split,
 * in words, from the counts the page already has.
 *
 * A plain function, as the Overview's status line is, so the sentence can be held to
 * the numbers by a test. A band that said "5 runners are live" over a count that
 * left out the ones still starting up would be wrong in a way a layout change is
 * very good at hiding.
 */

export interface RunnerCounts {
  provisioning: number;
  registering: number;
  idle: number;
  busy: number;
  draining: number;
}

export interface RunnersLine {
  headline: string;
  /** The line drawn in the brand's blue; empty when there is nothing to split. */
  accent: string;
  lede: string;
}

const runners = (n: number) => `${n} ${n === 1 ? 'runner' : 'runners'}`;

export function runnersLine(counts: RunnerCounts): RunnersLine {
  const starting = counts.provisioning + counts.registering;
  const live = starting + counts.idle + counts.busy + counts.draining;
  if (live === 0) {
    return {
      headline: 'No runners are live.',
      accent: '',
      lede: 'The next job that needs one will start it.',
    };
  }
  const notes = [
    starting > 0 ? `${starting} ${starting === 1 ? 'is' : 'are'} starting up` : '',
    counts.draining > 0
      ? `${counts.draining} ${counts.draining === 1 ? 'is' : 'are'} draining`
      : '',
  ].filter(Boolean);
  return {
    headline: `${runners(live)} ${live === 1 ? 'is' : 'are'} live.`,
    accent: `${counts.busy} busy, ${counts.idle} idle.`,
    lede: notes.length ? `${notes.join(' and ')}.` : 'Every one is working or ready for work.',
  };
}
