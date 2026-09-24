# Fast chain bootstrap

`gtkm bootstrap` downloads an RLP block archive through Tor, verifies an optional
SHA-256 digest, stores the archive in the node data directory, and imports the
blocks in batches using the normal consensus validation path. It does not copy a
foreign database into `chaindata`, so the archive can be inspected and retained
if an import is interrupted.

## Destination

The archive is placed under the platform-specific `gtkm` instance directory:

- Linux: `~/.tkmchain/gtkm/bootstrap/`
- Windows: `%LOCALAPPDATA%\Tkmchain\gtkm\bootstrap\` (or the configured
  Windows Tkmchain data directory)

Pass `--datadir <base>` when the node uses a custom data directory. The command
adds the normal `gtkm` instance directory below that base, matching node startup.

## Create an archive on a synced node

Stop mining only if the node needs exclusive disk access. Export the canonical
blocks from the source node; the export command validates the chain while it
writes the RLP stream:

```bash
head=$(./build/bin/gtkm --datadir "$HOME/.tkmchain" \
  attach --exec 'eth.blockNumber' | tr -d '\n')
./build/bin/gtkm --datadir "$HOME/.tkmchain" \
  export /srv/bootstrap/tkm-mainnet.rlp.gz 0 "$head"
sha256sum /srv/bootstrap/tkm-mainnet.rlp.gz
```

Publish the archive and its digest over an HTTPS or Tor onion URL. The digest
must be communicated separately from the download URL.

## Official archive

The official current-chain archive is published at:

`https://tkmchain.site/tkm-mainnet.rlp.gz`

Published SHA-256: `1a0137cdbef67217edbe6d4fb7001666897f477335461b07d2f03dcab2b2f779`

## Bootstrap a new or partially synced node

Stop any running `gtkm` process first. Tor must be listening on
`127.0.0.1:9050` unless another SOCKS5 endpoint is supplied:

```bash
./build/bin/gtkm bootstrap \
  --url 'https://tkmchain.site/tkm-mainnet.rlp.gz' \
  --sha256 '1a0137cdbef67217edbe6d4fb7001666897f477335461b07d2f03dcab2b2f779' \
  --datadir "$HOME/.tkmchain"
```

For an onion URL, the same command works because the download is routed through
Tor. The command stores the file under `~/.tkmchain/gtkm/bootstrap/` and imports
it immediately. It skips the extra full-database compaction pass so the node can
be started and continue peer synchronization sooner.

To download without importing:

```bash
./build/bin/gtkm bootstrap \
  --url 'https://bootstrap.example/onion/tkm-mainnet.rlp.gz' \
  --sha256 '<64-hex-character-sha256>' \
  --no-import
```

The command prints the exact archive path and digest. If the import fails, the
archive remains in that directory and can be retried with the regular import
command after correcting the problem:

```bash
./build/bin/gtkm --datadir "$HOME/.tkmchain" \
  import "$HOME/.tkmchain/gtkm/bootstrap/chain-<timestamp>.rlp.gz" \
  --no-compaction
```

The import still validates every block, parent link, transaction, RandomX seal,
mix digest, and consensus rule. A bootstrap archive cannot bypass consensus or
replace an existing canonical chain.
