package shield3wallet

import (
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type PaymentPayload struct {
	Version           uint64
	ChainID           uint64
	Address           common.Address
	Owner             shielded3.Digest
	IncomingPublicKey []byte
	StampPublicKey    []byte
	Stamp             pqcrypto.ShieldedV3StampRecord
}
type PaymentCode struct {
	Payload   PaymentPayload
	PublicKey []byte
	Signature []byte
}
type Identity struct {
	ChainID           uint64
	Address           common.Address
	NullifierKey      shielded3.Digest
	SpendingSecret    shielded3.Digest
	Owner             shielded3.Digest
	IncomingSeed      []byte
	OutgoingSeed      []byte
	StampSeed         []byte
	IncomingPublicKey []byte
	OutgoingPublicKey []byte
	StampPublicKey    []byte
	Stamp             *pqcrypto.ShieldedV3StampRecord
	Code              string
}

func (i *Identity) Clear() {
	if i == nil {
		return
	}
	clear(i.SpendingSecret[:])
	clear(i.NullifierKey[:])
	clear(i.IncomingSeed)
	clear(i.OutgoingSeed)
	clear(i.StampSeed)
}
func DeriveSpendingSecret(seed []byte, chainID uint64) (shielded3.Digest, error) {
	var d shielded3.Digest
	if len(seed) != pqcrypto.MLDSA87SeedSize || chainID == 0 {
		return d, pqcrypto.ErrInvalidPrivateKey
	}
	for i := range d {
		var salt [16]byte
		binary.BigEndian.PutUint64(salt[:8], chainID)
		binary.BigEndian.PutUint32(salt[8:12], uint32(i))
		for retry := uint32(0); ; retry++ {
			binary.BigEndian.PutUint32(salt[12:], retry)
			raw, err := hkdf.Key(sha512.New, seed, salt[:], "TKM_SHIELD3_SPENDING_SECRET_V1", 8)
			if err != nil {
				return d, err
			}
			word := binary.LittleEndian.Uint64(raw)
			clear(raw)
			if word < 0xffffffff00000001 {
				d[i] = word
				break
			}
		}
	}
	return d, nil
}
func paymentMessage(payload PaymentPayload) ([]byte, error) {
	data, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return nil, err
	}
	h := sha512.New()
	h.Write([]byte("TKM_SHIELD3_PAYMENT_CODE_V1"))
	h.Write(data)
	return h.Sum(nil), nil
}
func NewIdentity(seed []byte, chainID uint64, stamp *pqcrypto.ShieldedV3StampRecord) (*Identity, error) {
	identity := &Identity{ChainID: chainID, Stamp: stamp}
	fail := func(err error) (*Identity, error) { identity.Clear(); return nil, err }
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return fail(err)
	}
	pub := pqcrypto.PublicKeyBytes(key)
	identity.Address, err = pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	if err != nil {
		return fail(err)
	}
	if stamp == nil || stamp.ChainID != chainID || !pqcrypto.VerifyShieldedV3Stamp(pub, stamp) {
		return fail(errors.New("add a private name/country stamp to this account before using Shield3"))
	}
	identity.SpendingSecret, err = DeriveSpendingSecret(seed, chainID)
	if err != nil {
		return fail(err)
	}
	identity.Owner, err = shielded3.HashWords(append([]uint64{3001}, identity.SpendingSecret[:]...))
	if err != nil {
		return fail(err)
	}
	identity.NullifierKey, err = shielded3.HashWords(append([]uint64{3005}, identity.SpendingSecret[:]...))
	if err != nil {
		return fail(err)
	}

	for _, role := range []struct {
		purpose        pqcrypto.ShieldedV3Purpose
		secret, public *[]byte
	}{{pqcrypto.ShieldedV3Incoming, &identity.IncomingSeed, &identity.IncomingPublicKey}, {pqcrypto.ShieldedV3Outgoing, &identity.OutgoingSeed, &identity.OutgoingPublicKey}, {pqcrypto.ShieldedV3Stamp, &identity.StampSeed, &identity.StampPublicKey}} {
		*role.secret, err = pqcrypto.DeriveShieldedV3ViewKey(seed, chainID, role.purpose)
		if err != nil {
			return fail(err)
		}
		*role.public, err = pqcrypto.ShieldedV3ViewPublicKey(*role.secret)
		if err != nil {
			return fail(err)
		}
	}
	if _, err = pqcrypto.OpenShieldedV3Stamp(identity.StampSeed, stamp); err != nil {
		return fail(err)
	}
	payload := PaymentPayload{3, chainID, identity.Address, identity.Owner, identity.IncomingPublicKey, identity.StampPublicKey, *stamp}
	message, err := paymentMessage(payload)
	if err != nil {
		return fail(err)
	}
	signature, err := pqcrypto.SignMLDSA87(key, message)
	if err != nil {
		return fail(err)
	}
	data, err := rlp.EncodeToBytes(PaymentCode{payload, pub, signature})
	if err != nil {
		return fail(err)
	}
	identity.Code = "tkmshield3." + base64.RawURLEncoding.EncodeToString(data)
	return identity, nil
}
func DecodePaymentCode(value string, chainID uint64) (PaymentPayload, error) {
	fail := func() (PaymentPayload, error) {
		return PaymentPayload{}, errors.New("invalid or unauthenticated Shield3 receiving address")
	}
	const prefix = "tkmshield3."
	if len(value) > 65536 || len(value) <= len(prefix) || value[:len(prefix)] != prefix {
		return fail()
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(value[len(prefix):])
	if err != nil {
		return fail()
	}
	var code PaymentCode
	if err = rlp.DecodeBytes(data, &code); err != nil {
		return fail()
	}
	p := code.Payload
	if p.Version != 3 || p.ChainID != chainID || chainID == 0 || p.Owner == (shielded3.Digest{}) || p.Stamp.ChainID != chainID {
		return fail()
	}
	if _, err = shielded3.DigestFromBytes(p.Owner.Bytes()); err != nil {
		return fail()
	}
	for _, pub := range [][]byte{p.IncomingPublicKey, p.StampPublicKey} {
		if _, err = mlkem.NewEncapsulationKey1024(pub); err != nil {
			return fail()
		}
	}
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, code.PublicKey)
	if err != nil || address != p.Address || !pqcrypto.VerifyShieldedV3Stamp(code.PublicKey, &p.Stamp) {
		return fail()
	}
	message, err := paymentMessage(p)
	if err != nil || !pqcrypto.VerifyMLDSA87(code.PublicKey, message, code.Signature) {
		return fail()
	}
	return p, nil
}
func parseAmount(value string) (*big.Int, error) {
	if len(value) == 0 || len(value) > 78 {
		return nil, shielded3.ErrInvalidStatement
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return nil, shielded3.ErrInvalidStatement
		}
	}
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok || amount.BitLen() > 256 {
		return nil, shielded3.ErrInvalidStatement
	}
	return amount, nil
}
