package network

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"BS-client/utils"

	"github.com/quic-go/quic-go"
)

// uploadLogic orchestrates opening the file, sending metadata, then sending the file itself.
func UploadLogic(uploadFile, uploadDest *string) error {
	// 1. Validate the file path
	if *uploadFile == "" {
		return fmt.Errorf("file path is required. Use -file flag")
	}

	// 2. Get file info
	fileInfo, err := os.Stat(*uploadFile)
	if err != nil {
		return fmt.Errorf("failed to stat file: %v", err)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("cannot send directories")
	}

	fileSize := fileInfo.Size()
	fileName := fileInfo.Name()

	// 3. Compute checksum (MD5)
	checksum, err := utils.ComputeChecksum(*uploadFile)
	if err != nil {
		return fmt.Errorf("failed to compute checksum: %v", err)
	}

	// 4. Build metadata
	metadata := &utils.Metadata{
		Operation:    1, // 1 = Upload
		FileName:     fileName,
		FileSize:     uint64(fileSize),
		ChunkSize:    1024 * 1024, // 1MB (initial or nominal chunk size)
		Timestamp:    uint64(time.Now().Unix()),
		ChecksumType: 0, // 0 = MD5
		Checksum:     checksum,
	}

	// Validate metadata
	if !utils.IsValidMetadata(metadata) {
		log.Fatalf("Metadata did not pass local validation. Aborting.")
	}

	// Dial the server via QUIC
	ctx := context.Background()
	conn, err := quic.DialAddr(ctx, *uploadDest, GenerateTLSConfig(), GenerateQuicConfig())
	if err != nil {
		return fmt.Errorf("failed to dial server: %v", err)
	}
	defer conn.CloseWithError(0, "closing connection")

	// Send metadata on its own stream
	metadataStream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("failed to open metadata stream: %v", err)
	}
	if err := utils.EncodeAndSendMetadata(metadataStream, metadata); err != nil {
		return fmt.Errorf("failed to send metadata: %v", err)
	}
	metadataStream.Close()

	// Open the file once for upload
	file, err := os.Open(*uploadFile)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Initialize your BandwidthTracer.
	tracer := NewBandwidthTracer() // your logic here

	// Send the file with dynamic chunk sizes
	if err := UploadFileParallel(ctx, conn, file, tracer, 8); err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	// Signal completion of this file transfer
	log.Printf("File %s uploaded successfully", fileName)
	return nil
}
