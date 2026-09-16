# Native TKM wallet

TKM Wallet — Windows x64 / Android arm64

Windows: extract the complete ZIP to one folder, then open TKM-Wallet-Windows-x64.exe. Keep shielded-payout-prover.exe beside it. Microsoft WebView2 Runtime is required. The wallet starts a local full node; initial synchronization and verified proof-key downloads can take time.

Android: install TKM-Wallet-Android-arm64-debug.apk on an ARM64 Android 8+ device. This is a debug-signed testing build. Update an existing installation only when its signing key matches; back up recovery material before any uninstall. Node and prover executables are installed by Android under its native library directory.

Recovery: Add wallet generates or restores 24 words. Receive > Private balance & recovery can reveal an existing PQ account's phrase after password verification. TKM PQ recovery v1 represents the exact 32-byte ML-DSA seed using BIP39 entropy/checksum words. It is not an Ethereum HD wallet derivation and has no additional mnemonic passphrase. Preserve the words privately. Never share them with support.

Transfers before Antartical: use the full shield2 receiving code. The wallet validates recipients, builds proofs through its local prover, signs locally and records attempted transaction hashes. Shielded balance scanning requires the wallet password. Public and shielded balances are shown separately.


## Shield3 at Antartical

Mainnet activation is October 1, 2026 at 00:00 UTC. New wallets require a private
name/country stamp before account creation; existing PQ wallets can add their
original stamp once in Receive & backup. Save the encrypted keyfile to preserve
that original stamp alongside the 24 recovery words.

Use the full `tkmshield3.` receiving address for private sends. The maximum is
5,000,000 TKM per transaction, plus network fees. The embedded native prover
requires one confirmed note covering the requested amount and fees. Viewing
keys reveal incoming and outgoing note history; the separate stamp key reveals
name and country. Receiving public keys cannot decrypt either.

Shield public funds creates a Shield3 note from the public balance and exposes
that funding amount. To move old Shield2 notes, use Migrate one Shield2 note,
wait for its confirmation, then shield the resulting public balance. This
legacy migration also exposes its withdrawal amount. Private note amounts are
encrypted; outer signer accounts, timing, gas and fees remain public.

Submitted hashes appear in Activity as awaiting confirmation until their
receipts arrive. Shield3 saves exact signed transaction bytes locally before
broadcast so the same request can be retried after an RPC timeout or restart
without creating another proof or spending another note.

Builds require Rust 1.89 and CGO. Add the `x86_64-pc-windows-gnu` target for Windows
and `aarch64-linux-android` for Android; release.yml installs both where needed.
See [Shield3 implementation and testing](../../zk/shielded3/README.md).

Validation: Go GUI/proxy and cross-language identity tests; 18 JavaScript recovery/signing/Mail/migration/boundary tests; Chromium Mail/Phone flows at widths 320, 390 and 1440, with earlier wallet checks also at 820; Windows cross-build; Android Gradle build and APK inspection. No real funds were transferred. Native Windows and physical Android runtime verification remains necessary before production release.

Developer checks: `go test ./internal/gui`; in `wallet-engine`, run `npm ci`, `npm test`, and `npm run build` before native builds. Build Windows with `make gtkm-gui-windows` and its adjacent Windows prover; build Android with `./android/build.sh`. The embedded engine uses the same PQ identity and shield2 format as the Go node, covered by a public deterministic fixture.

Mail and Phone (0.5.0): Mail registers mailboxes, publishes the seed-derived X25519 mail key, encrypts messages locally, and signs proof-bound actions with ML-DSA-87. Payment, gas and confirmation are shown explicitly. Registration and key publication are separate confirmed steps; after an interruption, check registration and use Publish Mail key if needed. Inbox and Sent Mail support pagination and local decryption. Existing mailboxes with a different published mail key are protected against accidental replacement; their original private key is still required.

Phone now verifies domain-separated ML-DSA-87 owner and device signatures. Registration and sending use PQ locally. Move a number to PQ allows its existing ECDSA or PQ owner to sign a transfer to a local PQ account. Transfer removes previous device/recovery authority; register the new device afterward. ECDSA verification remains for existing records and migration, so legacy users are not silently locked out. This is a Phone service upgrade, not a new block-consensus fork. Upgrade participating Phone nodes together: older nodes cannot verify PQ actions, and new device/transfer propagation requires owner signatures.

No live funds or phone ownership were changed during validation. Tests use public deterministic fixtures, fake RPC/prover responses, and local browser flows. Physical Android and native Windows runtime testing remains required before production release.

### Consensus stamps at Antartical

Android embeds the same wallet UI and local backend as the desktop GUI. Send
and Receive show stamping first: enter private name/country labels or restore
the original encrypted backup, register the stamp, and check confirmation.
Registration transfers zero TKM and requires a public balance for normal gas
fees. Sending, receiving-code sharing, funding and legacy migration remain
locked until the immutable on-chain stamp confirms. A wallet-file stamp alone
is insufficient; block execution and both transaction pools enforce the gate.
Private proofs require registered output owners without publishing the payment
recipient or its registry index. Registration hashes appear in Activity, and
stable request IDs preserve signed registration bytes across retries/restarts.
