package network

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/Sec-ctr/Boltstore/utils"
)

// StartQuicServer starts a QUIC server on the specified address with a given concurrency limit.
func StartQuicServer(addr string, ctx context.Context, tlsConfig *tls.Config, quicConfig *quic.Config, concurrencyLimit int) error {

	listener, err := quic.ListenAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	defer listener.Close()

	log.Printf("Server listening on: %v", listener.Addr())

	// Create a semaphore channel to limit how many connections we handle concurrently.
	sem := make(chan struct{}, concurrencyLimit)

	// Loop to accept incoming connections
	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			// If the parent context is canceled, we expect to return nil and shut down gracefully
			if ctx.Err() != nil {
				log.Printf("Shutting down QUIC server...")
				return nil
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		// Acquire one concurrency slot.
		sem <- struct{}{}

		// Handle each connection in its own goroutine
		go func(c quic.Connection) {
			defer func() {
				// Release the concurrency slot when the goroutine finishes
				<-sem
			}()

			// We can optionally wrap the entire connection in a big context for
			// a maximum operation time (e.g., 2 hours) if desired:
			connCtx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
			defer cancel()

			// Handle the connection logic
			if err := handleConnection(connCtx, c); err != nil {
				log.Printf("[ERROR] Connection from %s: %v", c.RemoteAddr(), err)
				// Provide a nonzero application error code if you want to signal an error to the client
				c.CloseWithError(1, fmt.Sprintf("operation failed: %v", err))
				return
			}

			// Close the connection gracefully with code 0
			c.CloseWithError(0, "session complete")
		}(conn)
	}
}

// handleConnection manages the overall lifecycle of a single QUIC connection
func handleConnection(ctx context.Context, conn quic.Connection) error {
	log.Printf("New session from %v", conn.RemoteAddr().String())

	// For metadata, let's give a short timeout to receive it;
	// if the client doesn't send metadata quickly, we close the connection.
	metaCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := handleMetaStream(metaCtx, conn); err != nil {
		// Return the error so the caller can log it and possibly close the connection with an error code
		return fmt.Errorf("handleMetaStream failed: %w", err)
	}

	return nil // Success
}

// handleMetaStream accepts the "metadata stream", decodes the metadata,
// and dispatches to the appropriate handler (upload, download, delete).
func handleMetaStream(ctx context.Context, conn quic.Connection) error {
	metaStream, err := conn.AcceptStream(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			// If we timed out waiting for metadata, close the connection
			log.Printf("[WARN] Metadata not received in time from %s", conn.RemoteAddr())
			return conn.CloseWithError(1, "metadata timeout")
		}
		return fmt.Errorf("failed to accept metadata stream: %w", err)
	}
	defer metaStream.Close()

	// Attempt to decode metadata
	metadata, err := utils.DecodeMetadata(metaStream)
	if err != nil {
		log.Printf("[WARN] Invalid metadata from %s: %v", conn.RemoteAddr().String(), err)
		return conn.CloseWithError(2, "invalid metadata")
	}

	// Validate metadata
	if !utils.IsValidMetadata(metadata) {
		log.Printf("[WARN] Metadata validation failed for %s: %+v", conn.RemoteAddr().String(), metadata)
		return conn.CloseWithError(2, "metadata validation failed")
	}

	log.Printf("Metadata received from %s: %v", conn.RemoteAddr(), metadata)

	// Depending on the operation, handle the next step
	switch metadata.Operation {
	case 1: // Upload
		log.Printf("Preparing to handle upload for file: %s", metadata.FileName)
		if err := utils.HandleFileUploadParallel(ctx, conn, metadata); err != nil {
			return fmt.Errorf("handleFileUploadParallel failed: %w", err)
		}
	case 2: // Download
		log.Printf("Preparing to handle download for file: %s", metadata.FileName)
		if err := utils.HandleFileDownload(ctx, conn, metadata); err != nil {
			return fmt.Errorf("handleFileDownload failed: %w", err)
		}
	case 3: // Delete
		log.Printf("Preparing to handle delete for file: %s", metadata.FileName)
		if err := utils.HandleFileDelete(ctx, metadata); err != nil {
			return fmt.Errorf("handleFileDelete failed: %w", err)
		}
	default:
		log.Printf("[WARN] Unsupported operation type %d from %s", metadata.Operation, conn.RemoteAddr())
		return conn.CloseWithError(4, "unsupported operation")
	}

	log.Printf("Operation %d successfully completed for %s", metadata.Operation, conn.RemoteAddr())
	return nil
}
