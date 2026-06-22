//go:build ignore

package main

import (
        "encoding/json"
        "fmt"
        "math/big"
        "os"
        "slices"
        "strconv"

        "github.com/ethereum/go-ethereum/common"
        "github.com/ethereum/go-ethereum/rlp"
)

// BigInt is a custom type that can unmarshal both string and number values
type BigInt struct{ *big.Int }

func (b *BigInt) UnmarshalJSON(data []byte) error {
        if b.Int == nil {
                b.Int = new(big.Int)
        }
        // Try to unmarshal as string first
        var s string
        if err := json.Unmarshal(data, &s); err == nil {
                // It's a string - parse it
                if len(s) >= 2 && s[:2] == "0x" {
                        b.Int.SetString(s[2:], 16)
                } else {
                        b.Int.SetString(s, 10)
                }
                return nil
        }
        // Try to unmarshal as number
        var n json.Number
        if err := json.Unmarshal(data, &n); err == nil {
                b.Int.SetString(string(n), 10)
                return nil
        }
        return fmt.Errorf("cannot unmarshal %s into *big.Int", string(data))
}

type GenesisAccount struct {
        Code    []byte                      `json:"code,omitempty"`
        Storage map[common.Hash]common.Hash `json:"storage,omitempty"`
        Balance *BigInt                     `json:"balance,omitempty"`
        Nonce   uint64                      `json:"nonce,omitempty"`
}

type Genesis struct {
        Config       interface{}                `json:"config"`
        Nonce        uint64                     `json:"nonce"`
        Timestamp    uint64                     `json:"timestamp"`
        ExtraData    string                     `json:"extraData"`
        GasLimit     uint64                     `json:"gasLimit"`
        Difficulty   *BigInt                    `json:"difficulty"`
        Mixhash      common.Hash                `json:"mixHash"`
        Coinbase     common.Address             `json:"coinbase"`
        Alloc        map[common.Address]GenesisAccount `json:"alloc"`
        Number       uint64                     `json:"number"`
        GasUsed      uint64                     `json:"gasUsed"`
        ParentHash   common.Hash                `json:"parentHash"`
        BaseFee      *BigInt                    `json:"baseFeePerGas"`
        BlobGasUsed  *uint64                    `json:"blobGasUsed"`
        ExcessBlobGas *uint64                   `json:"excessBlobGas"`
}

type allocItem struct {
        Addr    *big.Int
        Balance *big.Int
        Misc    *allocItemMisc `rlp:"optional"`
}

type allocItemMisc struct {
        Nonce uint64
        Code  []byte
        Slots []allocItemStorageItem
}

type allocItemStorageItem struct {
        Key common.Hash
        Val common.Hash
}

func makelist(g *Genesis) []allocItem {
        items := make([]allocItem, 0, len(g.Alloc))
        for addr, account := range g.Alloc {
                var misc *allocItemMisc
                if len(account.Storage) > 0 || len(account.Code) > 0 || account.Nonce != 0 {
                        misc = &allocItemMisc{
                                Nonce: account.Nonce,
                                Code:  account.Code,
                                Slots: make([]allocItemStorageItem, 0, len(account.Storage)),
                        }
                        for key, val := range account.Storage {
                                misc.Slots = append(misc.Slots, allocItemStorageItem{key, val})
                        }
                        slices.SortFunc(misc.Slots, func(a, b allocItemStorageItem) int {
                                return a.Key.Cmp(b.Key)
                        })
                }
                bigAddr := new(big.Int).SetBytes(addr.Bytes())
                var balance *big.Int
                if account.Balance != nil && account.Balance.Int != nil {
                        balance = account.Balance.Int
                } else {
                        balance = big.NewInt(0)
                }
                items = append(items, allocItem{bigAddr, balance, misc})
        }
        slices.SortFunc(items, func(a, b allocItem) int {
                return a.Addr.Cmp(b.Addr)
        })
        return items
}

func makealloc(g *Genesis) string {
        a := makelist(g)
        data, err := rlp.EncodeToBytes(a)
        if err != nil {
                panic(err)
        }
        return strconv.QuoteToASCII(string(data))
}

func main() {
        if len(os.Args) != 2 {
                fmt.Fprintln(os.Stderr, "Usage: mkalloc genesis.json")
                os.Exit(1)
        }

        g := new(Genesis)
        file, err := os.Open(os.Args[1])
        if err != nil {
                panic(err)
        }
        defer file.Close()
        if err := json.NewDecoder(file).Decode(g); err != nil {
                panic(err)
        }
        fmt.Println("const allocData =", makealloc(g))
}
