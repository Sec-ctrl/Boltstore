package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/quic-go/quic-go"

	// Adjust these imports to your actual paths
	"github.com/Sec-ctr/Boltstore/utils"
)

// uploadFile dials the QUIC server at serverAddr, sends "upload" metadata,
// and then uploads the file contents to the server.
func uploadFile(serverAddr, filePath string) error {
	// 1. Prepare TLS config for client
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,           // For testing only; do NOT use in production.
		NextProtos:         []string{"h3"}, // or your chosen ALPN, must match server
	}

	// 2. Prepare QUIC config (you can customize as needed)
	quicConfig := &quic.Config{
		// Adjust your config if necessary
		KeepAlive: true,
	}

	// 3. Dial the QUIC server
	fmt.Printf("Connecting to server at %s ...\n", serverAddr)
	conn, err := quic.DialAddr(context.Background(), serverAddr, tlsConfig, quicConfig)
	if err != nil {
		return fmt.Errorf("failed to dial server: %w", err)
	}
	defer conn.CloseWithError(0, "client done")

	// 4. Prepare and send metadata (Operation=1 means upload)
	metadata := &utils.Metadata{
		Operation: 1,        // 1 = Upload
		FileName:  filePath, // The name you want the server to store
		FileSize:  0,        // If you want to pass size, do so
		// Add any other fields your Metadata struct requires
	}

	// 5. Open the “metadata stream” and send the metadata
	metaStream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return fmt.Errorf("failed to open metadata stream: %w", err)
	}
	// Once finished writing, close the metadata stream (server will read it)
	defer metaStream.Close()

	if err := utils.EncodeMetadata(metaStream, metadata); err != nil {
		return fmt.Errorf("failed to encode metadata: %w", err)
	}

	log.Printf("Metadata sent for file: %s", metadata.FileName)

	// 6. Give server some time to parse metadata before sending file data
	//    (Optional small sleep; or you can wait for server ack on metaStream, etc.)
	time.Sleep(200 * time.Millisecond)

	// 7. Open a second stream for the file data
	dataStream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return fmt.Errorf("failed to open data stream: %w", err)
	}
	defer dataStream.Close()

	// 8. Send the file contents
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	log.Printf("Uploading file data for: %s", filePath)
	if _, err := io.Copy(dataStream, file); err != nil {
		return fmt.Errorf("failed to send file data: %w", err)
	}

	log.Printf("Finished uploading file: %s", filePath)
	return nil
}

// Example usage:
//
//	go run client.go /path/to/myfile.txt
func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <file-to-upload>\n", os.Args[0])
		os.Exit(1)
	}

	filePath := os.Args[1]
	serverAddr := "localhost:4242" // or the address/port you used for the server

	if err := uploadFile(serverAddr, filePath); err != nil {
		log.Fatalf("Upload failed: %v", err)
	}

	fmt.Println("Upload succeeded!")
}
