# Shield6 production parameters

The production release uses fresh BN254 Groth16 parameters for the version 2
recipient-output and registration circuits. This is a **single-operator trusted
setup**, using operating-system randomness. It is not an independent multiparty
ceremony and has not received an independent cryptographic audit. A compromise
of setup randomness would compromise proof soundness. Test-only setup files
must never be substituted for these parameters.

`params/shield6_production.go` pins both verification keys and the proving-key
hashes in consensus. `params/shield6_production.json` records the public artifact
hashes, sizes, setup provenance, release version and activation time. Mainnet
and its RandomX configuration activate Shield6, recipient proofs and confidential
backing at the same timestamp. Other networks retain their own schedules.

Download the exact public artifacts from the checked-out release:

```sh
python3 scripts/shield6-release-parameters.py --output "$HOME/.tkmchain/shield6-parameters"
export TKM_SHIELD6_RECIPIENT_PARAMETERS="$HOME/.tkmchain/shield6-parameters"
```

The downloader authenticates all ten files before publishing each file. Wallet
loaders independently check the manifest against the node's consensus pins,
then authenticate parameters again before deserialization. The GitHub release
also packages these artifacts as `Shield6-parameters-v2-<version>.tar.gz`.
Desktop wallets use the parameter directory environment variable;
Android imports the eight circuit/key/manifest files through its parameter
picker. The two provenance files describe setup and are not wallet secrets.

Release validation downloads these same production artifacts and uses them for
registration, deposit, private payment, TCP propagation, canonical import and
reorg checks. It fails if their proving or verification pins differ from the
mainnet configuration. It never generates substitute parameters for that gate.

The generator is available for a future, separately coordinated setup:

```sh
shielded-ceremony -circuit shield6-recipient-v2 generate-local -single-operator -out-dir fresh-parameters
shielded-ceremony -circuit shield6-registration-v2 generate-local -single-operator -out-dir fresh-parameters
```

It checks the compiled circuit fingerprints, refuses existing artifacts and
publishes the manifest last. New parameters require a new consensus deployment;
they must not replace an active release's files or keys.
