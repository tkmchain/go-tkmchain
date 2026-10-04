package shield3wallet

import (
	"bytes"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func sponsorshipCodeFixture(t *testing.T) *types.Transaction {
	t.Helper()
	sponsor, err := pqcrypto.NewMLDSA87FromSeed(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	seed := bytes.Repeat([]byte{2}, 32)
	beneficiary, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	stamp, err := pqcrypto.CreateShieldedV3Stamp(seed, 8979, "Hidden Name", "Hidden Country")
	if err != nil {
		t.Fatal(err)
	}
	data, err := core.EncodeAntarticalStamp(&core.AntarticalStampRegistration{
		Version: 1, Owner: shielded3.Digest{1, 2, 3, 4, 5}, Stamp: *stamp,
		BeneficiaryPublicKey: pqcrypto.PublicKeyBytes(beneficiary), ValidUntil: params.MainnetAntarticalTime + 3600,
	})
	if err != nil {
		t.Fatal(err)
	}
	return types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), Nonce: 12, Gas: StampWalletGas,
		GasFeeCap: big.NewInt(100), GasTipCap: big.NewInt(10), To: &params.ShieldedPoolAddress, Value: new(big.Int),
		Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(sponsor), Data: data})
}

func TestStampSponsorshipCodeRoundTrip(t *testing.T) {
	tx := sponsorshipCodeFixture(t)
	want, err := stampSponsorshipPacket(tx)
	if err != nil {
		t.Fatal(err)
	}
	// The code must never trust caller-supplied display metadata.
	packet := want
	packet.Sponsor = common.Address{}
	packet.Beneficiary = common.Address{}
	packet.MaxFeeWei = "0"
	packet.ValidUntil = 0
	code, err := EncodeStampSponsorshipCode(packet)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeStampSponsorshipCode(code, 8979)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Transaction, want.Transaction) || got.Sponsor != want.Sponsor || got.Beneficiary != want.Beneficiary || got.MaxFeeWei != "400000000" || got.ValidUntil != want.ValidUntil {
		t.Fatal("code changed transaction or trusted forged display fields")
	}
	if _, err := DecodeStampSponsorshipCode(code, 8980); err == nil {
		t.Fatal("cross-chain code accepted")
	}
	for _, bad := range []string{"", "tkmstamp2." + code, code + "=", code + "\n", StampSponsorshipCodePrefix + strings.Repeat("A", MaxStampSponsorshipCodeSize)} {
		if _, err := DecodeStampSponsorshipCode(bad, 8979); err == nil {
			t.Fatal("malformed code accepted")
		}
	}
}

func TestStampSponsorshipCodeRejectsPaymentAndWrongPool(t *testing.T) {
	tx := sponsorshipCodeFixture(t)
	algorithm, pub, _, _ := tx.PQTkmFields()
	for _, change := range []string{"value", "pool", "signature"} {
		t.Run(change, func(t *testing.T) {
			pool := params.ShieldedPoolAddress
			draft := &types.PQTkmTx{ChainID: tx.ChainId(), Nonce: tx.Nonce(), Gas: tx.Gas(), GasFeeCap: tx.GasFeeCap(), GasTipCap: tx.GasTipCap(), To: &pool, Value: new(big.Int), Algorithm: algorithm, PublicKey: pub, Data: tx.Data()}
			if change == "value" {
				draft.Value = big.NewInt(1)
			}
			if change == "pool" {
				pool = common.HexToAddress("0x1234")
			}
			bad := types.NewTx(draft)
			if change == "signature" {
				key, err := pqcrypto.NewMLDSA87FromSeed(bytes.Repeat([]byte{1}, 32))
				if err != nil {
					t.Fatal(err)
				}
				bad, err = types.SignPQTkmTx(bad, types.NewQuantumSigner(big.NewInt(8979)), key)
				if err != nil {
					t.Fatal(err)
				}
			}
			raw, err := bad.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeStampSponsorshipCode(StampSponsorshipCodePrefix+base64.RawURLEncoding.EncodeToString(raw), 8979); err == nil {
				t.Fatal("non-offer transaction accepted")
			}
		})
	}
}
