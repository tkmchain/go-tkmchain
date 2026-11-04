package shield3wallet

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

const (
	shieldedV6WalletPage   = uint64(64)
	shieldedV6KeyImagePage = uint64(256)
)

type shieldedV6RPCStatus struct {
	Active             bool           `json:"active"`
	ChainID            *hexutil.Big   `json:"chainId"`
	OutputCount        hexutil.Uint64 `json:"outputCount"`
	SpentKeyImageCount hexutil.Uint64 `json:"spentKeyImageCount"`
	Accumulator        common.Hash    `json:"accumulatorRoot"`
}

type shieldedV6RPCOutput struct {
	AccumulatorIndex hexutil.Uint64   `json:"accumulatorIndex"`
	OutputPosition   hexutil.Uint64   `json:"outputPosition"`
	BlockNumber      hexutil.Uint64   `json:"blockNumber"`
	BlockHash        common.Hash      `json:"blockHash"`
	TransactionHash  common.Hash      `json:"transactionHash"`
	OneTimeKey       hexutil.Bytes    `json:"oneTimeKey"`
	EphemeralKey     hexutil.Bytes    `json:"ephemeralKey"`
	Commitment       hexutil.Bytes    `json:"commitment"`
	AmountLimbs      [2]hexutil.Bytes `json:"amountLimbs"`
	EncryptedNote    hexutil.Bytes    `json:"encryptedNote"`
}

type shieldedV6RingRPCOutput struct {
	Index      hexutil.Uint64 `json:"index"`
	OneTimeKey hexutil.Bytes  `json:"oneTimeKey"`
	Commitment hexutil.Bytes  `json:"commitment"`
}

type shieldedV6SpendCandidate struct {
	note            *shielded6.Note
	transactionHash common.Hash
	blockNumber     uint64
}

// ShieldedV6OwnedNote binds a locally decrypted note to its canonical origin.
// The Note must be cleared by the caller after use.
type ShieldedV6OwnedNote struct {
	Note            *shielded6.Note
	TransactionHash common.Hash
	BlockNumber     uint64
}

type ShieldedV6ScanResult struct {
	BalanceWei      *big.Int
	OutputCount     uint64
	AccumulatorRoot common.Hash
	HeadHash        common.Hash
	HeadNumber      uint64
	Notes           []ShieldedV6OwnedNote
}

// ShieldedV6Ring is a locally selected canonical CLSAG ring. SignerIndex is
// never sent to the RPC endpoint; all member lookups are made in randomized
// order through one bounded batch request per ring.
type ShieldedV6Ring struct {
	Members     [shielded6.RingSize]shielded6.RingMember
	Indices     [shielded6.RingSize]uint64
	SignerIndex uint64
	Root        common.Hash
}

