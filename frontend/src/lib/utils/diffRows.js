/**
 * Cap a generateDiff row list by CHANGED rows only.
 *
 * generateDiff interleaves context and gap rows between the added / removed /
 * modified ones. A cap that counts raw rows silently shrinks what it shows —
 * a context-heavy diff could surface a seventh of the changes it used to —
 * and its "N more lines" footer counts scenery as change. So the cap counts
 * only changed rows and lets context and gaps ride free; they are bounded by
 * construction at six rows plus a gap per hunk.
 */
export function isChangedRow(row) {
  return row.type === 'added' || row.type === 'removed' || row.type === 'modified';
}

/**
 * Cut the list at the row where the changed-row count passes maxChanged, and
 * report how many changed rows fell past it for the footer.
 */
export function capDiffRows(lineDiffs, maxChanged) {
  const list = lineDiffs || [];
  let changed = 0;
  let cut = list.length;
  for (let i = 0; i < list.length; i++) {
    if (isChangedRow(list[i]) && ++changed > maxChanged && cut === list.length) {
      cut = i;
    }
  }
  return { rows: list.slice(0, cut), hiddenChanged: Math.max(0, changed - maxChanged) };
}
