package main

import (
	"context"
	"log"

	"github.com/Sec-ctr/Boltstore/network"
)

const addr = ":4242"

func main() {
	tlsConfig := network.GenerateTLSConfig()
	quicConfig := network.GenerateQuicConfig()

	ctx := context.Background()

	concurrencyLimit := 10

	if err := network.StartQuicServer(addr, ctx, tlsConfig, quicConfig, concurrencyLimit); err != nil {
		log.Fatalf("Failed to start Quic server: %+v", err)
	}

}
