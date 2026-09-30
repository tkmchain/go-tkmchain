# TKM Email desktop app

`tkm-email` is the standalone desktop launcher for TKM EmailVM. It is kept
separate from `gtkm` and exposes only the email section. It does not start a
blockchain node, open a wallet send/receive screen, expose phone services, or
handle recovery phrases.

```sh
make tkm-email
./build/bin/tkm-email
```

`make production` builds this launcher alongside `gtkm` and the managed
shielded prover, so a production build always contains the node and the
standalone email app together.

The default URL is the wallet's email-only view (`?app=email`), so desktop and
Android use the same interface and local PQ keyfile login. The shared view
keeps the signed-in address and balance visible, but hides wallet transfers,
shielded send/receive, phone, king, and transaction-history controls. Mailbox,
domain, and email metadata purchases are accepted only when the displayed
address has the required TKM balance. The launcher always serves the client
through a loopback reverse proxy whose upstream dialer is Tor SOCKS5. If Tor is not listening, requests fail with
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

The client performs message encryption and approved EmailVM signing locally.
The email app rejects `tkmwallet://` handoffs and never launches the full TKM
Wallet. Use the full wallet application separately when you need transfers or
shielded note management.
