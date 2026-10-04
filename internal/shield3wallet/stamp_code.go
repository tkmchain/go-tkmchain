package shield3wallet

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
)

const StampSponsorshipCodePrefix = "tkmstamp1."

// Codes carry only the public transaction. Display fields are reconstructed
// from it; importing a code never trusts externally supplied fee/address labels.
const MaxStampSponsorshipCodeSize = len(StampSponsorshipCodePrefix) + (int(core.ShieldedV3MaxTxSize)+2)/3*4

func EncodeStampSponsorshipCode(packet StampSponsorship) (string, error) {
	var tx types.Transaction
	if uint64(len(packet.Transaction)) > core.ShieldedV3MaxTxSize || tx.UnmarshalBinary(packet.Transaction) != nil || !tx.ChainId().IsUint64() {
		return "", errors.New("invalid stamp sponsorship transaction")
	}
	code := StampSponsorshipCodePrefix + base64.RawURLEncoding.EncodeToString(packet.Transaction)
	if _, err := DecodeStampSponsorshipCode(code, tx.ChainId().Uint64()); err != nil {
		return "", err
	}
	return code, nil
}

// DecodeStampSponsorshipCode checks the format and transaction constraints.
// Live state and authorization checks still run before proving or signing.
func DecodeStampSponsorshipCode(code string, chainID uint64) (StampSponsorship, error) {
	if len(code) > MaxStampSponsorshipCodeSize || !strings.HasPrefix(code, StampSponsorshipCodePrefix) {
		return StampSponsorship{}, errors.New("expected a tkmstamp1 sponsorship code")
	}
	encoded := strings.TrimPrefix(code, StampSponsorshipCodePrefix)
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != encoded {
		return StampSponsorship{}, errors.New("invalid sponsorship code encoding")
	}
	tx, _, _, err := stampSponsorshipTransaction(raw, chainID)
	if err != nil {
		return StampSponsorship{}, err
	}
	return stampSponsorshipPacket(tx)
}
