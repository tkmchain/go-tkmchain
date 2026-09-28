# TKM Email desktop app

`tkm-email` is the standalone desktop launcher for TKM EmailVM. It is kept
separate from `gtkm` and does not start a blockchain node, open a wallet
keystore, or handle recovery phrases.

```sh
make tkm-email
./build/bin/tkm-email
```

For a self-hosted or onion client:

```sh
./build/bin/tkm-email --url https://mail.tkmchain.site/ --no-open
```

The client performs message encryption locally. TKM spending and proof signing
remain in the wallet, so separating the app does not create a second place that
can access wallet secrets.
