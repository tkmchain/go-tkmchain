package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type preparationRPC struct{ submissions [][]byte }

func (*preparationRPC) GetTransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	return nil, nil
}
func (r *preparationRPC) SendRawTransaction(_ context.Context, raw hexutil.Bytes) (common.Hash, error) {
	r.submissions = append(r.submissions, common.CopyBytes(raw))
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	return tx.Hash(), nil
}

func TestPreparationRestartRebroadcastsExactTransaction(t *testing.T) {
	server := rpc.NewServer()
	defer server.Stop()
	backend := new(preparationRPC)
	if err := server.RegisterName("eth", backend); err != nil {
		t.Fatal(err)
	}
	client := ethclient.NewClient(rpc.DialInProc(server))
	defer client.Close()
	data, err := core.EncodeShieldedV4Transaction(&core.ShieldedV4Transaction{Version: 4, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int)})
	if err != nil {
		t.Fatal(err)
	}
	tx := types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Value: new(big.Int), Data: data})
	raw, err := tx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "requests.json")
	id := &shield3wallet.Identity{ChainID: 8979, Address: common.HexToAddress("0x1234")}
	db := RequestDB{Requests: map[string]RequestRecord{}, NotePreparation: &NotePreparationRecord{ChainID: id.ChainID, Signer: id.Address, TxHash: tx.Hash().Hex(), SignedTransaction: hexutil.Encode(raw)}}
	if err := writeRequestDB(path, db); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		// Each attempt reads a fresh journal, as a restarted process does.
		loaded, err := readRequestDB(path)
		if err != nil {
			t.Fatal(err)
		}
		p := &Prover{client: client}
		p.cfg.RequestsPath = path
		err = p.preparePrivateNotes(context.Background(), nil, id, nil, 4, &loaded)
		var pending *notePreparationPending
		if !errors.As(err, &pending) || pending.hash != tx.Hash().Hex() {
			t.Fatalf("pending preparation lost: %v", err)
		}
		if len(loaded.Requests) != 0 {
			t.Fatal("preparation marked a payout sent")
		}
		other := *id
		other.ChainID++
		if err := p.preparePrivateNotes(context.Background(), nil, &other, nil, 4, &loaded); err == nil {
			t.Fatal("preparation from another chain accepted")
		}
	}
	if len(backend.submissions) != 2 || !bytes.Equal(raw, backend.submissions[0]) || !bytes.Equal(raw, backend.submissions[1]) {
		t.Fatal("retry created different transaction bytes")
	}
}

func TestAutomaticPrivateNotePreparation(t *testing.T) {
	tkm := func(v int64) *big.Int { return new(big.Int).Mul(big.NewInt(v), big.NewInt(params.Ether)) }
	notes := make([]shield3wallet.OwnedNote, 10)
	for i := range notes {
		notes[i].Commitment = shielded3.Digest{uint64(i + 1)}
		notes[i].ValueWei = tkm(100).String()
	}
	amount, err := privateNotePreparation(notes, tkm(1000))
	if err != nil || amount == nil || amount.Cmp(tkm(400)) != 0 {
		t.Fatalf("automatic first consolidation: %v %v", amount, err)
	}
	// Confirmation replaces the four inputs with one self-owned output.
	notes = append(notes[4:], shield3wallet.OwnedNote{Commitment: shielded3.Digest{11}, Note: shield3wallet.Note{ValueWei: tkm(400).String()}})
	amount, err = privateNotePreparation(notes, tkm(1000))
	if err != nil || amount == nil || amount.Cmp(tkm(700)) != 0 {
		t.Fatalf("second consolidation: %v %v", amount, err)
	}
	if amount, err = privateNotePreparation(notes, tkm(500)); err != nil || amount != nil {
		t.Fatalf("unnecessary consolidation: %v %v", amount, err)
	}
	if _, err := privateNotePreparation(notes, tkm(1001)); err == nil {
		t.Fatal("unbacked notes permitted")
	}
	if _, err := privateNotePreparation(append(notes, notes[0]), tkm(1000)); err == nil {
		t.Fatal("duplicate note counted twice")
	}
	if _, err := privateNotePreparation(notes, tkm(5000001)); err == nil {
		t.Fatal("send maximum bypassed")
	}
}

func TestAutomaticPublicFundingBudget(t *testing.T) {
	for _, cfg := range []Config{{AutoPublicFunding: true}, {AutoPublicFunding: true, AutoPublicFundingLimitWei: "-1"}, {PrepaidFeeLimitWei: "bad"}} {
		if err := validateAutomaticFundingConfig(cfg); err == nil {
			t.Fatal("invalid automatic funding config accepted")
		}
	}
	if err := validateAutomaticFundingConfig(Config{AutoPublicFunding: true, AutoPublicFundingLimitWei: "1000", PrepaidFeeLimitWei: "10"}); err != nil {
		t.Fatal(err)
	}
	notes := []shield3wallet.OwnedNote{{Note: shield3wallet.Note{ValueWei: "400"}}}
	amount, err := publicFundingAmount(notes, big.NewInt(1000), "600")
	if err != nil || amount.Int64() != 600 {
		t.Fatalf("expected only missing principal: %v %v", amount, err)
	}
	for _, limit := range []string{"", "0", "-1", "599", "bad"} {
		if _, err := publicFundingAmount(notes, big.NewInt(1000), limit); err == nil {
			t.Fatal("unauthorized public deposit")
		}
	}
	if _, err := publicFundingAmount(notes, big.NewInt(300), "600"); err == nil {
		t.Fatal("unnecessary public deposit")
	}
	if _, err := publicFundingAmount(notes, nil, "600"); err == nil {
		t.Fatal("missing target accepted")
	}
	if (Config{}).AutoPublicFunding {
		t.Fatal("public funding must be explicitly enabled")
	}
}

func TestPreparationJournalIsNotPayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.json")
	db := RequestDB{Requests: map[string]RequestRecord{}, NotePreparation: &NotePreparationRecord{TxHash: "0x1234", SignedTransaction: "0x5678"}}
	if err := writeRequestDB(path, db); err != nil {
		t.Fatal(err)
	}
	loaded, err := readRequestDB(path)
	if err != nil || loaded.NotePreparation == nil || *loaded.NotePreparation != *db.NotePreparation || len(loaded.Requests) != 0 {
		t.Fatalf("preparation journal lost: %v", err)
	}
	response, err := json.Marshal(PayoutResponse{Status: "preparing-notes", PreparationTxHash: db.NotePreparation.TxHash})
	if err != nil || strings.Contains(string(response), `"txHash"`) {
		t.Fatalf("preparation misreported as payout: %s %v", response, err)
	}
}

func TestPreparationRefusesPublicFunding(t *testing.T) {
	for _, version := range []int{3, 4} {
		for _, public := range []bool{false, true} {
			var data []byte
			var err error
			if version == 4 {
				data, err = core.EncodeShieldedV4Transaction(&core.ShieldedV4Transaction{Version: 4, Deposit: public, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int)})
			} else {
				data, err = core.EncodeShieldedV3Transaction(&core.ShieldedV3Transaction{Version: 3, Deposit: public, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int)})
			}
			if err != nil {
				t.Fatal(err)
			}
			tx := types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Value: new(big.Int), Data: data})
			if err := validatePrivatePreparation(tx); (err != nil) != public {
				t.Fatalf("version %d deposit %v: %v", version, public, err)
			}
		}
	}
}
