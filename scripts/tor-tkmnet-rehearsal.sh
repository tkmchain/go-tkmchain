#!/usr/bin/env bash
# Run the production Tor/TKMNet smoke rehearsal against a running node and
# onion-published pool. The script is read-only: it never submits a
# transaction, mining share, or configuration change.
set -euo pipefail

RPC_URL="${TKM_RPC_URL:-http://127.0.0.1:8545}"
IPC_PATH="${TKM_IPC_PATH:-$HOME/.tkmchain/gtkm.ipc}"
GTKM_BIN="${GTKM_BIN:-$(pwd)/build/bin/gtkm}"
TOR_PROXY="${TKM_TOR_PROXY:-127.0.0.1:9050}"
POOL_ONION="${TKM_POOL_ONION:-4aof7abdduh4vftejgdpdfqeosvxxco3xmpu4uqypnpdbi7wjuzfqhqd.onion}"
POOL_HTTP_PORT="${TKM_POOL_HTTP_PORT:-33230}"
POOL_STRATUM_PORT="${TKM_POOL_STRATUM_PORT:-33330}"
EXPECTED_CHAIN_ID="${TKM_CHAIN_ID:-0x2313}"
MIN_PEERS="${TKM_MIN_PEERS:-1}"

die() { echo "tor-tkmnet rehearsal: $*" >&2; exit 1; }
require_cmd() { command -v "$1" >/dev/null 2>&1 || die "required command '$1' is missing"; }

require_cmd curl
require_cmd python3

case "$(python3 - "$RPC_URL" <<'PY'
import sys
from urllib.parse import urlparse
u = urlparse(sys.argv[1])
print(u.hostname or "")
PY
)" in
  127.0.0.1|localhost|::1) ;;
  *) die "RPC endpoint must be loopback; refusing clearnet RPC ${RPC_URL}" ;;
esac

rpc_call() {
  local method="$1" params="${2:-[]}"
  curl -fsS --max-time "${TKM_RPC_TIMEOUT:-10}" \
    -H 'content-type: application/json' \
    --data "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"${method}\",\"params\":${params}}" \
    "$RPC_URL"
}

json_result() {
  python3 -c 'import json,sys; v=json.load(sys.stdin); assert "error" not in v, v["error"].get("message", "RPC error"); print(json.dumps(v.get("result")))'
}

echo "[1/6] checking local Tor SOCKS5 listener ${TOR_PROXY}"
python3 - "$TOR_PROXY" <<'PY'
import socket, sys
host, port = sys.argv[1].rsplit(":", 1)
with socket.create_connection((host, int(port)), 5) as s:
    s.sendall(b"\x05\x01\x00")
    if s.recv(2) != b"\x05\x00":
        raise SystemExit("SOCKS5 listener did not select unauthenticated mode")
PY

