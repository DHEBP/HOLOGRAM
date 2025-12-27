/**
 * Wails Runtime Utilities
 * 
 * Helpers for ensuring Wails bindings are ready before use.
 */

/**
 * Wait for Wails Go bindings to be available.
 * This resolves a timing issue where Svelte components mount before
 * window['go']['main']['App'] is initialized by the Wails runtime.
 * 
 * @param {number} timeout - Max time to wait in ms (default: 5000)
 * @param {number} interval - Polling interval in ms (default: 50)
 * @returns {Promise<boolean>} - Resolves true when ready, rejects on timeout
 */
export function waitForWails(timeout = 5000, interval = 50) {
  return new Promise((resolve, reject) => {
    const startTime = Date.now();
    
    function check() {
      // Check if Wails Go bindings are available
      if (window['go']?.['main']?.['App']) {
        resolve(true);
        return;
      }
      
      // Check for timeout
      if (Date.now() - startTime > timeout) {
        reject(new Error('Wails runtime initialization timeout'));
        return;
      }
      
      // Try again
      setTimeout(check, interval);
    }
    
    check();
  });
}

/**
 * Check if Wails Go bindings are currently available.
 * Use this for synchronous checks without waiting.
 * 
 * @returns {boolean}
 */
export function isWailsReady() {
  return !!(window['go']?.['main']?.['App']);
}

/**
 * Safely call a Wails backend function, waiting for runtime if needed.
 * 
 * @param {Function} fn - The Wails function to call
 * @param {...any} args - Arguments to pass to the function
 * @returns {Promise<any>}
 */
export async function safeWailsCall(fn, ...args) {
  await waitForWails();
  return fn(...args);
}

