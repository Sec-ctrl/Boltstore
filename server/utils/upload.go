package utils

import (
	"context" // Example: you might replace with a faster or stronger library
	"fmt"
	"io"
	"log"
	"os"

	"github.com/quic-go/quic-go"
)

// HandleFileUpload handles the file upload operation over a second stream.
func HandleFileUpload(ctx context.Context, conn quic.Connection, metadata *Metadata) (err error) {
	// If you want indefinite upload with manual cancel, this is fine:
	ctxUp, cancel := context.WithCancel(ctx)
	defer cancel()

	log.Printf("Starting file upload: %s (size: %d bytes)", metadata.FileName, metadata.FileSize)

	// Accept the data stream from the client
	dataStream, err := conn.AcceptStream(ctxUp)
	if err != nil {
		// Check if context was canceled or timed out
		select {
		case <-ctxUp.Done():
			log.Printf("[WARN] Upload timed out or canceled for %s", metadata.FileName)
			return conn.CloseWithError(3, "upload data timeout or canceled")
		default:
		}
		return fmt.Errorf("failed to accept upload data stream: %w", err)
	}
	defer dataStream.Close()

	// Preallocate file if possible (platform-specific). For example:
	//   err = preallocate(tempFileName, metadata.FileSize)
	//   if err != nil { ... }

	// Create temporary file
	tempFileName := metadata.FileName + ".uploading"
	file, err := os.Create(tempFileName)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", tempFileName, err)
	}
	// Make sure we clean up on error
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(tempFileName)
		}
	}()

	// Optional: Wrap dataStream to count bytes as we copy.
	// This allows progress-logging at intervals without manual chunking.
	countingReader := &CountingReader{
		Reader: dataStream,
		total:  0,
		size:   metadata.FileSize,
		fileID: metadata.FileName,
	}

	// If we want to do a *non-blocking* checksum, we can read the data twice:
	//   1) copy from dataStream -> file
	//   2) simultaneously compute the hash
	// But that requires splitting or tee-ing the stream. For maximum speed, the
	// simplest approach is to write everything to disk, then compute the hash
	// from disk. This means less concurrency, but simpler logic.

	// Copy exactly metadata.FileSize bytes from the stream to the file:
	// (If file size is untrusted, you might read more or validate differently.)
	written, err := io.CopyN(file, countingReader, int64(metadata.FileSize))
	if err != nil {
		if err == io.EOF {
			// If we got EOF early, we can handle incomplete uploads.
			return fmt.Errorf("upload incomplete: expected %d bytes, got %d bytes", metadata.FileSize, written)
		}
		return fmt.Errorf("failed to copy data to file: %w", err)
	}

	// Check if there's extra data beyond metadata.FileSize
	// If the client is sending more data than advertised, it's an error.
	extraBuf := make([]byte, 1)
	n, err := dataStream.Read(extraBuf)
	if n > 0 || err == nil {
		return fmt.Errorf("file size exceeded: client sent more data than expected")
	}
	if err != nil && err != io.EOF {
		// Some non-EOF error occurred
		return fmt.Errorf("error reading post-data: %w", err)
	}

	// If we made it this far, we have exactly the correct file size on disk.
	// Now compute the MD5 (or use another hash).
	// For huge files, consider streaming the file data. This is simpler:
	sum, err := FileMD5Checksum(tempFileName)
	if err != nil {
		return fmt.Errorf("failed to compute checksum for file %s: %w", tempFileName, err)
	}
	if sum != string(metadata.Checksum) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", metadata.Checksum, sum)
	}

	// Rename to finalize
	if err := os.Rename(tempFileName, metadata.FileName); err != nil {
		return fmt.Errorf("failed to finalize file %s: %w", metadata.FileName, err)
	}

	log.Printf("Upload complete for file %s (%d bytes, checksum validated)", metadata.FileName, written)
	return nil
}

// CountingReader allows us to log progress without a manual read loop.
type CountingReader struct {
	Reader io.Reader
	total  uint64
	size   uint64
	fileID string
}

// Read implements io.Reader and logs progress every 10 MB or at completion.
func (c *CountingReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	if n > 0 {
		c.total += uint64(n)
		// Log every 1000MB or on completion
		if (c.total%(1000*1024*1024) == 0) || c.total == c.size {
			log.Printf("File upload progress for %s: %d bytes written", c.fileID, c.total)
		}
	}
	return n, err
}