// SelectShieldedV6Ring samples decoys uniformly without replacement from the
// active canonical accumulator. The real index is mixed into a locally
// randomized request order so the RPC server receives no explicit real/decoy
// marker. The root and output count are checked again before returning.
func SelectShieldedV6Ring(ctx context.Context, rpc RPC, chainID, realIndex uint64, random io.Reader) (ShieldedV6Ring, error) {
	var result ShieldedV6Ring
	if rpc == nil || chainID == 0 {
		return result, errors.New("Shield6 ring selection requires RPC and chain ID")
	}
	if random == nil {
		random = cryptorand.Reader
	}
	var before shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &before, "tkmprivacy_shieldedV6Status"); err != nil {
		return result, err
	}
	if !before.Active || before.ChainID == nil || (*big.Int)(before.ChainID).Cmp(new(big.Int).SetUint64(chainID)) != 0 {
		return result, errors.New("Shield6 is not active on the requested chain")
	}
	count := uint64(before.OutputCount)
	if count < shielded6.RingSize || realIndex >= count {
		return result, errors.New("Shield6 accumulator does not contain enough outputs for a ring")
	}
	selectedPositions := make(map[uint64]struct{}, shielded6.RingSize-1)
	population := count - 1
	for j := population - (shielded6.RingSize - 1); j < population; j++ {
		bound := new(big.Int).SetUint64(j + 1)
		n, err := cryptorand.Int(random, bound)
		if err != nil {
			return result, err
		}
		candidate := n.Uint64()
		if _, exists := selectedPositions[candidate]; exists {
			candidate = j
		}
		selectedPositions[candidate] = struct{}{}
	}
	if len(selectedPositions) != shielded6.RingSize-1 {
		return result, errors.New("Shield6 decoy selection produced a duplicate index")
	}
	indices := make([]uint64, 0, shielded6.RingSize)
	indices = append(indices, realIndex)
	for position := range selectedPositions {
		index := position
		if index >= realIndex {
			index++
		}
		indices = append(indices, index)
	}
	// Randomize RPC request order so the real member is not a distinguished
	// first/last lookup. Fisher-Yates uses rejection-sampled crypto/rand.Int.
	for i := len(indices) - 1; i > 0; i-- {
		n, err := cryptorand.Int(random, new(big.Int).SetInt64(int64(i+1)))
		if err != nil {
			return result, err
		}
		j := int(n.Int64())
		indices[i], indices[j] = indices[j], indices[i]
	}
	request := make([]hexutil.Uint64, len(indices))
	for i, index := range indices {
		request[i] = hexutil.Uint64(index)
	}
	var page []shieldedV6RingRPCOutput
	if err := rpc.CallContext(ctx, &page, "tkmprivacy_shieldedV6OutputsByIndex", request); err != nil {
		return result, fmt.Errorf("fetch Shield6 ring candidates: %w", err)
	}
	if len(page) != len(indices) {
		return result, errors.New("node returned an incomplete Shield6 ring-candidate batch")
	}
	candidates := make([]struct {
		index  uint64
		member shielded6.RingMember
	}, len(indices))
	for i, index := range indices {
		if uint64(page[i].Index) != index || len(page[i].OneTimeKey) != shielded6.PointSize || len(page[i].Commitment) != shielded6.PointSize {
			return result, fmt.Errorf("node returned a malformed Shield6 ring member at %d", index)
		}
		candidates[i].index = index
		copy(candidates[i].member.OneTimeKey[:], page[i].OneTimeKey)
		copy(candidates[i].member.Commitment[:], page[i].Commitment)
	}
	var after shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &after, "tkmprivacy_shieldedV6Status"); err != nil {
		return result, err
	}
	if !after.Active || after.Accumulator != before.Accumulator || after.OutputCount != before.OutputCount || after.ChainID == nil || (*big.Int)(after.ChainID).Cmp(new(big.Int).SetUint64(chainID)) != 0 {
		return result, errors.New("Shield6 accumulator changed during ring selection; retry")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].index < candidates[j].index })
	ordered := make([]shielded6.IndexedRingMember, len(candidates))
	for i, candidate := range candidates {
		ordered[i] = shielded6.IndexedRingMember{Index: candidate.index, Member: candidate.member}
	}
	ring, ringIndices, signer, err := shielded6.SelectRing(realIndex, ordered, random)
	if err != nil {
		return result, err
	}
	result.Members, result.Indices, result.SignerIndex, result.Root = ring, ringIndices, signer, before.Accumulator
	return result, nil
}

