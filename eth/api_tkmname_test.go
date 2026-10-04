package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/internal/shield3wallet"
)

func TestTkmNameAPIRequiresCompleteFixedBatch(t *testing.T) {
	directory, err := shield3wallet.NewUsernameDirectory(8979)
	if err != nil {
		t.Fatal(err)
	}
	api := &TkmNameAPI{directory: directory}
	if _, err := api.LookupBatch([]string{"alice"}); err == nil {
		t.Fatal("accepted a short lookup batch")
	}
	names := make([]string, shield3wallet.UsernameLookupBatchLen)
	for i := range names {
		names[i] = "cover" + string(rune('a'+i))
	}
	if _, err := api.LookupBatch(names); err == nil {
		t.Fatal("returned a partial batch containing missing records")
	}
}
