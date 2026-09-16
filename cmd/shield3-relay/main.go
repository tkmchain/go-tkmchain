// shield3-relay runs a stamped shared operator. TLS is normally terminated by
// a reverse proxy; the default HTTP listener is loopback only.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/internal/shield3relay"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func run() error {
	endpoint := flag.String("rpc", "http://127.0.0.1:8545", "local daemon RPC or IPC endpoint")
	listen := flag.String("listen", "127.0.0.1:8790", "HTTP listener; use a TLS reverse proxy for remote wallets")
	wallet := flag.String("keystore", "", "encrypted stamped PQ keystore JSON")
	password := flag.String("password-file", "", "operator-only file containing keystore password")
	state := flag.String("state-dir", "", "durable relay state directory")
	flag.Parse()
	if !shielded3.NativeAvailable() {
		return errors.New("build with -tags shield3 and the embedded native verifier")
	}
	if *wallet == "" || *password == "" || *state == "" {
		return errors.New("keystore, password-file and state-dir are required")
	}
	blob, err := os.ReadFile(*wallet)
	if err != nil {
		return err
	}
	secret, err := os.ReadFile(*password)
	if err != nil {
		return err
	}
	defer clear(secret)
	key, err := keystore.DecryptPQKey(blob, strings.TrimRight(string(secret), "\r\n"))
	if err != nil {
		return errors.New("cannot unlock stamped operator keystore")
	}
	defer clear(key.Seed)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client, err := rpc.DialContext(ctx, *endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	var chain hexutil.Big
	if err := client.CallContext(ctx, &chain, "eth_chainId"); err != nil {
		return err
	}
	if !(*big.Int)(&chain).IsUint64() {
		return errors.New("invalid daemon chain ID")
	}
	identity, err := shield3wallet.NewIdentity(key.Seed, (*big.Int)(&chain).Uint64(), key.Shield3Stamp)
	if err != nil {
		return err
	}
	defer identity.Clear()
	service, err := shield3relay.New(client, key.Seed, identity, *state)
	if err != nil {
		return err
	}
	defer service.Close()
	server := &http.Server{Addr: *listen, Handler: service, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	go func() {
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		server.Shutdown(shutdown)
	}()
	fmt.Printf("Shield3 relay operator %s listening on %s\n", identity.Address, *listen)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
