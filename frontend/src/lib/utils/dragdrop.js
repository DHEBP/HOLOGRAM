/**
 * Is this drag carrying files from outside the app?
 *
 * 'Files' alone is not enough. Measured on WebKitGTK 2026-08-09: a file dragged from the
 * desktop file manager advertises exactly `text/uri-list,text/html` and never 'Files', so
 * a guard keying only on 'Files' does nothing — and because nothing then cancels dragover,
 * WebKit never dispatches drop and navigates the webview to the file instead.
 *
 * types is read array-agnostically: it is a FrozenArray per spec, but a DOMStringList in
 * older engines has contains() and no includes(), and calling undefined throws inside the
 * listener, which would silently disarm whichever guard called it.
 */
export function isFileDrag(types) {
  if (!types) return false;
  return Array.prototype.indexOf.call(types, 'Files') !== -1 ||
         Array.prototype.indexOf.call(types, 'text/uri-list') !== -1;
}
