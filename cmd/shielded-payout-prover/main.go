package main

import (
	"flag"
	"log"

	"github.com/ethereum/go-ethereum/internal/shieldedpayoutprover"
)

func main() {
	configPath := flag.String("config", "/home/mike/shielded-prover/config.json", "path to prover config")
	flag.Parse()

	cfg, err := shieldedpayoutprover.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	prover, err := shieldedpayoutprover.NewProver(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := prover.Serve(); err != nil {
		log.Fatal(err)
	}
}
