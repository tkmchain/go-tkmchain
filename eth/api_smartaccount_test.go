package eth

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

func smartTestBig(n int64) *hexutil.Big { value := hexutil.Big(*big.NewInt(n)); return &value }

func TestSmartAccountStatusIsNonConsensus(t *testing.T) {
	status := NewSmartAccountAPI().Status()
	if status.Consensus || status.RequiresFork || status.Deployed {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.AuthModes["ownerSignatures"] != 2 || status.AuthModes["sessionKey"] != 1 {
		t.Fatalf("authorization modes missing: %#v", status.AuthModes)
	}
}

func TestSmartAccountOperationHashCommitsToAllFields(t *testing.T) {
	api := NewSmartAccountAPI()
	op := SmartUserOperation{Account: common.HexToAddress("0x1111111111111111111111111111111111111111"), Target: common.HexToAddress("0x2222222222222222222222222222222222222222"), Value: smartTestBig(25), Data: common.FromHex("0x12345678aabb"), Nonce: smartTestBig(7), ValidUntil: 999, GasLimit: smartTestBig(100000), Paymaster: common.HexToAddress("0x3333333333333333333333333333333333333333"), PaymasterData: common.FromHex("0xaabb")}
	entry := common.HexToAddress("0x4444444444444444444444444444444444444444")
	hash, err := api.OperationHash(op, entry, smartTestBig(8979))
	if err != nil {
		t.Fatal(err)
	}
	if hash == (common.Hash{}) {
		t.Fatal("empty hash")
	}
	op.Data[0] ^= 1
	changed, err := api.OperationHash(op, entry, smartTestBig(8979))
	if err != nil {
		t.Fatal(err)
	}
	if changed == hash {
		t.Fatal("operation hash does not commit to calldata")
	}
	otherChain, _ := api.OperationHash(op, entry, smartTestBig(8980))
	if otherChain == changed {
		t.Fatal("operation hash is replayable across chains")
	}
}

func TestSmartAccountBuildersAndValidation(t *testing.T) {
	api := NewSmartAccountAPI()
	owner := common.HexToAddress("0x1111111111111111111111111111111111111111")
	data, err := api.CreateData(SmartAccountCreateRequest{Owners: []common.Address{owner}, Threshold: 1, Salt: common.HexToHash("0x01")})
	if err != nil {
		t.Fatal(err)
	}
	want := crypto.Keccak256([]byte("createAccount(address[],uint16,bytes32)"))[:4]
	if !bytes.HasPrefix(data, want) {
		t.Fatalf("wrong create selector: %x", data[:4])
	}
	if _, err := api.CreateData(SmartAccountCreateRequest{Owners: []common.Address{owner, owner}, Threshold: 2}); err == nil {
		t.Fatal("duplicate owners accepted")
	}
	if _, err := api.SetRecoveryPolicyData(1, 3599); err == nil {
		t.Fatal("unsafe recovery delay accepted")
	}
	recoveryHash, err := api.RecoveryHash([]common.Address{owner}, 1)
	if err != nil || recoveryHash == (common.Hash{}) {
		t.Fatalf("recovery hash: %s %v", recoveryHash, err)
	}
}

func TestSmartAccountSessionBuilder(t *testing.T) {
	api := NewSmartAccountAPI()
	data, err := api.SetSessionData(SmartAccountSessionRequest{Key: common.HexToAddress("0x1111111111111111111111111111111111111111"), Target: common.HexToAddress("0x2222222222222222222222222222222222222222"), Selector: common.FromHex("0xa9059cbb"), MaxValuePerCall: smartTestBig(100), RemainingValue: smartTestBig(1000), ValidAfter: 10, ValidUntil: 100, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) <= 4 {
		t.Fatal("missing encoded session")
	}
	if _, err := api.SetSessionData(SmartAccountSessionRequest{Selector: common.FromHex("0x12")}); err == nil {
		t.Fatal("invalid session accepted")
	}
}

func TestSmartAccountPredictCreate2(t *testing.T) {
	api := NewSmartAccountAPI()
	factory := common.HexToAddress("0x1111111111111111111111111111111111111111")
	salt := common.HexToHash("0x02")
	codeHash := common.HexToHash("0x03")
	have, err := api.PredictAddress(factory, salt, codeHash)
	if err != nil {
		t.Fatal(err)
	}
	want := common.BytesToAddress(crypto.Keccak256([]byte{0xff}, factory.Bytes(), salt.Bytes(), codeHash.Bytes())[12:])
	if have != want {
		t.Fatalf("prediction mismatch: %s != %s", have, want)
	}
}
