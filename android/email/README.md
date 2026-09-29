# TKM Email for Android

TKM Email is a separate Android application from TKM Wallet. It contains only
the EmailVM client and does not start `gtkm`, access the wallet keystore, or
store recovery phrases. Mail registration and shielded payment signing remain
in the wallet or the authenticated EmailVM web client.

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
hosted client performs local message encryption and requires the normal
TKM/Tor network path. The URL is a build-time constant in `MainActivity.java`
so production builds can point to a different authenticated EmailVM endpoint
without sharing wallet credentials.
