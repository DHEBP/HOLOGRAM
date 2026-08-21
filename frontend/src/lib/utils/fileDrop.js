/**
 * Helpers for Wails native OnFileDrop (Linux GTK coords vs CSS layout).
 * See https://github.com/wailsapp/wails/issues/3686
 */

/**
 * Convert native drop coordinates to CSS pixels for getBoundingClientRect().
 * GTK/Wails may supply device pixels; layout uses CSS pixels.
 */
export function dropCoordsToCSS(x, y) {
  const dpr = window.devicePixelRatio || 1;
  if (dpr <= 1) {
    return { x, y };
  }
  return { x: x / dpr, y: y / dpr };
}

/**
 * Resolve native drop coordinates to the CSS-pixel point inside element, or null if the drop
 * landed outside it. Tries CSS-scaled coords first, then raw (platform variance).
 *
 * Returning the point rather than a boolean is deliberate. A caller that needs to pass the
 * position on — to elementFromPoint inside the frame, say — must use the SAME space the hit
 * test matched in, and a boolean cannot tell it which one that was. Passing the raw coords
 * after a CSS-scaled match aims at devicePixelRatio times the intended offset, which is
 * invisible at dpr 1 and lands nowhere near the cursor on a scaled display.
 */
export function resolveDropPoint(x, y, element) {
  if (!element) {
    return null;
  }
  const rect = element.getBoundingClientRect();
  const css = dropCoordsToCSS(x, y);
  if (pointInRect(css.x, css.y, rect)) {
    return css;
  }
  if (pointInRect(x, y, rect)) {
    return { x, y };
  }
  return null;
}

/**
 * True if native drop coordinates fall inside element's bounding box.
 */
export function isDropPointInElement(x, y, element) {
  return resolveDropPoint(x, y, element) !== null;
}

function pointInRect(x, y, rect) {
  return x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom;
}
