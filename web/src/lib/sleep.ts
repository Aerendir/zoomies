/** Resolves after `ms` milliseconds: the one wait the UI's retry and spinner timings are written with. */
export const sleep = (ms: number): Promise<void> =>
  new Promise((resolve) => setTimeout(resolve, ms));
