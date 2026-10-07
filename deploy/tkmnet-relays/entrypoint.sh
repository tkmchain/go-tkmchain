#!/bin/sh
set -eu

TOR_STATE=/var/lib/tor
HS_DIR=${TKMNET_TOR_HS_DIR:-/var/lib/tor/hidden}
mkdir -p "$TOR_STATE/data" "$HS_DIR" /var/lib/tkmnet /etc/tkmnet
chown -R debian-tor:debian-tor "$TOR_STATE"
chown -R tkmrelay:tkmrelay /var/lib/tkmnet
chmod 0700 "$HS_DIR" "$TOR_STATE/data" /var/lib/tkmnet

PEERS_SOURCE=${TKMNET_PEERS_SOURCE:-/etc/tkmnet/peers.json}
if [ -r "$PEERS_SOURCE" ]; then
  cp "$PEERS_SOURCE" /var/lib/tkmnet/peers.json
else
  printf '[]\n' >/var/lib/tkmnet/peers.json
fi
chown tkmrelay:tkmrelay /var/lib/tkmnet/peers.json
chmod 0600 /var/lib/tkmnet/peers.json

cat >/run/torrc <<TORRC
User debian-tor
DataDirectory $TOR_STATE/data
SocksPort 127.0.0.1:9050 IsolateSOCKSAuth
SafeSocks 1
HiddenServiceDir $HS_DIR
HiddenServiceVersion 3
HiddenServicePort 39000 127.0.0.1:39000
Log notice stdout
TORRC

tor -f /run/torrc &
TOR_PID=$!
RELAY_PID=
cleanup() {
  if [ -n "$RELAY_PID" ]; then kill -TERM "$RELAY_PID" 2>/dev/null || true; fi
  kill -TERM "$TOR_PID" 2>/dev/null || true
  wait "$RELAY_PID" 2>/dev/null || true
  wait "$TOR_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

attempt=0
while [ ! -s "$HS_DIR/hostname" ]; do
  attempt=$((attempt + 1))
  if [ "$attempt" -gt 120 ] || ! kill -0 "$TOR_PID" 2>/dev/null; then
    echo "Tor did not create its onion hostname" >&2
    exit 1
  fi
  sleep 1
done
chmod 0644 "$HS_DIR/hostname"
cp "$HS_DIR/hostname" /var/lib/tkmnet/onion.hostname
chown tkmrelay:tkmrelay /var/lib/tkmnet/onion.hostname
chmod 0644 /var/lib/tkmnet/onion.hostname

# Keep every public TKMNet listener on loopback; Tor is the only route in.
gosu tkmrelay /usr/local/bin/tkmnet-relay &
RELAY_PID=$!
wait "$RELAY_PID"
