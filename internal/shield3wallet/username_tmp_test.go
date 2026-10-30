package shield3wallet

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/binary"
 "testing"
)

func TestPrintTmpUsernameHandle(t *testing.T) {
 var chain [8]byte
 binary.BigEndian.PutUint64(chain[:], 8979)
 h := sha256.New(); h.Write([]byte("TKM_SHIELD3_USERNAME_HANDLE_V1")); h.Write(chain[:]); h.Write([]byte("laugh26me"))
 t.Log("digest", hex.EncodeToString(h.Sum(nil)[:4]))
 got, _ := UsernameHandle("laugh26me", 8979); t.Log("handle", got)
}