// ScanShieldedV6 fetches bounded canonical output pages, scans encrypted notes
// locally, and excludes confirmed or pending key-image spends. It rejects a
// scan if the chain head or accumulator changes while pages are being read.
func ScanShieldedV6(ctx context.Context, rpc RPC, keys *shielded6.WalletKeys) (ShieldedV6ScanResult, error) {
	result := ShieldedV6ScanResult{BalanceWei: new(big.Int), Notes: make([]ShieldedV6OwnedNote, 0)}
	if rpc == nil || keys == nil || keys.ChainID == 0 {
		return result, errors.New("Shield6 wallet scanner requires RPC and wallet keys")
	}
	var chain hexutil.Big
	if err := rpc.CallContext(ctx, &chain, "eth_chainId"); err != nil {
		return result, err
	}
	if (*big.Int)(&chain).Cmp(new(big.Int).SetUint64(keys.ChainID)) != 0 {
		return result, errors.New("Shield6 wallet chain ID mismatch")
	}
	var head struct {
		Hash   common.Hash    `json:"hash"`
		Number hexutil.Uint64 `json:"number"`
	}
	if err := rpc.CallContext(ctx, &head, "eth_getBlockByNumber", "latest", false); err != nil {
		return result, err
	}
	if head.Hash == (common.Hash{}) {
		return result, errors.New("Shield6 chain head unavailable")
	}
	var status shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &status, "tkmprivacy_shieldedV6Status"); err != nil {
		return result, err
	}
	if status.ChainID == nil || (*big.Int)(status.ChainID).Cmp(new(big.Int).SetUint64(keys.ChainID)) != 0 {
		return result, errors.New("Shield6 node chain ID mismatch")
	}
	if !status.Active {
		return result, errors.New("Shield6 is not active on this chain")
	}
	result.OutputCount = uint64(status.OutputCount)
	result.AccumulatorRoot = status.Accumulator
	result.HeadHash, result.HeadNumber = head.Hash, uint64(head.Number)
	// Match the public canonical key-image set against owned notes locally.
	// Sending wallet-derived key images to RPC would disclose which outputs
	// this wallet controls.
	spentKeyImages := make(map[[shielded6.PointSize]byte]struct{})
	for from := uint64(0); from < uint64(status.SpentKeyImageCount); {
		limit := shieldedV6KeyImagePage
		if left := uint64(status.SpentKeyImageCount) - from; left < limit {
			limit = left
		}
		var page []hexutil.Bytes
		if err := rpc.CallContext(ctx, &page, "tkmprivacy_shieldedV6SpentKeyImages", hexutil.Uint64(from), hexutil.Uint64(limit)); err != nil {
			return result, fmt.Errorf("fetch public Shield6 key-image page at %d: %w", from, err)
		}
		if uint64(len(page)) != limit {
			return result, fmt.Errorf("incomplete public Shield6 key-image page at %d", from)
		}
		for i, encoded := range page {
			if len(encoded) != shielded6.PointSize || shielded6.ValidatePublicKey(encoded) != nil {
				return result, fmt.Errorf("malformed public Shield6 key image at %d", from+uint64(i))
			}
			var image [shielded6.PointSize]byte
			copy(image[:], encoded)
			if _, duplicate := spentKeyImages[image]; duplicate {
				return result, fmt.Errorf("duplicate public Shield6 key image at %d", from+uint64(i))
			}
			spentKeyImages[image] = struct{}{}
		}
		from += limit
	}
	var pendingKeyImages []hexutil.Bytes
	if err := rpc.CallContext(ctx, &pendingKeyImages, "tkmprivacy_shieldedV6PendingKeyImages"); err != nil {
		return result, fmt.Errorf("fetch public pending Shield6 key images: %w", err)
	}
	for i, encoded := range pendingKeyImages {
		if len(encoded) != shielded6.PointSize || shielded6.ValidatePublicKey(encoded) != nil {
			return result, fmt.Errorf("malformed pending Shield6 key image at %d", i)
		}
		var image [shielded6.PointSize]byte
		copy(image[:], encoded)
		spentKeyImages[image] = struct{}{}
	}
	for from := uint64(0); from < result.OutputCount; {
		limit := shieldedV6WalletPage
		if left := result.OutputCount - from; left < limit {
			limit = left
		}
		var outputs []shieldedV6RPCOutput
		if err := rpc.CallContext(ctx, &outputs, "tkmprivacy_shieldedV6WalletOutputs", hexutil.Uint64(from), hexutil.Uint64(limit)); err != nil {
			clearShieldedV6Notes(result.Notes)
			return ShieldedV6ScanResult{}, fmt.Errorf("fetch Shield6 wallet output page at %d: %w", from, err)
		}
		if uint64(len(outputs)) != limit {
			clearShieldedV6Notes(result.Notes)
			return ShieldedV6ScanResult{}, fmt.Errorf("incomplete Shield6 output page at %d", from)
		}
		candidates := make([]shieldedV6SpendCandidate, 0)
		clearCandidates := func() { clearShieldedV6SpendCandidates(candidates) }
		for i := range outputs {
			view := outputs[i]
			if uint64(view.AccumulatorIndex) != from+uint64(i) || view.BlockHash == (common.Hash{}) || view.TransactionHash == (common.Hash{}) {
				clearCandidates()
				clearShieldedV6Notes(result.Notes)
				return ShieldedV6ScanResult{}, fmt.Errorf("noncanonical Shield6 output record at %d", from+uint64(i))
			}
			output, err := shieldedV6OutputFromRPC(view)
			if err != nil {
				clearCandidates()
				clearShieldedV6Notes(result.Notes)
				return ShieldedV6ScanResult{}, fmt.Errorf("malformed Shield6 output %d: %w", from+uint64(i), err)
			}
			note, owned, err := shielded6.ScanOutput(keys, uint64(view.OutputPosition), uint64(view.AccumulatorIndex), output)
			if err != nil {
				if note != nil {
					note.Clear()
				}
				clearCandidates()
				clearShieldedV6Notes(result.Notes)
				return ShieldedV6ScanResult{}, fmt.Errorf("scan Shield6 output %d: %w", from+uint64(i), err)
			}
			if !owned {
				if note != nil {
					note.Clear()
				}
				continue
			}
			keyImage, err := shielded6.DeriveKeyImage(output.OneTimeKey[:], note.SpendSecret[:])
			if err != nil {
				note.Clear()
				clearCandidates()
				clearShieldedV6Notes(result.Notes)
				return ShieldedV6ScanResult{}, fmt.Errorf("derive Shield6 key image for output %d: %w", from+uint64(i), err)
			}
			_, isSpent := spentKeyImages[keyImage]
			clear(keyImage[:])
			if isSpent {
				note.Clear()
				continue
			}
			candidates = append(candidates, shieldedV6SpendCandidate{
				note: note, transactionHash: view.TransactionHash,
				blockNumber: uint64(view.BlockNumber),
			})
		}
		if len(candidates) > 0 {
			for i := range candidates {
				result.BalanceWei.Add(result.BalanceWei, candidates[i].note.Amount)
				result.Notes = append(result.Notes, ShieldedV6OwnedNote{
					Note: candidates[i].note, TransactionHash: candidates[i].transactionHash,
					BlockNumber: candidates[i].blockNumber,
				})
				candidates[i].note = nil // ownership moved to result.Notes
			}
			clearCandidates()
		}
		from += limit
	}
	var endStatus shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &endStatus, "tkmprivacy_shieldedV6Status"); err != nil {
		clearShieldedV6Notes(result.Notes)
		return ShieldedV6ScanResult{}, err
	}
	var endHead struct {
		Hash common.Hash `json:"hash"`
	}
	if err := rpc.CallContext(ctx, &endHead, "eth_getBlockByNumber", "latest", false); err != nil {
		clearShieldedV6Notes(result.Notes)
		return ShieldedV6ScanResult{}, err
	}
	if !endStatus.Active || endStatus.ChainID == nil || (*big.Int)(endStatus.ChainID).Cmp(new(big.Int).SetUint64(keys.ChainID)) != 0 || endStatus.Accumulator != status.Accumulator || uint64(endStatus.OutputCount) != uint64(status.OutputCount) || endStatus.SpentKeyImageCount != status.SpentKeyImageCount || endHead.Hash != head.Hash {
		clearShieldedV6Notes(result.Notes)
		return ShieldedV6ScanResult{}, errors.New("chain changed during Shield6 scan; retry from the new canonical head")
	}
	return result, nil
}

