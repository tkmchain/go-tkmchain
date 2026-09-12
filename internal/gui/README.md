# Native TKM wallet

TKM Wallet — Windows x64 / Android arm64

Windows: extract the complete ZIP to one folder, then open TKM-Wallet-Windows-x64.exe. Keep shielded-payout-prover.exe beside it. Microsoft WebView2 Runtime is required. The wallet starts a local full node; initial synchronization and verified proof-key downloads can take time.

Android: install TKM-Wallet-Android-arm64-debug.apk on an ARM64 Android 8+ device. This is a debug-signed testing build. Update an existing installation only when its signing key matches; back up recovery material before any uninstall. Node and prover executables are installed by Android under its native library directory.

Recovery: Add wallet generates or restores 24 words. Receive > Private balance & recovery can reveal an existing PQ account's phrase after password verification. TKM PQ recovery v1 represents the exact 32-byte ML-DSA seed using BIP39 entropy/checksum words. It is not an Ethereum HD wallet derivation and has no additional mnemonic passphrase. Preserve the words privately. Never share them with support.

Transfers: use the full shield2 receiving code. The wallet validates recipients, builds proofs through its local prover, signs locally and records attempted transaction hashes. Shielded balance scanning requires the wallet password. Public and shielded balances are shown separately.

Validation: Go GUI/proxy and cross-language identity tests; 16 JavaScript recovery/signing/Mail tests; Chromium Mail/Phone flows at widths 320, 390 and 1440, with earlier wallet checks also at 820; Windows cross-build; Android Gradle build and APK inspection. No real funds were transferred. Native Windows and physical Android runtime verification remains necessary before production release.

Developer checks: `go test ./internal/gui`; in `wallet-engine`, run `npm ci`, `npm test`, and `npm run build` before native builds. Build Windows with `make gtkm-gui-windows` and its adjacent Windows prover; build Android with `./android/build.sh`. The embedded engine uses the same PQ identity and shield2 format as the Go node, covered by a public deterministic fixture.

Mail and Phone (0.5.0): Mail registers mailboxes, publishes the seed-derived X25519 mail key, encrypts messages locally, and signs proof-bound actions with ML-DSA-87. Payment, gas and confirmation are shown explicitly. Registration and key publication are separate confirmed steps; after an interruption, check registration and use Publish Mail key if needed. Inbox and Sent Mail support pagination and local decryption. Existing mailboxes with a different published mail key are protected against accidental replacement; their original private key is still required.

Phone now verifies domain-separated ML-DSA-87 owner and device signatures. Registration and sending use PQ locally. Move a number to PQ allows its existing ECDSA or PQ owner to sign a transfer to a local PQ account. Transfer removes previous device/recovery authority; register the new device afterward. ECDSA verification remains for existing records and migration, so legacy users are not silently locked out. This is a Phone service upgrade, not a new block-consensus fork. Upgrade participating Phone nodes together: older nodes cannot verify PQ actions, and new device/transfer propagation requires owner signatures.

No live funds or phone ownership were changed during validation. Tests use public deterministic fixtures, fake RPC/prover responses, and local browser flows. Physical Android and native Windows runtime testing remains required before production release.
