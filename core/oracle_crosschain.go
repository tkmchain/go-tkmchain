package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/antartical"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	OracleObservationMagic = "TKM-ORACLE-ENVELOPE-V1"
	CrossChainMessageMagic = "TKM-XCHAIN-ENVELOPE-V1"
	MaxOracleValueBytes    = 4096
	MaxCrossChainPayload   = 64 * 1024
)

var (
	ErrOracleEnvelope     = errors.New("invalid TKM oracle envelope")
	ErrOracleReplay       = errors.New("TKM oracle round is not newer than state")
	ErrCrossChainEnvelope = errors.New("invalid TKM cross-chain envelope")
	ErrCrossChainReplay   = errors.New("TKM cross-chain message already relayed")
)

func HasOracleObservationPrefix(data []byte) bool {
	return bytes.HasPrefix(data, []byte(OracleObservationMagic))
}

func HasCrossChainMessagePrefix(data []byte) bool {
	return bytes.HasPrefix(data, []byte(CrossChainMessageMagic))
}

func EncodeOracleObservation(o *antartical.OracleObservation) ([]byte, error) {
	if o == nil {
		return nil, ErrOracleEnvelope
	}
	b, err := rlp.EncodeToBytes(o)
	if err != nil {
		return nil, err
	}
	return append([]byte(OracleObservationMagic), b...), nil
}

func DecodeOracleObservation(data []byte) (*antartical.OracleObservation, error) {
	if !HasOracleObservationPrefix(data) || uint64(len(data)) > ShieldedV3MaxTxSize {
		return nil, ErrOracleEnvelope
	}
	var o antartical.OracleObservation
	if err := rlp.DecodeBytes(data[len(OracleObservationMagic):], &o); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOracleEnvelope, err)
	}
	return &o, nil
}

func EncodeCrossChainMessage(m *antartical.SignedCrossChainMessage) ([]byte, error) {
	if m == nil {
		return nil, ErrCrossChainEnvelope
	}
	b, err := rlp.EncodeToBytes(m)
	if err != nil {
		return nil, err
	}
	return append([]byte(CrossChainMessageMagic), b...), nil
}

func DecodeCrossChainMessage(data []byte) (*antartical.SignedCrossChainMessage, error) {
	if !HasCrossChainMessagePrefix(data) || uint64(len(data)) > ShieldedV3MaxTxSize {
		return nil, ErrCrossChainEnvelope
	}
	var m antartical.SignedCrossChainMessage
	if err := rlp.DecodeBytes(data[len(CrossChainMessageMagic):], &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCrossChainEnvelope, err)
	}
	return &m, nil
}

func oracleRoundSlot(feed common.Hash) common.Hash {
	return ShieldedV3StateSlot("oracle/round", feed.Bytes())
}

func oracleObservationSlot(feed common.Hash, round uint64) common.Hash {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], round)
	key := append(append([]byte(nil), feed.Bytes()...), encoded[:]...)
	return ShieldedV3StateSlot("oracle/observation", key)
}

func oracleValueLengthSlot(feed common.Hash, round uint64) common.Hash {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], round)
	key := append(append([]byte(nil), feed.Bytes()...), encoded[:]...)
	return ShieldedV3StateSlot("oracle/value-length", key)
}

func oracleValueChunkSlot(feed common.Hash, round, index uint64) common.Hash {
	var encoded [16]byte
	binary.BigEndian.PutUint64(encoded[:8], round)
	binary.BigEndian.PutUint64(encoded[8:], index)
	key := append(append([]byte(nil), feed.Bytes()...), encoded[:]...)
	return ShieldedV3StateSlot("oracle/value", key)
}

// OracleObservationValue returns the canonical value persisted by the latest
// accepted round. Values are stored in 32-byte words so every client derives
// the same state root without depending on an ABI or database encoding.
func OracleObservationValue(st *state.StateDB, feed common.Hash, round uint64) ([]byte, bool) {
	if st == nil {
		return nil, false
	}
	length := uint64FromHash(st.GetState(params.ShieldedPoolAddress, oracleValueLengthSlot(feed, round)))
	if length == 0 || length > MaxOracleValueBytes {
		return nil, false
	}
	value := make([]byte, 0, length)
	for index := uint64(0); uint64(len(value)) < length; index++ {
		value = append(value, st.GetState(params.ShieldedPoolAddress, oracleValueChunkSlot(feed, round, index)).Bytes()...)
	}
	return value[:length], true
}

func crossChainReplaySlot(key common.Hash) common.Hash {
	return ShieldedV3StateSlot("cross-chain/replay", key.Bytes())
}

func crossChainPayloadLengthSlot(key common.Hash) common.Hash {
	return ShieldedV3StateSlot("cross-chain/payload-length", key.Bytes())
}

