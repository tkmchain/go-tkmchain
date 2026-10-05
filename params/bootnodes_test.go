package params

import (
	"strings"
	"testing"
)

func TestMainnetBootnodesUseCurrentOnionEnodes(t *testing.T) {
	if len(MainnetBootnodes) < 2 {
		t.Fatalf("MainnetBootnodes has %d entries, want at least two", len(MainnetBootnodes))
	}
	seen := make(map[string]bool, len(MainnetBootnodes))
	for _, raw := range MainnetBootnodes {
		if !strings.HasPrefix(raw, "enode://") || !strings.Contains(raw, ".onion:3000?discport=0") {
			t.Errorf("bootnode is not an onion-only TCP enode: %q", raw)
		}
		if seen[raw] {
			t.Errorf("duplicate bootnode: %q", raw)
		}
		seen[raw] = true
	}
	const server82 = "enode://551bdaafa74ab8db0e7d60225bafa30d48833ccea03f9718c11d960aa815ac1de17fad9c26a1618e490283954fabe3252b4120957f64c85a13a6dea34b62fe3b@oll36d63j2ujkcjjpwm7bitpfs6zojcjk5odrj7se3eqrhwsbb6gknid.onion:3000?discport=0"
	if !seen[server82] {
		t.Fatal("current server 82 onion enode is missing from MainnetBootnodes")
	}
	for _, raw := range MainnetBootnodes {
		if strings.Contains(raw, "eaoerarabizbzwbbawjrlcyawnrnoobj3ndy3oh627hwl5rbmedukoqd.onion") {
			t.Fatal("stale server 82 onion hostname remains in MainnetBootnodes")
		}
	}
}
