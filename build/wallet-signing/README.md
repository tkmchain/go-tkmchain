# Wallet publisher metadata and release signing

The publisher name is the real name supplied by the release owner. TKMChain is the provisional Android resource default; set TKM_PUBLISHER to the intended name before a release. Publisher metadata does not create a trusted Windows signature.

## Windows

Generate version resources for both executables before rebuilding:

    python3 build/wallet-signing/windows-metadata.py --publisher 'Your publisher name' --version 0.5.0
    make gtkm-gui-windows
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o build/bin/shielded-payout-prover.exe ./cmd/shielded-payout-prover

Sign both executables on the machine holding the trusted certificate. The script uses the certificate store, so passwords do not appear on the command line:

    ./build/wallet-signing/sign-windows.ps1 -CertificateThumbprint YOUR_CERTIFICATE_THUMBPRINT -TimestampUrl YOUR_CA_RFC3161_TIMESTAMP_URL -Files ./build/bin/gtkm-gui-windows-amd64.exe,./build/bin/shielded-payout-prover.exe

The script applies SHA-256 signatures and RFC 3161 timestamps, and verifies each signature with SignTool. It does not install a certificate or change trust settings. Repackage the Windows ZIP and regenerate SHA256SUMS only after verification. A self-signed certificate is not a substitute for a trusted publisher certificate.

## Android

Keep the release keystore and password files outside the repository, backed up securely. Supply file paths and an alias through these environment variables:

- TKM_PUBLISHER
- TKM_ANDROID_KEYSTORE (absolute path)
- TKM_ANDROID_KEY_ALIAS
- TKM_ANDROID_STORE_PASSWORD_FILE
- TKM_ANDROID_KEY_PASSWORD_FILE

Password files contain only the relevant password, optionally followed by a newline. Restrict access to the signing user. Never put passwords in source, logs or chat.

    TKM_ANDROID_BUILD_VARIANT=release ./android/build.sh

The result is android/app/build/outputs/apk/release/app-release.apk. Run the Android SDK apksigner verify command on it before distributing. Missing signing configuration fails the build rather than producing an unsigned release. The default build variant remains debug for development.

An existing Android installation normally needs an update signed with the same key. A new release key cannot simply replace the existing debug certificate. Preserve wallet recovery material before planning that migration; signing does not itself migrate installed wallet data.

References: https://learn.microsoft.com/en-us/windows/win32/seccrypto/signtool and https://developer.android.com/studio/publish/app-signing
