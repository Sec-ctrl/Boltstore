package network

import (
	"crypto/tls"
	"log"

	"github.com/quic-go/quic-go"
)

func GenerateTLSConfig() *tls.Config {
	cert, err := tls.LoadX509KeyPair("certs/client.crt", "certs/client.key")
	if err != nil {
		log.Fatalf("Failed to load key pair: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"quic-file-transfer"},
	}
}

func GenerateQuicConfig() *quic.Config {
	return &quic.Config{
		Allow0RTT: false,
	}
}
