package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/antartical"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

func TestOracleAndCrossChainEnvelopesRoundTrip(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.PubkeyToAddress(key.PublicKey)
	observation := antartical.OracleObservation{FeedID: common.HexToHash("0x01"), Round: 4, Value: []byte("42"), Timestamp: 99, Signer: signer}
	digest, err := observation.Hash(big.NewInt(8979))
	if err != nil {
		t.Fatal(err)
	}
	observation.Signature, err = crypto.Sign(digest.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeOracleObservation(&observation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeOracleObservation(encoded)
	if err != nil || decoded.Round != observation.Round || decoded.Signer != signer {
		t.Fatalf("oracle envelope round trip failed: %v", err)
	}
	message := &antartical.SignedCrossChainMessage{Message: antartical.CrossChainMessage{
		SourceChainID: big.NewInt(8979), DestinationChainID: big.NewInt(8980), Nonce: 7,
		Sender: signer, Target: common.HexToAddress("0x02"), Payload: []byte("payload"),
	}, Signer: signer}
	digest, err = message.Message.Hash()
	if err != nil {
		t.Fatal(err)
	}
	message.Signature, err = crypto.Sign(digest.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	xencoded, err := EncodeCrossChainMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	xdecoded, err := DecodeCrossChainMessage(xencoded)
	if err != nil || xdecoded.Message.Nonce != message.Message.Nonce || xdecoded.Signer != signer || xdecoded.Verify() != nil {
		t.Fatalf("cross-chain envelope round trip failed: %v", err)
	}
}

func TestOracleAndCrossChainStateReplay(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.PubkeyToAddress(key.PublicKey)
	config := *params.MainnetChainConfig
	config.AntarticalTime = new(uint64)
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	obs := antartical.OracleObservation{FeedID: common.HexToHash("0x10"), Round: 1, Value: []byte("42"), Timestamp: 1, Signer: signer}
	digest, _ := obs.Hash(config.ChainID)
	obs.Signature, _ = crypto.Sign(digest.Bytes(), key)
	data, _ := EncodeOracleObservation(&obs)
	tx := types.NewTx(&types.DynamicFeeTx{ChainID: config.ChainID, To: &params.ShieldedPoolAddress, Value: new(big.Int), Gas: 100000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Data: data})
	if err := ProcessOracleTransaction(&config, new(big.Int), 1, st, tx, signer); err != nil {
		t.Fatal(err)
	}
	if value, ok := OracleObservationValue(st, obs.FeedID, obs.Round); !ok || string(value) != "42" {
		t.Fatalf("oracle value was not persisted: %q %v", value, ok)
	}
	if err := ProcessOracleTransaction(&config, new(big.Int), 1, st, tx, signer); err == nil {
		t.Fatal("accepted a replayed oracle round")
	}

	message := &antartical.SignedCrossChainMessage{Message: antartical.CrossChainMessage{SourceChainID: big.NewInt(8980), DestinationChainID: config.ChainID, Nonce: 1, Sender: signer, Target: common.HexToAddress("0x20"), Payload: []byte("bridge")}, Signer: signer}
	digest, _ = message.Message.Hash()
	message.Signature, _ = crypto.Sign(digest.Bytes(), key)
	xdata, _ := EncodeCrossChainMessage(message)
	xtx := types.NewTx(&types.DynamicFeeTx{ChainID: config.ChainID, To: &params.ShieldedPoolAddress, Value: new(big.Int), Gas: 100000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Data: xdata})
	if err := ProcessCrossChainTransaction(&config, new(big.Int), 1, st, xtx, signer); err != nil {
		t.Fatal(err)
	}
	replay, _ := message.Message.ReplayKey()
	if payload, ok := CrossChainPayload(st, replay); !ok || string(payload) != "bridge" {
		t.Fatalf("cross-chain payload was not persisted: %q %v", payload, ok)
	}
	if err := ProcessCrossChainTransaction(&config, new(big.Int), 1, st, xtx, signer); err == nil {
		t.Fatal("accepted a replayed cross-chain message")
	}
}
