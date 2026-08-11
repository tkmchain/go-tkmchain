#!/usr/bin/env bash
set -euo pipefail

# Dev-only single-participant shielded ceremony example.
# Run from repo root: ./scripts/run-single-party-ceremony.sh

WORKDIR=/tmp/shielded-ceremony-$(date +%s)
mkdir -p "$WORKDIR"
cd "$WORKDIR"

echo "Working dir: $WORKDIR"

echo "Phase 1: init"
go run ../../cmd/shielded-ceremony init-phase1 -out phase1-0.bin

echo "Phase 1: contribute (single participant)"
go run ../../cmd/shielded-ceremony contribute-phase1 -in phase1-0.bin -out phase1-1.bin

echo "Phase 1: verify"
go run ../../cmd/shielded-ceremony verify-phase1 -beacon 0xDEADBEEF -out commons.bin phase1-1.bin

echo "Phase 2: init"
go run ../../cmd/shielded-ceremony init-phase2 -commons commons.bin -out phase2-0.bin

echo "Phase 2: contribute (single participant)"
go run ../../cmd/shielded-ceremony contribute-phase2 -in phase2-0.bin -out phase2-1.bin

echo "Finalize: produce proving.key, verifying.key, verifying.hex"
go run ../../cmd/shielded-ceremony finalize -commons commons.bin -beacon 0xDEADBEEF -pk proving.key -vk verifying.key -vk-hex verifying.hex phase2-1.bin

echo "Done. proving.key (private) and verifying.hex (public) are in $WORKDIR"

# Show verifying hex summary
echo "verifying.hex (first 256 chars):"
head -c 256 verifying.hex | xxd -p | sed 's/\(..\)/\1 /g' || true

echo "IMPORTANT: move proving.key to a secure prover host and do NOT commit it to source control."