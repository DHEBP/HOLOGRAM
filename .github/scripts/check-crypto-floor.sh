#!/usr/bin/env bash
# Fail CI if golang.org/x/crypto or golang.org/x/net resolves below the security floor
# that clears their SSH / net-package CVEs. Each pin is an // indirect line in go.mod,
# elevated by MVS via the derohe fork's walletapi; a `go mod tidy` finding no DIRECT
# requirer could silently drop it back to the transitive v0.17.0-era minimum and re-open
# the alerts. We check the RESOLVED build version (not the go.mod text) so a dropped line
# still fails. Needs the Go toolchain -> run this step AFTER "Set up Go".

set -euo pipefail

# module -> minimum acceptable version (the CVE patch floor)
FLOORS=(
  "golang.org/x/crypto v0.52.0"
  "golang.org/x/net    v0.55.0"
)

fail=0
for entry in "${FLOORS[@]}"; do
  mod="${entry%% *}"
  floor="${entry##* }"

  v="$(go list -m -f '{{.Version}}' "$mod")" || {
    echo "::error::cannot resolve ${mod} — is it still in the build graph?"
    fail=1
    continue
  }

  # sort -V: if v >= floor, the smaller of {v,floor} is floor.
  if [ "$(printf '%s\n%s\n' "$v" "$floor" | sort -V | head -n1)" != "$floor" ]; then
    echo "::error::${mod} ${v} is below the security floor ${floor} (CVEs) — bump it."
    fail=1
    continue
  fi

  echo "${mod} ${v} >= ${floor} ✓"
done

exit "$fail"