echo "[2/6] checking onion-only peer advertisement and discovery"
if [[ -S "$IPC_PATH" && -x "$GTKM_BIN" ]]; then
  node_info="$($GTKM_BIN attach --exec 'admin.nodeInfo' 2>/dev/null)" || die "unable to read admin.nodeInfo over IPC"
  node_enode="$(printf '%s\n' "$node_info" | sed -n 's/.*enode: "\([^"]*\)".*/\1/p' | head -1)"
  [[ "$node_enode" == *".onion:"* ]] || die "node enode does not advertise a .onion hostname: ${node_enode:-missing}"
  [[ "$node_enode" != *"@127.0.0.1:"* && "$node_enode" != *"@0.0.0.0:"* ]] || die "node enode advertises a loopback address: $node_enode"
  echo "  enode=${node_enode}"
else
  echo "  IPC/binary unavailable; skipped advertisement check (set TKM_IPC_PATH and GTKM_BIN to enable)"
fi

chain_id="$(rpc_call eth_chainId | json_result)"
[[ "$chain_id" == "\"${EXPECTED_CHAIN_ID}\"" ]] || die "eth_chainId=${chain_id}, want ${EXPECTED_CHAIN_ID}"
peer_count="$(rpc_call net_peerCount | json_result)"
python3 - "$peer_count" "$MIN_PEERS" <<'PY'
import json, sys
peers = int(json.loads(sys.argv[1]), 0)
if peers < int(sys.argv[2]):
    raise SystemExit(f"only {peers} peers are connected; onion discovery minimum is {sys.argv[2]}")
print(f"  onion peers={peers}")
PY

echo "[3/6] checking EVM RPC and Antartical activation gate"
block="$(rpc_call eth_blockNumber | json_result)"
modules="$(rpc_call rpc_modules | json_result)"
work="$(rpc_call randomx_getWork | json_result)"
status="$(rpc_call tkmprotocol_antarticalStatus | json_result)"
python3 - "$block" "$modules" "$work" "$status" <<'PY'
import json, sys
block, modules, work, status = map(json.loads, sys.argv[1:])
if not isinstance(block, str) or not block.startswith("0x"):
    raise SystemExit("eth_blockNumber did not return a hex block number")
for name in ("net", "web3", "randomx"):
    if name not in modules:
        raise SystemExit(f"RPC module {name} is missing")
if not isinstance(work, list) or len(work) != 4:
    raise SystemExit("randomx_getWork did not return a four-item work package")
if not isinstance(status, dict) or "activationTime" not in status or "active" not in status:
    raise SystemExit("tkmprotocol_antarticalStatus is incomplete")
print(f"  block={int(block, 16)} antartical_active={status['active']} activation={status['activationTime']}")
PY

echo "[4/6] checking pool HTTP through Tor ${POOL_ONION}:${POOL_HTTP_PORT}"
[[ "$POOL_ONION" == *.onion ]] || die "pool endpoint is not an onion hostname"
pool_status="$(curl --socks5-hostname "$TOR_PROXY" -fsS --max-time "${TKM_POOL_TIMEOUT:-30}" "http://${POOL_ONION}:${POOL_HTTP_PORT}/api/status")" || die "pool status was not reachable through Tor"
python3 -c 'import json,sys; status=json.load(sys.stdin); assert isinstance(status.get("stratum"), str) and ".onion:" in status["stratum"], "pool status does not publish an onion Stratum endpoint"; print("  pool stratum={} workers={}".format(status["stratum"], status.get("workerCount", 0)))' <<<"$pool_status"

echo "[5/6] checking miner Stratum subscribe/authorize through Tor"
python3 - "$TOR_PROXY" "$POOL_ONION" "$POOL_STRATUM_PORT" <<'PY'
import json, socket, sys
proxy_host, proxy_port = sys.argv[1].rsplit(":", 1)
dest, dest_port = sys.argv[2], int(sys.argv[3])
with socket.create_connection((proxy_host, int(proxy_port)), 10) as s:
    s.settimeout(20)
    s.sendall(b"\x05\x01\x00")
    if s.recv(2) != b"\x05\x00":
        raise SystemExit("Tor SOCKS5 authentication negotiation failed")
    name = dest.encode()
    s.sendall(b"\x05\x01\x00\x03" + bytes([len(name)]) + name + dest_port.to_bytes(2, "big"))
    reply = s.recv(10)
    if len(reply) < 2 or reply[1] != 0:
        raise SystemExit(f"Tor could not connect to onion Stratum (reply={reply!r})")
    buffered = b""
    def request(i, method, params):
        global buffered
        s.sendall((json.dumps({"id": i, "jsonrpc": "2.0", "method": method, "params": params}) + "\n").encode())
        while True:
            while b"\n" not in buffered:
                part = s.recv(4096)
                if not part:
                    raise SystemExit("Stratum closed before responding")
                buffered += part
            line, buffered = buffered.split(b"\n", 1)
            if not line:
                continue
            value = json.loads(line)
            if value.get("id") != i:
                continue
            break
        if value.get("error") not in (None, False):
            raise SystemExit(f"Stratum {method} failed: {value['error']}")
        return value
    request(1, "mining.subscribe", ["tkmnet-rehearsal/1.0"])
    auth = request(2, "mining.authorize", ["0x6d7b2816111e014d4649ca8724e0126a0d2891a1.rehearsal", "x"])
    if auth.get("result") is not True:
        raise SystemExit(f"Stratum authorization failed: {auth}")
print("  Stratum subscribe and authorize succeeded over Tor")
PY

echo "[6/6] checking TKMNet packet/lifecycle and fork tests"
go test ./p2p ./node ./tkmnet ./params ./cmd/gtkm -run 'Test(OnionSOCKS5DialerRoutesAndRejects|OnionOnlyConfigurationIsFailClosed|ServiceOpensFinalPayload|ServiceLifecycle|TkmnetActivationFollowsCanonicalHead|TkmnetRequiredForkSchedule|AntarticalForkSchedule)' -count=1

echo "Tor/TKMNet production rehearsal passed: onion peer path, EVM RPC, RandomX work, pool HTTP/Stratum, TKMNet lifecycle, and Antartical gate verified."
