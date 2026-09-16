package core

import (
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

// Every slot uses a separate V3 domain. A full digest occupies two storage
// words; the legacy field/tree state is never read by the V3 note tree.
func ShieldedV3StateSlot(role string, data []byte) common.Hash {
	h := sha512.New()
	h.Write([]byte("TKM_SHIELD3_STATE_V1/" + role + "/"))
	h.Write(data)
	return common.BytesToHash(h.Sum(nil))
}
func v3ReadDigest(st shieldedStateReader, role string, data []byte) (shielded3.Digest, error) {
	head := st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot(role+"/0", data))
	tail := st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot(role+"/1", data))
	for _, v := range tail[:24] {
		if v != 0 {
			return shielded3.Digest{}, ErrInvalidShieldedTx
		}
	}
	raw := append(head.Bytes(), tail[24:]...)
	return shielded3.DigestFromBytes(raw)
}
func v3WriteDigest(st shieldedStateWriter, role string, key []byte, d shielded3.Digest) {
	raw := d.Bytes()
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot(role+"/0", key), common.BytesToHash(raw[:32]))
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot(role+"/1", key), common.BytesToHash(raw[32:]))
}
func ShieldedV3NextIndex(st shieldedStateReader) uint64 {
	return uint64FromHash(st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("next-index", nil)))
}
func ShieldedV3NullifierTransaction(st shieldedStateReader, n shielded3.Digest) common.Hash {
	return st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("nullifier", n.Bytes()))
}

var v3ZeroCache struct {
	once    sync.Once
	digests [shielded3.MerkleDepth + 1]shielded3.Digest
	err     error
}

func v3Zeroes() ([shielded3.MerkleDepth + 1]shielded3.Digest, error) {
	v3ZeroCache.once.Do(func() {
		for i := 1; i < len(v3ZeroCache.digests); i++ {
			v3ZeroCache.digests[i], v3ZeroCache.err = shielded3.HashPair(v3ZeroCache.digests[i-1], v3ZeroCache.digests[i-1])
			if v3ZeroCache.err != nil {
				return
			}
		}
	})
	return v3ZeroCache.digests, v3ZeroCache.err
}
func v3NodeKey(level int, index uint64) []byte {
	key := make([]byte, 12)
	binary.BigEndian.PutUint32(key, uint32(level))
	binary.BigEndian.PutUint64(key[4:], index)
	return key
}
func v3Node(st shieldedStateReader, level int, index uint64, zeroes [shielded3.MerkleDepth + 1]shielded3.Digest) (shielded3.Digest, error) {
	key := v3NodeKey(level, index)
	if st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("node-present", key)) == (common.Hash{}) {
		return zeroes[level], nil
	}
	return v3ReadDigest(st, "node", key)
}
func appendShieldedV3Leaf(st shieldedStateWriter, commitment shielded3.Digest, txHash common.Hash) error {
	index := ShieldedV3NextIndex(st)
	if index >= uint64(1)<<shielded3.MerkleDepth {
		return fmt.Errorf("%w: Shield3 tree full", ErrInvalidShieldedTx)
	}
	zeroes, err := v3Zeroes()
	if err != nil {
		return err
	}
	current := commitment
	cursor := index
	for level := 0; level <= shielded3.MerkleDepth; level++ {
		key := v3NodeKey(level, cursor)
		v3WriteDigest(st, "node", key, current)
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("node-present", key), uint64Hash(1))
		if level == shielded3.MerkleDepth {
			break
		}
		sibling, err := v3Node(st, level, cursor^1, zeroes)
		if err != nil {
			return err
		}
		if cursor&1 == 0 {
			current, err = shielded3.HashPair(current, sibling)
		} else {
			current, err = shielded3.HashPair(sibling, current)
		}
		if err != nil {
			return err
		}
		cursor >>= 1
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("commitment-index", commitment.Bytes()), uint64Hash(index+1))
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("next-index", nil), uint64Hash(index+1))
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("root", current.Bytes()), txHash)
	v3WriteDigest(st, "current-root", nil, current)
	return nil
}

type ShieldedV3Path struct {
	Commitment shielded3.Digest                        `json:"commitment"`
	Index      uint64                                  `json:"index"`
	Root       shielded3.Digest                        `json:"root"`
	Path       [shielded3.MerkleDepth]shielded3.Digest `json:"path"`
	Found      bool                                    `json:"found"`
}

func ShieldedV3CommitmentPath(st shieldedStateReader, commitment shielded3.Digest) (ShieldedV3Path, error) {
	result := ShieldedV3Path{Commitment: commitment}
	index := uint64FromHash(st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("commitment-index", commitment.Bytes())))
	if index == 0 {
		return result, nil
	}
	result.Found = true
	result.Index = index - 1
	zeroes, err := v3Zeroes()
	if err != nil {
		return result, err
	}
	cursor := result.Index
	for i := range result.Path {
		result.Path[i], err = v3Node(st, i, cursor^1, zeroes)
		if err != nil {
			return result, err
		}
		cursor >>= 1
	}
	result.Root, err = v3ReadDigest(st, "current-root", nil)
	return result, err
}
func ShieldedV3Root(st shieldedStateReader) (shielded3.Digest, error) {
	if ShieldedV3NextIndex(st) == 0 {
		zeroes, err := v3Zeroes()
		return zeroes[shielded3.MerkleDepth], err
	}
	return v3ReadDigest(st, "current-root", nil)
}
