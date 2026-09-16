package core

import (
	"context"
	"encoding/binary"
	"errors"
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestShield3LegacyMigrationRequiresEmptyChange(t *testing.T) {
	key, err := pqcrypto.NewMLDSA87FromSeed(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sender, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(key))
	if err != nil {
		t.Fatal(err)
	}
	envelope := testShieldedEnvelope(t, 1)
	envelope.Version = ShieldedTxVersionV2
	envelope.WithdrawalRecipient = sender
	envelope.WithdrawalValue = big.NewInt(7)
	envelope.MigrationOutputRandomness = make([]common.Hash, 4)
	for i := range envelope.Outputs {
		random := common.BigToHash(big.NewInt(int64(i + 10)))
		envelope.MigrationOutputRandomness[i] = random
		owner := fr.Element{}
		if i == 0 {
			owner.SetBytes(sender.Bytes())
		}
		envelope.Outputs[i].Commitment = hashFromShieldedField(shieldedFieldHash(2001, owner, fieldElementFromUint64(1), fr.Element{}, fieldElementFromHash(random)))
	}
	tx := func() *types.Transaction {
		t.Helper()
		data, err := EncodeShieldedTransaction(envelope)
		if err != nil {
			t.Fatal(err)
		}
		signed, err := types.SignNewPQTkmTx(key, types.NewQuantumSigner(big.NewInt(8979)), &types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Gas: 100000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Value: new(big.Int), Data: data})
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, tx()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime-1, tx()); err == nil {
		t.Fatal("migration openings accepted before fork")
	}
	envelope.Outputs[1].Commitment = common.HexToHash("0x123")
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, tx()); err == nil {
		t.Fatal("migration allowed private change")
	}
}
func TestShield3NeverFallsBackToLegacyVerifier(t *testing.T) {
	SetShieldedProofVerifier(testShieldedVerifier{})
	defer SetShieldedProofVerifier(nil)
	private, public, err := pqcrypto.GenerateShieldedV3ViewKey()
	defer clear(private)
	if err != nil {
		t.Fatal(err)
	}
	envelope := &ShieldedV3Transaction{Version: 3, Deposit: true, StampRoot: shielded3.Digest{99}, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int), Proof: binary.LittleEndian.AppendUint64(binary.LittleEndian.AppendUint32(nil, 1), 1)}
	// A canonical-looking proof is insufficient even when a legacy verifier
	// has been configured to accept every legacy statement.
	for i := range envelope.Outputs {
		out := &envelope.Outputs[i]
		out.Commitment = shielded3.Digest{uint64(i + 1)}
		for _, record := range []struct {
			role pqcrypto.ShieldedV3Purpose
			data *[]byte
		}{{pqcrypto.ShieldedV3Incoming, &out.Incoming}, {pqcrypto.ShieldedV3Outgoing, &out.Outgoing}, {pqcrypto.ShieldedV3Stamp, &out.Stamp}} {
			*record.data, err = pqcrypto.SealShieldedV3(public, nil, ShieldedV3OutputContext(8979, record.role, out.Commitment))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	data, err := EncodeShieldedV3Transaction(envelope)
	if err != nil {
		t.Fatal(err)
	}
	tx := types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Gas: 7000000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Value: big.NewInt(1), Data: data})
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	tx, err = types.SignPQTkmTx(tx, types.NewQuantumSigner(big.NewInt(8979)), key)
	if err != nil {
		t.Fatal(err)
	}
	from, err := types.Sender(types.NewQuantumSigner(big.NewInt(8979)), tx)
	if err != nil {
		t.Fatal(err)
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", from.Bytes()), common.HexToHash("0x1"))
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/root", envelope.StampRoot.Bytes()), common.HexToHash("0x1"))
	err = ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, st, tx, make(map[common.Hash]struct{}))
	if !errors.Is(err, ErrInvalidShieldedTx) || ShieldedV3NextIndex(st) != 0 {
		t.Fatalf("invalid native proof accepted or modified state: %v", err)
	}
	if !shielded3.NativeAvailable() && !errors.Is(err, shielded3.ErrBackendUnavailable) {
		t.Fatalf("missing native verifier did not fail closed: %v", err)
	}
	if err := (shielded3.NativeBackend{}).Verify(context.Background(), shielded3.Statement{}, nil); err == nil {
		t.Fatal("empty statement accepted")
	}
}
