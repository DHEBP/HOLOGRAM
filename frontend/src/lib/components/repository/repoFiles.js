/**
 * Pure helpers for a TELA repository file list.
 *
 * Kept out of the Svelte components on purpose: this is the logic most likely
 * to be subtly wrong (which file opens first, how a historical name is spelled)
 * and it is plain string work, so it can be exercised without a DOM.
 */

/** The file name without its directory. Always a string, never the input. */
export function baseName(name) {
  const s = name || '';
  const cut = s.lastIndexOf('/');
  return cut < 0 ? s : s.slice(cut + 1);
}

/**
 * Group entries by directory for the sidebar.
 *
 * A TELA file name already carries its SubDir joined in ("assets/css/app.css"),
 * so the folder structure is in the string and needs no separate walk. Root
 * files come first, then folders in name order — the order a forge shows.
 */
export function groupByFolder(list) {
  const root = [];
  const folders = new Map();

  for (const entry of list || []) {
    const name = entry?.name || '';
    const cut = name.lastIndexOf('/');
    if (cut < 0) {
      root.push(entry);
      continue;
    }
    const dir = name.slice(0, cut);
    if (!folders.has(dir)) folders.set(dir, []);
    folders.get(dir).push(entry);
  }

  const out = [];
  if (root.length > 0) out.push({ dir: '', files: root });
  for (const dir of [...folders.keys()].sort()) {
    out.push({ dir, files: folders.get(dir) });
  }
  return out;
}

/**
 * Narrow a file list to names containing the query.
 *
 * Matches the FULL path ("assets/css/app.css"), case-insensitively, so typing
 * a folder name finds everything under it. A name filter only — never content,
 * never fuzzy.
 */
export function filterEntries(list, query) {
  const q = (query || '').trim().toLowerCase();
  if (!q) return list || [];
  return (list || []).filter((e) => (e?.name || '').toLowerCase().includes(q));
}

/**
 * Which file opens first.
 *
 * Five rules, in order, so the choice is predictable rather than "whatever the
 * INDEX happened to list first". Deliberately does not guess further: a repo
 * with no README and no root index.html opens on its first readable file.
 */
export function pickDefaultFile(list) {
  const all = list || [];
  const docs = all.filter((e) => e?.kind === 'doc');
  const rootDocs = docs.filter((e) => !(e.name || '').includes('/'));

  return (
    rootDocs.find((e) => /^readme\.md$/i.test(e.name || '')) ||
    docs.find((e) => /^readme\.md$/i.test(baseName(e.name || ''))) ||
    rootDocs.find((e) => /\.md$/i.test(e.name || '')) ||
    rootDocs.find((e) => /^index\.html$/i.test(e.name || '')) ||
    docs[0] ||
    all[0] ||
    null
  );
}

/**
 * Turn a historical commit's file map into the same entry shape the current
 * state uses.
 *
 * CloneAtCommit writes under <cloneRoot>/<dURL>/ and the reader keys on the
 * path relative to the clone ROOT, so a historical file name carries a dURL
 * segment that a current-state name does not. Strip it, or the same file reads
 * as two different files between the two views.
 *
 * SCID is empty because a reconstructed version is read off disk, not off a
 * DOC contract — which is also why signatures are not shown for one.
 */
export function entriesFromCommitFiles(files, commitDurl) {
  const prefix = commitDurl ? `${commitDurl}/` : '';
  return Object.keys(files || {})
    .sort()
    .map((key) => {
      const name = prefix && key.startsWith(prefix) ? key.slice(prefix.length) : key;
      const content = files[key] ?? '';
      return {
        name,
        scid: '',
        docType: '',
        kind: 'doc',
        content,
        bytes: content.length,
        reason: ''
      };
    });
}