func shieldedV6OutputFromRPC(output shieldedV6RPCOutput) (shielded6.OutputRecord, error) {
	var record shielded6.OutputRecord
	for _, field := range []struct {
		name string
		data []byte
		dest []byte
	}{
		{"one-time key", output.OneTimeKey, record.OneTimeKey[:]},
		{"ephemeral key", output.EphemeralKey, record.EphemeralKey[:]},
		{"commitment", output.Commitment, record.Commitment[:]},
	} {
		if len(field.data) != len(field.dest) {
			return record, fmt.Errorf("%s has %d bytes", field.name, len(field.data))
		}
		copy(field.dest, field.data)
	}
	for i := range record.AmountLimbs {
		if len(output.AmountLimbs[i]) != len(record.AmountLimbs[i]) {
			return record, fmt.Errorf("amount limb %d has %d bytes", i, len(output.AmountLimbs[i]))
		}
		copy(record.AmountLimbs[i][:], output.AmountLimbs[i])
	}
	if len(output.EncryptedNote) == 0 || len(output.EncryptedNote) > core.ShieldedV6MaxCiphertext {
		return record, errors.New("encrypted note size is invalid")
	}
	record.EncryptedNote = append([]byte(nil), output.EncryptedNote...)
	return record, nil
}

func clearShieldedV6Notes(notes []ShieldedV6OwnedNote) {
	for i := range notes {
		if notes[i].Note != nil {
			notes[i].Note.Clear()
		}
	}
}

func clearShieldedV6SpendCandidates(candidates []shieldedV6SpendCandidate) {
	for i := range candidates {
		if candidates[i].note != nil {
			candidates[i].note.Clear()
			candidates[i].note = nil
		}
	}
}

