#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source_file="$root_dir/contracts/smartaccount/TKMSmartAccounts.sol"
source_rel="contracts/smartaccount/TKMSmartAccounts.sol"
artifact_dir="$root_dir/contracts/smartaccount/artifacts"
build_dir="$(mktemp -d)"
trap 'rm -rf "$build_dir"' EXIT

(cd "$root_dir" && npx -y solc@0.8.30 --bin --abi "$source_rel" contracts/smartaccount/TKMPoolTreasurySmartAccount.sol -o "$build_dir")
mkdir -p "$artifact_dir"

for contract in TKMEntryPoint TKMAccount TKMAccountFactory TKMAllowlistPaymaster TKMPoolTreasurySmartAccount; do
  prefix="${source_rel%.sol}_sol_${contract}"
  if [[ "$contract" == "TKMPoolTreasurySmartAccount" ]]; then prefix="contracts/smartaccount/TKMPoolTreasurySmartAccount_sol_${contract}"; fi
  prefix="${prefix//\//_}"
  cp "$build_dir/$prefix.abi" "$artifact_dir/$contract.abi"
  cp "$build_dir/$prefix.bin" "$artifact_dir/$contract.bin"
done

echo "Smart-account artifacts updated in $artifact_dir"
