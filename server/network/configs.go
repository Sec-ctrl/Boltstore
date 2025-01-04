package network

import (
	"crypto/tls"
	"log"

	"github.com/quic-go/quic-go"
)

func GenerateTLSConfig() *tls.Config {
	cert, err := tls.LoadX509KeyPair("certs/server.crt", "certs/server.key")
	if err != nil {
		log.Fatalf("Failed to load key pair: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"quic-file-transfer"},
	}
}

func GenerateQuicConfig() *quic.Config {
	tokenStore := quic.NewLRUTokenStore(5, 1)
	return &quic.Config{
		TokenStore: tokenStore,
		Allow0RTT:  false,
	}
}
