package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestSmartAccountAuthorizationBuilders(t *testing.T) {
	api := NewSmartAccountAPI()
	signature := make(hexutil.Bytes, 65)
	signature[64] = 1
	owners, err := api.OwnerAuthorization([]hexutil.Bytes{signature})
	if err != nil {
		t.Fatal(err)
	}
	if owners[0] != 2 || len(owners) <= 66 {
		t.Fatalf("invalid owner authorization: %x", owners)
	}
	session, err := api.SessionAuthorization(signature)
	if err != nil {
		t.Fatal(err)
	}
	if session[0] != 1 || len(session) != 66 {
		t.Fatalf("invalid session authorization: %x", session)
	}
	if _, err := api.OwnerAuthorization([]hexutil.Bytes{{1}}); err == nil {
		t.Fatal("short signature accepted")
	}
}

func TestSmartAccountSponsorshipBuilders(t *testing.T) {
	api := NewSmartAccountAPI()
	operation := common.HexToHash("0x01")
	paymaster := common.HexToAddress("0x1111111111111111111111111111111111111111")
	hash, err := api.SponsorshipHash(operation, 1000, paymaster, smartTestBig(8979))
	if err != nil {
		t.Fatal(err)
	}
	if hash == (common.Hash{}) {
		t.Fatal("empty sponsorship hash")
	}
	other, _ := api.SponsorshipHash(operation, 1000, paymaster, smartTestBig(8980))
	if hash == other {
		t.Fatal("sponsorship hash is replayable across chains")
	}
	signature := make(hexutil.Bytes, 65)
	data, err := api.SponsorshipData(1000, signature)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) <= 65 {
		t.Fatal("invalid sponsorship encoding")
	}
}
