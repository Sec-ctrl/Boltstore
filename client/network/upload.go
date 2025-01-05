package network

import (
	"BS-client/utils"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/quic-go/quic-go"
)

// UploadFileParallel sends the file in parallel using multiple QUIC streams.
// Each chunk has a unique offset and chunk size, which the server uses to place
// the data in the correct position in the destination file.
func UploadFileParallel(
	ctx context.Context,
	conn quic.Connection,
	file *os.File,
	tracer *BandwidthTracer,
	numStreams int,
) error {
	// Get file size so we know when to stop
	fi, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := fi.Size()

	// We'll track the current global offset using an atomic variable.
	// Alternatively, you can use a mutex-protected int64.
	var globalOffset int64 = 0

	// We'll close this channel once we reach the end of the file.
	chunksChan := make(chan int64)

	// Producer goroutine: generate offsets until EOF
	go func() {
		defer close(chunksChan)
		for {
			// Decide next chunk size up front (not strictly necessary, but typical).
			csize := tracer.GetChunkSize()
			if csize <= 0 {
				// If tracer returns 0 or negative, treat as error or small default
				csize = 1024 * 1024 // fallback to 1MB
			}

			// Get the next offset
			current := atomic.AddInt64(&globalOffset, int64(csize)) - int64(csize)

			// If we've already read the entire file, stop.
			if current >= fileSize {
				return
			}
			chunksChan <- current
		}
	}()

	// Now start the workers
	var wg sync.WaitGroup
	wg.Add(numStreams)

	var firstError error
	var once sync.Once

	for i := 0; i < numStreams; i++ {
		go func() {
			defer wg.Done()
			for offset := range chunksChan {
				err := sendNextChunk(ctx, conn, file, offset, tracer, fileSize)
				if err == io.EOF {
					// Normal end-of-file condition. Worker can just stop.
					return
				}
				if err != nil {
					once.Do(func() { firstError = err })
					// You might also want to do something like cancel the context
					// if you want all workers to bail out immediately on error.
					return
				}
			}
		}()
	}

	// Wait for all workers to finish
	wg.Wait()

	// Return the first error we encountered (if any)
	if firstError != nil {
		return firstError
	}

	return nil
}

func sendNextChunk(
	ctx context.Context,
	conn quic.Connection,
	file *os.File,
	offset int64,
	tracer *BandwidthTracer,
	fileSize int64,
) error {
	// Decide chunk size
	csize := tracer.GetChunkSize()
	if csize <= 0 {
		return fmt.Errorf("invalid chunk size: %d", csize)
	}

	// If offset is near the end, clamp csize
	remaining := fileSize - offset
	if remaining <= 0 {
		return io.EOF // no more data
	}
	if remaining < int64(csize) {
		csize = int(remaining)
	}

	// Read from file at the given offset
	buf := make([]byte, csize)
	n, err := file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return fmt.Errorf("file read error at offset %d: %w", offset, err)
	}
	// n might be less than csize if we are at EOF
	if n == 0 {
		// No more data to send
		return io.EOF
	}

	// Open a stream
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("failed to open chunk stream: %w", err)
	}
	defer stream.Close()

	// Write the chunk header
	header := utils.ChunkHeader{
		Offset:    uint64(offset),
		ChunkSize: uint64(n),
	}
	if err := utils.EncodeChunkHeader(stream, header); err != nil {
		return fmt.Errorf("failed to send chunk header: %w", err)
	}

	// Write the chunk data
	written, werr := stream.Write(buf[:n])
	if werr != nil {
		return fmt.Errorf("failed to write chunk data: %w", werr)
	}
	if written != n {
		return fmt.Errorf("partial write: wrote %d of %d bytes", written, n)
	}

	return nil
}
