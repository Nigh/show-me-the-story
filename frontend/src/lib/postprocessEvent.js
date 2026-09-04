/**
 * Normalize postprocess SSE/API payloads into the store shape { book_complete, state }.
 * Backend historically emitted bare PostProcessState on `postprocess_update`; API responses
 * and newer emitters use the wrapped shape. Accept both.
 */
export function normalizePostProcessPayload(d, prev) {
  if (d && typeof d === 'object' && 'book_complete' in d && d.state && typeof d.state === 'object' && !Array.isArray(d.state)) {
    return d;
  }
  if (d && typeof d === 'object') {
    return prev ? { ...prev, state: d } : { book_complete: true, state: d };
  }
  return prev ?? null;
}