// BuildAndSignShieldedV6Payment creates a complete private Shield6 payment
// using the canonical accumulator, then signs the outer account transaction
// with the wallet's ML-DSA-87 key. The caller supplies an unsigned PQ
// transaction skeleton containing nonce, fee caps, gas limit and public key.
// Notes and keys remain caller-owned and must be cleared by the caller.
func BuildAndSignShieldedV6Payment(ctx context.Context, rpc RPC, accountSeed []byte, identity *Identity, keys *shielded6.WalletKeys, skeleton *types.Transaction, notes []*shielded6.Note, outputs []core.ShieldedV6WalletOutput, random io.Reader) (*types.Transaction, error) {
	if rpc == nil || identity == nil || keys == nil || skeleton == nil || len(notes) == 0 || len(notes) > core.ShieldedV6MaxInputs || len(outputs) == 0 || len(outputs) > core.ShieldedV6MaxOutputs {
		return nil, errors.New("invalid Shield6 wallet payment request")
	}
	if identity.ChainID == 0 || keys.ChainID != identity.ChainID || skeleton.Type() != types.PQTkmTxType || skeleton.ChainId().Cmp(new(big.Int).SetUint64(identity.ChainID)) != 0 || skeleton.To() == nil || *skeleton.To() != params.ShieldedPoolAddress || skeleton.Value().Sign() != 0 || len(skeleton.Data()) != 0 || skeleton.Gas() == 0 || skeleton.GasFeeCap().Sign() <= 0 || skeleton.GasTipCap().Sign() < 0 || skeleton.GasTipCap().Cmp(skeleton.GasFeeCap()) > 0 {
		return nil, errors.New("invalid Shield6 PQ transaction skeleton")
	}
	algorithm, publicKey, signature, ok := skeleton.PQTkmFields()
	if !ok || algorithm != pqcrypto.AlgorithmMLDSA87 || len(signature) != 0 {
		return nil, errors.New("Shield6 payment requires an unsigned ML-DSA-87 transaction skeleton")
	}
	pqKey, err := pqcrypto.NewMLDSA87FromSeed(accountSeed)
	if err != nil {
		return nil, err
	}
	derivedAddress, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(pqKey))
	if err != nil || derivedAddress != identity.Address || !bytes.Equal(publicKey, pqcrypto.PublicKeyBytes(pqKey)) {
		return nil, errors.New("Shield6 account seed does not match the transaction sender identity")
	}
	verifiedIdentity, err := NewIdentity(accountSeed, identity.ChainID, identity.Stamp)
	if err != nil {
		return nil, err
	}
	defer verifiedIdentity.Clear()
	if verifiedIdentity.Owner != identity.Owner || verifiedIdentity.StampPublicKey == nil || !bytes.Equal(verifiedIdentity.StampPublicKey, identity.StampPublicKey) {
		return nil, errors.New("Shield6 account identity does not match its stamped keyfile")
	}
	stampPayload, err := verifiedIdentity.RegisteredStampPayload()
	if err != nil {
		return nil, err
	}
	if err := RequireRegisteredStamp(ctx, rpc, stampPayload); err != nil {
		return nil, err
	}
	var status shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &status, "tkmprivacy_shieldedV6Status"); err != nil {
		return nil, err
	}
	if !status.Active || status.ChainID == nil || (*big.Int)(status.ChainID).Cmp(new(big.Int).SetUint64(identity.ChainID)) != 0 {
		return nil, errors.New("Shield6 is not active on this wallet's chain")
	}
	var balance hexutil.Big
	if err := rpc.CallContext(ctx, &balance, "eth_getBalance", identity.Address, "latest"); err != nil {
		return nil, err
	}
	maxFee := new(big.Int).Mul(skeleton.GasFeeCap(), new(big.Int).SetUint64(skeleton.Gas()))
	if (*big.Int)(&balance).Cmp(maxFee) < 0 {
		return nil, errors.New("public account balance cannot cover Shield6 transaction gas")
	}
	if random == nil {
		random = cryptorand.Reader
	}
	inputs := make([]core.ShieldedV6WalletInput, len(notes))
	seenIndices := make(map[uint64]struct{}, len(notes))
	for i, note := range notes {
		if note == nil || note.ChainID != identity.ChainID || note.AssetID != shielded6.AssetTKM || note.Amount == nil || note.Amount.Sign() <= 0 {
			return nil, fmt.Errorf("invalid Shield6 input note %d", i)
		}
		if _, duplicate := seenIndices[note.Index]; duplicate {
			return nil, errors.New("Shield6 payment contains the same note more than once")
		}
		seenIndices[note.Index] = struct{}{}
		var ring ShieldedV6Ring
		ring, err := SelectShieldedV6Ring(ctx, rpc, identity.ChainID, note.Index, random)
		if err != nil {
			return nil, fmt.Errorf("select Shield6 input ring %d: %w", i, err)
		}
		if ring.Root != status.Accumulator {
			return nil, errors.New("Shield6 accumulator changed during payment construction; rescan and retry")
		}
		// Do not ask RPC about the selected input's key image: that would
		// identify the real note. The wallet scan matches confirmed public key
		// images locally; any race is safely rejected by txpool/consensus.
		inputs[i] = core.ShieldedV6WalletInput{Note: note, Ring: ring.Members, RingIndices: ring.Indices, SignerIndex: ring.SignerIndex}
	}
	data, err := core.BuildShieldedV6Envelope(skeleton, inputs, outputs, random)
	if err != nil {
		return nil, err
	}
	unsigned := types.NewTx(&types.PQTkmTx{
		ChainID: skeleton.ChainId(), Nonce: skeleton.Nonce(), GasTipCap: skeleton.GasTipCap(),
		GasFeeCap: skeleton.GasFeeCap(), Gas: skeleton.Gas(), To: skeleton.To(), Value: skeleton.Value(),
		Data: data, AccessList: skeleton.AccessList(), Algorithm: algorithm, PublicKey: publicKey,
	})
	gas, err := core.IntrinsicGasWithShield3(unsigned.Data(), unsigned.AccessList(), unsigned.SetCodeAuthorizations(), false, true, true, true, false, true)
	if err != nil {
		return nil, err
	}
	if gas.RegularGas > unsigned.Gas() {
		return nil, errors.New("Shield6 proof and signature exceed the authorized gas limit")
	}
	signed, err := types.SignPQTkmTx(unsigned, types.NewQuantumSigner(new(big.Int).SetUint64(identity.ChainID)), pqKey)
	if err != nil {
		return nil, err
	}
	from, err := types.Sender(types.NewQuantumSigner(new(big.Int).SetUint64(identity.ChainID)), signed)
	if err != nil || from != identity.Address {
		return nil, errors.New("signed Shield6 transaction sender mismatch")
	}
	return signed, nil
}

