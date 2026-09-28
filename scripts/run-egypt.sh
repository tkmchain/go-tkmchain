#!/bin/sh
set -eu

# Egypt is deliberately isolated from the production node. Do not point this
# launcher at ~/.tkmchain or a production datadir.
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
binary=${GTKM_BIN:-"$repo_root/build/bin/gtkm"}
datadir=${TKM_EGYPT_DATADIR:-"$HOME/.tkmchain-egypt"}

case "$datadir" in
  "$HOME/.tkmchain"|"$HOME/.tkmchain/"*)
    echo "refusing to run Egypt with production datadir: $datadir" >&2
    exit 1
    ;;
esac

if [ ! -x "$binary" ]; then
  echo "gtkm binary is not executable: $binary" >&2
  echo "build it first with: make gtkm" >&2
  exit 1
fi

mkdir -p "$datadir"
exec "$binary" --egypt --datadir "$datadir" "$@"
