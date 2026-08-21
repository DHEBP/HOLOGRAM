#!/usr/bin/env bash
# A14 invariant gates for the data-dir consolidation.
# Source of truth: redteam-hologram-datadir-consolidation.md (residual risk #4).
#
# These lock the load-bearing invariants that keep the shared globals "--data-dir"
# key INERT in-process and keep all HOLOGRAM state under ~/.dero/hologram. A future
# edit that regresses any of these silently breaks the redteam's criteria 5/6 safety
# argument, so this runs as a pre-commit / CI gate (see `make check-invariants`).
set -uo pipefail
cd "$(dirname "$0")/.." || exit 2

fail=0
note() { echo "  [FAIL] $*"; fail=1; }
# Scope: the HOLOGRAM GUI app package. Excludes vendored/generated dirs and the
# standalone dev tools (tela-cli, cmd/*) which legitimately operate on the CWD.
scan() {
  grep -rnE "$1" --include=*.go \
    --exclude-dir=vendor --exclude-dir=.git --exclude-dir=.task --exclude-dir=.claude \
    --exclude-dir=build --exclude-dir=dist --exclude-dir=node_modules \
    --exclude-dir=tela-cli --exclude-dir=cmd \
    . 2>/dev/null
}

echo "[A14] data-dir consolidation invariant gates"

# 1. No in-process caller of globals.GetDataDirectory in the HOLOGRAM package.
#    (walletapi opens wallets by absolute path; the --data-dir stub must stay unread.)
if scan 'globals\.GetDataDirectory\(' ; then
  note "HOLOGRAM must not call globals.GetDataDirectory() in-process"
fi

# 2. Zero GetDataDirectory references in the embedded walletapi / Transmission engine
#    — a reader there would make the shared key visible to the live-receive-drop debug.
for dep in ../derohe/walletapi ../Transmission ; do
  if [ -d "$dep" ]; then
    if grep -rnE '\.GetDataDirectory\(' --include=*.go "$dep" 2>/dev/null | grep -v '/vendor/' ; then
      note "$dep must contain zero GetDataDirectory() call sites"
    fi
  fi
done

# 3. os.Getwd() is only sanctioned in paths.go (the single CWD fallback). Everything
#    else must derive paths from getHologramDataDir()/getDatashardsDir().
if scan 'os\.Getwd\(' | grep -v '^\./paths\.go:' | grep -v '_test\.go:' ; then
  note "os.Getwd() outside paths.go — repoint via getHologramDataDir()/getDatashardsDir()"
fi

# 4. No CWD-relative datashards path (the old RemoveFile / store bug).
if scan 'Join\("\.", *"datashards"' ; then
  note 'filepath.Join(".", "datashards") is CWD-relative — anchor to getDatashardsDir()'
fi

# 5. No phantom gravdb- path — the storage panel must use gnomonDBDir(network).
if scan 'gravdb-' ; then
  note "gravdb- path is a phantom — use gnomonDBDir(network)"
fi

# 6. No dead network_mode settings key — every writer uses settings["network"].
if scan 'settings\["network_mode"\]' ; then
  note 'settings["network_mode"] is never written — read settings["network"]'
fi

if [ "$fail" -ne 0 ]; then
  echo "[A14] INVARIANTS VIOLATED"
  exit 1
fi
echo "[A14] all invariants hold"