// BuildAndSignShieldedV6Deposit creates an explicit public-to-private
// migration/deposit transaction. The amount and sender remain public at this
// boundary; the resulting note output is encrypted for the Shield6 wallet.
// The caller must wait for any legacy self-withdrawal to confirm before using
// its public balance here.
func BuildAndSignShieldedV6Deposit(ctx context.Context, rpc RPC, accountSeed []byte, identity *Identity, keys *shielded6.WalletKeys, skeleton *types.Transaction, amount *big.Int, random io.Reader) (*types.Transaction, error) {
	if rpc == nil || identity == nil || keys == nil || skeleton == nil || amount == nil || amount.Sign() <= 0 || amount.BitLen() > 128 {
		return nil, errors.New("invalid Shield6 deposit request")
	}
	if identity.ChainID == 0 || keys.ChainID != identity.ChainID || skeleton.Type() != types.PQTkmTxType || skeleton.ChainId().Cmp(new(big.Int).SetUint64(identity.ChainID)) != 0 || skeleton.To() == nil || *skeleton.To() != params.ShieldedPoolAddress || skeleton.Value().Cmp(amount) != 0 || len(skeleton.Data()) != 0 || skeleton.Gas() == 0 || skeleton.GasFeeCap().Sign() <= 0 || skeleton.GasTipCap().Sign() < 0 || skeleton.GasTipCap().Cmp(skeleton.GasFeeCap()) > 0 {
		return nil, errors.New("invalid Shield6 deposit transaction skeleton")
	}
	algorithm, publicKey, signature, ok := skeleton.PQTkmFields()
	if !ok || algorithm != pqcrypto.AlgorithmMLDSA87 || len(signature) != 0 {
		return nil, errors.New("Shield6 deposit requires an unsigned ML-DSA-87 transaction skeleton")
	}
	pqKey, err := pqcrypto.NewMLDSA87FromSeed(accountSeed)
	if err != nil {
		return nil, err
	}
	derivedAddress, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(pqKey))
	if err != nil || derivedAddress != identity.Address || !bytes.Equal(publicKey, pqcrypto.PublicKeyBytes(pqKey)) {
		return nil, errors.New("Shield6 deposit account seed does not match the transaction sender")
	}
	verifiedIdentity, err := NewIdentity(accountSeed, identity.ChainID, identity.Stamp)
	if err != nil {
		return nil, err
	}
	defer verifiedIdentity.Clear()
	if verifiedIdentity.Owner != identity.Owner || verifiedIdentity.StampPublicKey == nil || !bytes.Equal(verifiedIdentity.StampPublicKey, identity.StampPublicKey) {
		return nil, errors.New("Shield6 deposit identity does not match its stamped keyfile")
	}
	stampPayload, err := verifiedIdentity.RegisteredStampPayload()
	if err != nil {
		return nil, err
	}
	if err := RequireRegisteredStamp(ctx, rpc, stampPayload); err != nil {
		return nil, fmt.Errorf("Shield6 deposit sender stamp: %w", err)
	}
	var status shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &status, "tkmprivacy_shieldedV6Status"); err != nil {
		return nil, err
	}
	if !status.Active || status.ChainID == nil || (*big.Int)(status.ChainID).Cmp(new(big.Int).SetUint64(identity.ChainID)) != 0 {
		return nil, errors.New("Shield6 is not active on this wallet's chain")
	}
	var balance hexutil.Big
	if err := rpc.CallContext(ctx, &balance, "eth_getBalance", identity.Address, "latest"); err != nil {
		return nil, err
	}
	reserve := new(big.Int).Mul(skeleton.GasFeeCap(), new(big.Int).SetUint64(skeleton.Gas()))
	need := new(big.Int).Add(new(big.Int).Set(amount), reserve)
	if (*big.Int)(&balance).Cmp(need) < 0 {
		return nil, errors.New("public account balance cannot cover Shield6 deposit and maximum transaction gas")
	}
	if random == nil {
		random = cryptorand.Reader
	}
	data, err := core.BuildShieldedV6DepositEnvelope(skeleton, []core.ShieldedV6WalletOutput{{Address: keys.Address(), Amount: new(big.Int).Set(amount)}}, random)
	if err != nil {
		return nil, err
	}
	unsigned := types.NewTx(&types.PQTkmTx{
		ChainID: skeleton.ChainId(), Nonce: skeleton.Nonce(), GasTipCap: skeleton.GasTipCap(),
		GasFeeCap: skeleton.GasFeeCap(), Gas: skeleton.Gas(), To: skeleton.To(), Value: skeleton.Value(),
		Data: data, AccessList: skeleton.AccessList(), Algorithm: algorithm, PublicKey: publicKey,
	})
	gas, err := core.IntrinsicGasWithShield3(unsigned.Data(), unsigned.AccessList(), unsigned.SetCodeAuthorizations(), false, true, true, true, false, true)
	if err != nil {
		return nil, err
	}
	if gas.RegularGas > unsigned.Gas() {
		return nil, errors.New("Shield6 deposit proof and signature exceed the authorized gas limit")
	}
	signed, err := types.SignPQTkmTx(unsigned, types.NewQuantumSigner(new(big.Int).SetUint64(identity.ChainID)), pqKey)
	if err != nil {
		return nil, err
	}
	from, err := types.Sender(types.NewQuantumSigner(new(big.Int).SetUint64(identity.ChainID)), signed)
	if err != nil || from != identity.Address {
		return nil, errors.New("signed Shield6 deposit sender mismatch")
	}
	return signed, nil
}

