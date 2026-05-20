

package randomx

import (
    "os"
    "strconv"
)

func init() {
    // Check for RAM cache environment variable
    if val := os.Getenv("RANDOMX_RAM_CACHE"); val == "true" || val == "1" {
        DefaultRAMCacheEnabled = true
    }
    
    // Check for custom RAM size
    if val := os.Getenv("RANDOMX_RAM_SIZE_GB"); val != "" {
        if size, err := strconv.ParseUint(val, 10, 64); err == nil {
            DefaultRAMCacheSizeGB = size
        }
    }
}

var (
    DefaultRAMCacheEnabled = false
    DefaultRAMCacheSizeGB  = uint64(2)
)
