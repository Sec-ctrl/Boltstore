package network

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/quic-go/quic-go"
)

func UploadFileParallel(
	ctx context.Context,
	conn quic.Connection,
	file *os.File,
	tracer *BandwidthTracer, // something that calculates chunk sizes
	numStreams int, // how many parallel streams to use
) error {
	// 1) We might want a worker pool approach—spawn `numStreams` goroutines
	//    each reading chunks from the file and sending them in parallel.
	var wg sync.WaitGroup
	wg.Add(numStreams)

	// A channel to send “work items” (i.e., chunk offsets or chunk indices).
	// Alternatively, you can do a shared global offset protected by a mutex.
	chunksChan := make(chan struct{}, 100) // or unbuffered, or however big you want

	// A simple producer that keeps generating “chunk needed” signals
	go func() {
		defer close(chunksChan)
		for {
			// We break out once we hit EOF or some other condition
			// but for demonstration, let's just keep pushing
			chunksChan <- struct{}{}
		}
	}()

	// 2) Each worker goroutine repeatedly:
	//    - Reads the next chunk from the file
	//    - Opens a stream
	//    - Sends that chunk
	//    - Gets feedback
	for i := 0; i < numStreams; i++ {
		go func(workerID int) {
			defer wg.Done()
			for range chunksChan {
				if err := sendNextChunk(ctx, conn, file, tracer); err != nil {
					// Handle the error (maybe log or store in a shared variable)
					return
				}
			}
		}(i)
	}

	// Wait for all workers to finish
	wg.Wait()
	return nil
}

func sendNextChunk(
	ctx context.Context,
	conn quic.Connection,
	file *os.File,
	tracer *BandwidthTracer,
) error {
	// 1) Decide chunk size
	chunkSize := tracer.GetChunkSize()
	if chunkSize <= 0 {
		return fmt.Errorf("invalid chunk size: %d", chunkSize)
	}

	// 2) Read from file. Usually you'll need to track a global offset
	//    or something so each worker reads a different portion.
	//    For a trivial example, let's assume a single goroutine approach
	//    or a safe "read" that increments file offset each call.
	buf := make([]byte, chunkSize)
	n, err := file.Read(buf)
	if err == io.EOF && n == 0 {
		// Reached the end of file
		return io.EOF
	}
	if err != nil && err != io.EOF {
		return err
	}

	// 3) Open a bidirectional stream
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

	// 4) Send the chunk
	if _, err := stream.Write(buf[:n]); err != nil {
		return err
	}

	return nil
}
