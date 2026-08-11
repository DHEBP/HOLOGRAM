// Prefix-only test used to answer "did the user type an address or a name?".
// It is NOT a validity check — the authoritative parse is rpc.NewAddress on the Go side,
// which is what decides malformed / integrated / wrong-network. Keep it that way: widening
// this into real validation duplicates a consensus-adjacent parser in the frontend.
export function looksLikeDeroAddress(value) {
  const v = (value || '').trim();
  return v.startsWith('dero1') || v.startsWith('deroi1') || v.startsWith('deto1') || v.startsWith('detoi1');
}
