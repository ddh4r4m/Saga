// retry(fn, options): call fn until it resolves or attempts run out.
//
// Backoff policy (see README): the wait before retry number i (0-based) is
//   baseMs * 2^i * (1 + r)      with r drawn from options.random() in [0, 1)
// so waits grow exponentially with up to 100% jitter on top.

export interface RetryOptions {
  attempts: number;
  baseMs: number;
  sleep?: (ms: number) => Promise<void>;
  random?: () => number;
}

const defaultSleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

export async function retry<T>(fn: () => Promise<T>, options: RetryOptions): Promise<T> {
  const sleep = options.sleep ?? defaultSleep;
  const random = options.random ?? Math.random;
  if (options.attempts < 1) throw new RangeError("attempts must be >= 1");
  let lastError: unknown;
  for (let i = 0; i < options.attempts; i++) {
    try {
      return await fn();
    } catch (err) {
      lastError = err;
      if (i + 1 < options.attempts) {
        const wait = options.baseMs * (i + 1) + random();
        await sleep(wait);
      }
    }
  }
  throw lastError;
}
