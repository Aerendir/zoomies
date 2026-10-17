import type { ComparisonCaptureRequest } from '../api/types';

/** Refuse rounding a GitHub identity before JSON serialisation. */
export function parseAttempts(text: string): ComparisonCaptureRequest['baseline'] {
  const lines = text.trim().split(/\n/);
  if (!text.trim() || lines.length > 9) throw new Error('Select 1 to 9 attempts per arm.');
  return lines.map((line) => {
    const match = /^\s*(\d+)\s*:\s*(\d+)\s*$/.exec(line);
    if (!match) throw new Error('Enter one run ID:attempt per line, for example 123456:1.');
    const run_id = Number(match[1]);
    const attempt = Number(match[2]);
    if (
      !Number.isSafeInteger(run_id) ||
      run_id < 1 ||
      !Number.isSafeInteger(attempt) ||
      attempt < 1
    )
      throw new Error('Run IDs and attempts must be positive, safely representable whole numbers.');
    return { run_id, attempt };
  });
}

export function validateSelection(
  a: ComparisonCaptureRequest['baseline'],
  b: ComparisonCaptureRequest['candidate'],
): void {
  const all = [...a, ...b];
  if (all.length > 10) throw new Error('Select at most 10 attempts across both arms.');
  if (new Set(all.map((x) => `${x.run_id}:${x.attempt}`)).size !== all.length)
    throw new Error('An exact run attempt can appear only once across both arms.');
}

export function measuredMS(value: number | null): string {
  return value === null ? 'Unknown' : `${value.toLocaleString()} ms`;
}
