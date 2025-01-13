package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"

	"github.com/quic-go/quic-go"
)

// HandleFileUploadParallel handles the file upload operation over multiple streams.
func HandleFileUploadParallel(ctx context.Context, conn quic.Connection, metadata *Metadata) (err error) {
	log.Printf("Entering HandleFileUploadParallel...")
	// Validate metadata
	if metadata.FileSize == 0 {
		log.Printf("[Error] File size is 0 or not provided")
		return fmt.Errorf("file size is 0 or not provided")
	}

	fileName := metadata.FileName
	tempFileName := fileName + ".uploading"
	log.Printf("Temp file name: %s", tempFileName)

	// Create or truncate the temporary file
	file, err := os.OpenFile(tempFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		log.Printf("[Error] Failed to create file %s: %v", tempFileName, err)
		return fmt.Errorf("failed to create file %s: %w", tempFileName, err)
	}
	log.Printf("Created temp file: %s", tempFileName)

	// For safety, remove the temp file on any error
	defer func() {
		file.Close()
		if err != nil {
			log.Printf("[Debug] Removing temp file %s due to error", tempFileName)
			os.Remove(tempFileName)
		}
	}()

	// Optionally preallocate space for large files (platform-dependent).
	// Example (Linux/macOS):
	// _ = preallocateFile(file, int64(metadata.FileSize))

	log.Printf("[Server] Starting parallel file upload: %s (%d bytes)", fileName, metadata.FileSize)

	// totalReceived will track the total number of bytes we have written to disk
	var totalReceived uint64 = 0

	// We'll use a wait group to track the chunk-handling goroutines.
	var wg sync.WaitGroup

	// We also use a channel to capture any fatal error from chunk goroutines
	// so we can propagate it and signal cancellation.
	errChan := make(chan error, 1) // buffered to avoid goroutine leaks

	// cancelCtx will let us stop accepting new streams if any chunk fails
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	// 1) Start a goroutine that accepts streams in a loop
	go func() {
		for {
			log.Printf("Accepted new stream for chunk...")
			stream, acceptErr := conn.AcceptStream(cancelCtx)
			if acceptErr != nil {
				// It's normal to get an error once the context is canceled
				// or the connection is closed. We break out here.
				log.Printf("[Info] Context canceled, stopping stream acceptance.")
				if errors.Is(acceptErr, context.Canceled) {
					return
				}
				// If it's some other error, log it.
				// Usually indicates the client is done or disconnected.
				log.Printf("[Server] AcceptStream error: %v", acceptErr)
				return
			}

			// For each new stream, spin up a goroutine to handle that chunk
			wg.Add(1)
			go func(str quic.Stream) {
				defer wg.Done()
				log.Printf("Handling new chunk...")

				if chunkErr := handleOneChunk(str, file, metadata, &totalReceived); chunkErr != nil {
					// Send error to the channel if it's not already closed
					select {
					case errChan <- chunkErr:
						// also cancel the overall context
						log.Printf("[Server] Chunk error: %v", chunkErr)
						cancelFunc()
					default:
					}
				}
			}(stream)
		}
	}()

	// 2) Wait for either:
	//    - all chunks to finish (wg.Wait)
	//    - or an error from a chunk (errChan)
	//    - or context cancellation
	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case <-doneChan:
		// All chunk goroutines have finished. Possibly we have the entire file.
		log.Printf("[Server] All chunks finished, waiting for validation...")
	case e := <-errChan:
		// A chunk had an error
		err = e
		log.Printf("[Error] Chunk error: %v", err)
		return
	case <-ctx.Done():
		// Overall context canceled or timed out
		err = ctx.Err()
		log.Printf("[Error] Context canceled: %v", err)
		return
	}

	// 3) If we got here, the chunk goroutines ended.
	// Check if totalReceived == metadata.FileSize
	if atomic.LoadUint64(&totalReceived) != metadata.FileSize {
		return fmt.Errorf("upload incomplete: expected %d bytes, got %d bytes",
			metadata.FileSize, totalReceived)
	}

	// 4) Compute the checksum, compare with metadata.Checksum
	sum, csErr := FileMD5Checksum(tempFileName)
	if csErr != nil {
		return fmt.Errorf("failed to compute checksum for file %s: %w", tempFileName, csErr)
	}
	if sum != string(metadata.Checksum) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", metadata.Checksum, sum)
	}

	// 5) Rename file to finalize
	if renameErr := os.Rename(tempFileName, fileName); renameErr != nil {
		return fmt.Errorf("failed to finalize file %s: %w", fileName, renameErr)
	}

	log.Printf("[Server] Upload complete for file %s (%d bytes, checksum validated)", fileName, metadata.FileSize)
	return nil
}

// handleOneChunk reads the chunk offset + length header, then reads that many bytes
// and writes them to the correct offset in the file.
// Also updates totalReceived (thread-safe).
func handleOneChunk(
	stream quic.Stream,
	file *os.File,
	metadata *Metadata,
	totalReceived *uint64,
) error {
	defer stream.Close()
	log.Printf("Handling new stream for chunk...")
	// 1) Read the chunk header
	hdr, err := DecodeChunkHeader(stream)
	if err != nil {
		log.Printf("failed to decode chunk header: %v", err)
		return fmt.Errorf("failed to decode chunk header: %w", err)
	}

	// Validate offset + chunk size
	if hdr.Offset+hdr.ChunkSize > metadata.FileSize {
		log.Printf("chunk out of file bounds: offset=%d chunkSize=%d (fileSize=%d)", hdr.Offset, hdr.ChunkSize, metadata.FileSize)
		return fmt.Errorf("chunk out of file bounds: offset=%d chunkSize=%d (fileSize=%d)",
			hdr.Offset, hdr.ChunkSize, metadata.FileSize)
	}

	// 2) Read the chunk data from the stream
	buf := make([]byte, hdr.ChunkSize)
	if _, err := io.ReadFull(stream, buf); err != nil {
		log.Printf("failed to read chunk data: %v", err)
		return fmt.Errorf("failed to read chunk data: %w", err)
	}

	// 3) Write the chunk data at the correct offset (thread-safe approach)
	//    We do *not* want multiple goroutines messing with file offsets,
	//    so we use `WriteAt`.
	n, err := file.WriteAt(buf, int64(hdr.Offset))
	if err != nil {
		log.Printf("failed to write to file at offset %d: %v", hdr.Offset, err)
		return fmt.Errorf("failed to write to file at offset %d: %w", hdr.Offset, err)
	}
	if uint64(n) != hdr.ChunkSize {
		return fmt.Errorf("partial write: wrote %d out of %d bytes", n, hdr.ChunkSize)
	}

	// 4) Atomically add to the total received
	atomic.AddUint64(totalReceived, hdr.ChunkSize)
	log.Printf("Wrote chunk offset=%d size=%d, totalReceived=%d", hdr.Offset, hdr.ChunkSize, atomic.LoadUint64(totalReceived))

	return nil
}
