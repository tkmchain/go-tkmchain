# TKM Email desktop app

`tkm-email` is the standalone desktop launcher for TKM EmailVM. It is kept
separate from `gtkm` and does not start a blockchain node, open a wallet
keystore, or handle recovery phrases.

```sh
make tkm-email
./build/bin/tkm-email
```

`make production` builds this launcher alongside `gtkm` and the managed
shielded prover, so a production build always contains the node and the
standalone email app together.

The launcher always serves the client through a loopback reverse proxy whose
upstream dialer is Tor SOCKS5. If Tor is not listening, requests fail with
`502`; there is no direct-network fallback. Change the SOCKS endpoint only
when using a controlled Tor installation:

```sh
./build/bin/tkm-email --tor-socks5 socks5://127.0.0.1:9150
```

For a native WebView window instead of the system browser, build:

```sh
make tkm-email-gui
./build/bin/tkm-email-gui --no-open
```

For a self-hosted or onion client:

```sh
./build/bin/tkm-email --url https://mail.tkmchain.site/ --no-open
```

The client performs message encryption locally. TKM spending and proof signing
remain in the wallet, so separating the app does not create a second place that
can access wallet secrets.

Prepared EmailVM actions can be handed to the wallet with a
`tkmwallet://emailvm?payload=...` URI. The wallet treats the payload as a
review request and never signs or broadcasts it automatically.