func crossChainPayloadChunkSlot(key common.Hash, index uint64) common.Hash {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], index)
	return ShieldedV3StateSlot("cross-chain/payload", append(append([]byte(nil), key.Bytes()...), encoded[:]...))
}

func CrossChainPayload(st *state.StateDB, key common.Hash) ([]byte, bool) {
	if st == nil {
		return nil, false
	}
	length := uint64FromHash(st.GetState(params.ShieldedPoolAddress, crossChainPayloadLengthSlot(key)))
	if length == 0 || length > MaxCrossChainPayload {
		return nil, false
	}
	payload := make([]byte, 0, length)
	for index := uint64(0); uint64(len(payload)) < length; index++ {
		payload = append(payload, st.GetState(params.ShieldedPoolAddress, crossChainPayloadChunkSlot(key, index)).Bytes()...)
	}
	return payload[:length], true
}

// ValidateOracleTransaction applies an observation to consensus state.  The
// state stores the highest accepted round for each feed, so an old observation
// cannot overwrite a newer price even when relayed after a reorg.
func ProcessOracleTransaction(config *params.ChainConfig, number *big.Int, timestamp uint64, st *state.StateDB, tx *types.Transaction, sender common.Address) error {
	if config == nil || !config.IsAntartical(number, timestamp) {
		return antartical.ErrProfileInactive
	}
	if tx == nil || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Sign() != 0 || tx.ChainId().Cmp(config.ChainID) != 0 {
		return ErrOracleEnvelope
	}
	o, err := DecodeOracleObservation(tx.Data())
	if err != nil {
		return err
	}
	if o.Signer != sender || o.Verify(config.ChainID) != nil {
		return ErrOracleEnvelope
	}
	if len(o.Value) == 0 || len(o.Value) > MaxOracleValueBytes {
		return ErrOracleEnvelope
	}
	previous := uint64FromHash(st.GetState(params.ShieldedPoolAddress, oracleRoundSlot(o.FeedID)))
	if o.Round <= previous {
		return fmt.Errorf("%w: feed %s have %d want > %d", ErrOracleReplay, o.FeedID, o.Round, previous)
	}
	st.SetState(params.ShieldedPoolAddress, oracleRoundSlot(o.FeedID), uint64Hash(o.Round))
	hash, _ := o.Hash(config.ChainID)
	st.SetState(params.ShieldedPoolAddress, oracleObservationSlot(o.FeedID, o.Round), hash)
	st.SetState(params.ShieldedPoolAddress, oracleValueLengthSlot(o.FeedID, o.Round), uint64Hash(uint64(len(o.Value))))
	for index := 0; index*32 < len(o.Value); index++ {
		var word [32]byte
		copy(word[:], o.Value[index*32:])
		st.SetState(params.ShieldedPoolAddress, oracleValueChunkSlot(o.FeedID, o.Round, uint64(index)), common.Hash(word))
	}
	return nil
}

// ProcessCrossChainTransaction records an authenticated, destination-bound
// relay. It is intentionally a message layer only: bridge applications must
// consume this replay-protected commitment before releasing value.
func ProcessCrossChainTransaction(config *params.ChainConfig, number *big.Int, timestamp uint64, st *state.StateDB, tx *types.Transaction, sender common.Address) error {
	if config == nil || !config.IsAntartical(number, timestamp) {
		return antartical.ErrProfileInactive
	}
	if tx == nil || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Sign() != 0 || tx.ChainId().Cmp(config.ChainID) != 0 {
		return ErrCrossChainEnvelope
	}
	m, err := DecodeCrossChainMessage(tx.Data())
	if err != nil {
		return err
	}
	if m.Message.DestinationChainID == nil || m.Message.DestinationChainID.Cmp(config.ChainID) != 0 || m.Message.SourceChainID.Cmp(config.ChainID) == 0 || m.Signer != sender || m.Verify() != nil {
		return ErrCrossChainEnvelope
	}
	if len(m.Message.Payload) == 0 || len(m.Message.Payload) > MaxCrossChainPayload {
		return ErrCrossChainEnvelope
	}
	replay, err := m.Message.ReplayKey()
	if err != nil {
		return err
	}
	slot := crossChainReplaySlot(replay)
	if st.GetState(params.ShieldedPoolAddress, slot) != (common.Hash{}) {
		return ErrCrossChainReplay
	}
	st.SetState(params.ShieldedPoolAddress, slot, tx.Hash())
	st.SetState(params.ShieldedPoolAddress, crossChainPayloadLengthSlot(replay), uint64Hash(uint64(len(m.Message.Payload))))
	for index := 0; index*32 < len(m.Message.Payload); index++ {
		var word [32]byte
		copy(word[:], m.Message.Payload[index*32:])
		st.SetState(params.ShieldedPoolAddress, crossChainPayloadChunkSlot(replay, uint64(index)), common.Hash(word))
	}
	return nil
}
