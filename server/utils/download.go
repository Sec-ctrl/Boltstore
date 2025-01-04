package utils

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/quic-go/quic-go"
)

// handleFileDownload handles the file download operation by opening a new stream
// and sending the file data to the client.
func HandleFileDownload(ctx context.Context, conn quic.Connection, metadata *Metadata) error {
	ctxDown, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()

	log.Printf("Starting file download: %s", metadata.FileName)

	// Try to open the file to be downloaded
	file, err := os.Open(metadata.FileName)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", metadata.FileName, err)
	}
	defer file.Close()

	// The server can open a new stream to send data (or the client could open it;
	// choose a consistent protocol approach).
	dataStream, err := conn.OpenStreamSync(ctxDown)
	if err != nil {
		return fmt.Errorf("failed to open download data stream: %w", err)
	}
	defer dataStream.Close()

	// Copy the file contents to the stream
	sent, err := io.Copy(dataStream, file)
	if err != nil {
		return fmt.Errorf("failed sending file %s: %w", metadata.FileName, err)
	}

	log.Printf("Download complete for file %s (%d bytes sent)", metadata.FileName, sent)
	return nil
}
