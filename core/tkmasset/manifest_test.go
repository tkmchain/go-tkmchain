package tkmasset

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func testManifest() Manifest {
	return Manifest{
		Kind:        KindFungible,
		ChainID:     big.NewInt(8979),
		Decimals:    18,
		Flags:       FlagMintable | FlagBurnable | FlagPermit,
		PolicyHash:  common.HexToHash("0x1234"),
		Name:        "TKM Dollar",
		Symbol:      "TKMD",
		MetadataURI: "ipfs://tkm/asset.json",
	}
}

func TestManifestTrailerRoundTripAndAssetID(t *testing.T) {
	m := testManifest()
	canonical, err := m.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	code, err := AppendTrailer([]byte{0x60, 0x00, 0x56}, m)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := ParseRuntimeCode(code)
	if err != nil || !found {
		t.Fatalf("parse trailer: found=%v err=%v", found, err)
	}
	gotCanonical, err := got.MarshalBinary()
	if err != nil || !bytes.Equal(gotCanonical, canonical) {
		t.Fatalf("manifest round trip mismatch: %x/%x err=%v", gotCanonical, canonical, err)
	}
	hash, err := m.ManifestHash()
	if err != nil {
		t.Fatal(err)
	}
	contract := common.HexToAddress("0x1234567890123456789012345678901234567890")
	assetID, err := AssetID(m.ChainID, contract, m.Kind, hash)
	if err != nil {
		t.Fatal(err)
	}
	input, err := PrecompileInput(m.ChainID, contract, m.Kind, hash)
	if err != nil {
		t.Fatal(err)
	}
	precompileID, err := AssetIDFromPrecompileInput(input)
	if err != nil || precompileID != assetID {
		t.Fatalf("precompile identity = %s, want %s (err=%v)", precompileID, assetID, err)
	}
}

func TestParseRuntimeCodeDistinguishesEthereumContract(t *testing.T) {
	if _, found, err := ParseRuntimeCode([]byte{0x60, 0x00, 0x56}); err != nil || found {
		t.Fatalf("plain Ethereum code classified as TKM asset: found=%v err=%v", found, err)
	}
}

func TestManifestRejectsInvalidKindAndNFTDecimals(t *testing.T) {
	bad := testManifest()
	bad.Kind = Kind(99)
	if _, err := bad.MarshalBinary(); err == nil {
		t.Fatal("invalid kind accepted")
	}
	bad = testManifest()
	bad.Kind = KindNonFungible
	bad.Decimals = 18
	if _, err := bad.MarshalBinary(); err == nil {
		t.Fatal("NFT decimals accepted")
	}
}
