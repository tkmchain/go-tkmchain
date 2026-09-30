package antartical

import "math/big"

// DefaultBlocksPerHalving is the four-year halving interval used by the
// RandomX schedule. Keeping the interval in the Antartical package lets the
// validator reward verifier use exactly the same integer schedule without a
// dependency cycle through the RandomX engine.
const DefaultBlocksPerHalving uint64 = 4 * 365 * 24 * 60 * 60 / 120

// InitialRewardShares are the Antartical fixed per-block shares. The
// validator share belongs to exactly one selected validator at a height; it
// is not multiplied by committee size.
var InitialRewardShares = RewardShares{
	Miner:     new(big.Int).Mul(big.NewInt(90), big.NewInt(1e18)),
	Validator: new(big.Int).Mul(big.NewInt(70), big.NewInt(1e18)),
	Rotating:  new(big.Int).Mul(big.NewInt(35), big.NewInt(1e18)),
	MainKing:  new(big.Int).Mul(big.NewInt(5), big.NewInt(1e18)),
}

type RewardShares struct {
	Miner     *big.Int
	Validator *big.Int
	Rotating  *big.Int
	MainKing  *big.Int
}

func (r RewardShares) Total() *big.Int {
	total := new(big.Int)
	for _, share := range []*big.Int{r.Miner, r.Validator, r.Rotating, r.MainKing} {
		if share != nil {
			total.Add(total, share)
		}
	}
	return total
}

// RewardSharesAt applies the existing RandomX halving interval to every
// Antartical share. Integer arithmetic is performed in wei, so fractional TKM
// values such as 2.5 TKM remain exact.
func RewardSharesAt(blockNumber, blocksPerHalving uint64) RewardShares {
	shares := RewardShares{
		Miner: new(big.Int).Set(InitialRewardShares.Miner), Validator: new(big.Int).Set(InitialRewardShares.Validator),
		Rotating: new(big.Int).Set(InitialRewardShares.Rotating), MainKing: new(big.Int).Set(InitialRewardShares.MainKing),
	}
	if blocksPerHalving == 0 {
		return shares
	}
	for i := blockNumber / blocksPerHalving; i > 0; i-- {
		for _, share := range []*big.Int{shares.Miner, shares.Validator, shares.Rotating, shares.MainKing} {
			share.Div(share, big.NewInt(2))
		}
	}
	return shares
}