// BuildAndSignShieldedV6Withdrawal creates a public exit from Shield6. The
// amount and recipient are necessarily visible; any remainder is returned as
// a confidential Shield6 change output. The caller must provide already
// scanned spendable notes and an unsigned PQ transaction skeleton.
func BuildAndSignShieldedV6Withdrawal(ctx context.Context, rpc RPC, accountSeed []byte, identity *Identity, keys *shielded6.WalletKeys, skeleton *types.Transaction, notes []*shielded6.Note, recipient common.Address, amount *big.Int, random io.Reader) (*types.Transaction, error) {
	if rpc == nil || identity == nil || keys == nil || skeleton == nil || len(notes) == 0 || len(notes) > core.ShieldedV6MaxInputs || recipient == (common.Address{}) || amount == nil || amount.Sign() <= 0 || amount.BitLen() > 128 {
		return nil, errors.New("invalid Shield6 withdrawal request")
	}
	if identity.ChainID == 0 || keys.ChainID != identity.ChainID || skeleton.Type() != types.PQTkmTxType || skeleton.ChainId().Cmp(new(big.Int).SetUint64(identity.ChainID)) != 0 || skeleton.To() == nil || *skeleton.To() != params.ShieldedPoolAddress || skeleton.Value().Sign() != 0 || len(skeleton.Data()) != 0 || skeleton.Gas() == 0 || skeleton.GasFeeCap().Sign() <= 0 || skeleton.GasTipCap().Sign() < 0 || skeleton.GasTipCap().Cmp(skeleton.GasFeeCap()) > 0 {
		return nil, errors.New("invalid Shield6 withdrawal transaction skeleton")
	}
	algorithm, publicKey, signature, ok := skeleton.PQTkmFields()
	if !ok || algorithm != pqcrypto.AlgorithmMLDSA87 || len(signature) != 0 {
		return nil, errors.New("Shield6 withdrawal requires an unsigned ML-DSA-87 transaction skeleton")
	}
	pqKey, err := pqcrypto.NewMLDSA87FromSeed(accountSeed)
	if err != nil {
		return nil, err
	}
	derivedAddress, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(pqKey))
	if err != nil || derivedAddress != identity.Address || !bytes.Equal(publicKey, pqcrypto.PublicKeyBytes(pqKey)) {
		return nil, errors.New("Shield6 withdrawal account seed does not match the transaction sender")
	}
	verifiedIdentity, err := NewIdentity(accountSeed, identity.ChainID, identity.Stamp)
	if err != nil {
		return nil, err
	}
	defer verifiedIdentity.Clear()
	if verifiedIdentity.Owner != identity.Owner || verifiedIdentity.StampPublicKey == nil || !bytes.Equal(verifiedIdentity.StampPublicKey, identity.StampPublicKey) {
		return nil, errors.New("Shield6 withdrawal identity does not match its stamped keyfile")
	}
	stampPayload, err := verifiedIdentity.RegisteredStampPayload()
	if err != nil {
		return nil, err
	}
	if err := RequireRegisteredStamp(ctx, rpc, stampPayload); err != nil {
		return nil, fmt.Errorf("Shield6 withdrawal sender stamp: %w", err)
	}
	if err := RequireRegisteredAddress(ctx, rpc, recipient); err != nil {
		return nil, err
	}
	var status shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &status, "tkmprivacy_shieldedV6Status"); err != nil {
		return nil, err
	}
	if !status.Active || status.ChainID == nil || (*big.Int)(status.ChainID).Cmp(new(big.Int).SetUint64(identity.ChainID)) != 0 {
		return nil, errors.New("Shield6 is not active on this wallet's chain")
	}
	var balance hexutil.Big
	if err := rpc.CallContext(ctx, &balance, "eth_getBalance", identity.Address, "latest"); err != nil {
		return nil, err
	}
	maxFee := new(big.Int).Mul(skeleton.GasFeeCap(), new(big.Int).SetUint64(skeleton.Gas()))
	if (*big.Int)(&balance).Cmp(maxFee) < 0 {
		return nil, errors.New("public account balance cannot cover Shield6 withdrawal transaction gas")
	}
	if random == nil {
		random = cryptorand.Reader
	}
	total := new(big.Int)
	defer clearShieldedV6WalletBigInt(total)
	for i, note := range notes {
		if note == nil || note.ChainID != identity.ChainID || note.AssetID != shielded6.AssetTKM || note.Amount == nil || note.Amount.Sign() <= 0 {
			return nil, fmt.Errorf("invalid Shield6 withdrawal input note %d", i)
		}
		for j := 0; j < i; j++ {
			if notes[j].Index == note.Index {
				return nil, errors.New("Shield6 withdrawal contains the same note more than once")
			}
		}
		total.Add(total, note.Amount)
	}
	if total.Cmp(amount) < 0 {
		return nil, errors.New("selected Shield6 notes do not cover the public withdrawal amount")
	}
	outputs := make([]core.ShieldedV6WalletOutput, 0, 1)
	change := new(big.Int).Sub(total, amount)
	defer clearShieldedV6WalletBigInt(change)
	if change.Sign() > 0 {
		outputs = append(outputs, core.ShieldedV6WalletOutput{Address: keys.Address(), Amount: change})
	}
	inputs := make([]core.ShieldedV6WalletInput, len(notes))
	for i, note := range notes {
		ring, err := SelectShieldedV6Ring(ctx, rpc, identity.ChainID, note.Index, random)
		if err != nil {
			return nil, fmt.Errorf("select Shield6 withdrawal ring %d: %w", i, err)
		}
		if ring.Root != status.Accumulator {
			return nil, errors.New("Shield6 accumulator changed during withdrawal construction; rescan and retry")
		}
		inputs[i] = core.ShieldedV6WalletInput{Note: note, Ring: ring.Members, RingIndices: ring.Indices, SignerIndex: ring.SignerIndex}
	}
	data, err := core.BuildShieldedV6WithdrawalEnvelope(skeleton, inputs, outputs, recipient, amount, random)
	if err != nil {
		return nil, err
	}
	unsigned := types.NewTx(&types.PQTkmTx{
		ChainID: skeleton.ChainId(), Nonce: skeleton.Nonce(), GasTipCap: skeleton.GasTipCap(),
		GasFeeCap: skeleton.GasFeeCap(), Gas: skeleton.Gas(), To: skeleton.To(), Value: skeleton.Value(),
		Data: data, AccessList: skeleton.AccessList(), Algorithm: algorithm, PublicKey: publicKey,
	})
	gas, err := core.IntrinsicGasWithShield3(unsigned.Data(), unsigned.AccessList(), unsigned.SetCodeAuthorizations(), false, true, true, true, false, true)
	if err != nil {
		return nil, err
	}
	if gas.RegularGas > unsigned.Gas() {
		return nil, errors.New("Shield6 withdrawal proof and signature exceed the authorized gas limit")
	}
	signed, err := types.SignPQTkmTx(unsigned, types.NewQuantumSigner(new(big.Int).SetUint64(identity.ChainID)), pqKey)
	if err != nil {
		return nil, err
	}
	from, err := types.Sender(types.NewQuantumSigner(new(big.Int).SetUint64(identity.ChainID)), signed)
	if err != nil || from != identity.Address {
		return nil, errors.New("signed Shield6 withdrawal sender mismatch")
	}
	return signed, nil
}

func clearShieldedV6WalletBigInt(value *big.Int) {
	if value == nil {
		return
	}
	clear(value.Bits())
	value.SetInt64(0)
}
