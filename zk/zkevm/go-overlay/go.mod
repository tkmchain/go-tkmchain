module github.com/ethereum/go-ethereum/zk/zkevm/go-overlay

go 1.26

// The files in this directory are copied into a temporary Go toolchain by
// zk/zkevm/build_keeper.sh. They intentionally contain different standard
// library package names (unix, os, and runtime), so keeping this directory as
// a nested module prevents the root test command from treating the overlay as
// an ordinary Go package.
