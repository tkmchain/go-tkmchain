# TKM Email for Android

TKM Email is a separate Android application from TKM Wallet. It contains only
the EmailVM client and does not start `gtkm`, access the wallet keystore, or
store recovery phrases. After keyfile login the hosted client opens only the
EmailVM workspace; wallet address and balance are not shown. It removes wallet send/receive,
shielded-wallet, phone, king, and transaction-history controls.

Build both Android applications from the repository root:

```sh
TKM_ANDROID_BUILD_VARIANT=debug ./android/build.sh
```

The standalone package is:

```text
android/email/build/outputs/apk/debug/email-debug.apk
```

The app opens the wallet's email interface at
`https://wallet.tkmchain.site/?app=email`. Android and desktop therefore use
the same keyfile login, EmailVM registry, encrypted inbox, and send flow. The
authenticated surface is labeled `EmailVM · private by default`; wallet
management actions remain in TKM Wallet. Paid mailbox and domain actions are
limited to the local EmailVM flow and require enough TKM. Wallet
handoff links are rejected inside this app; use TKM Wallet separately for
transfers. The hosted client performs local message encryption and requires the normal
TKM/Tor network path. The URL is a build-time constant in `MainActivity.java`
so production builds can point to a different authenticated EmailVM endpoint
without sharing wallet credentials.

## Encrypted keyfile login

Tap **Choose encrypted keyfile** in the hosted login screen. The Android app
opens the system document picker and passes the selected `content://` document
to the WebView through the standard file-input callback. A canceled selection
clears the pending callback, and no direct filesystem path or recovery phrase
is stored by the app.
