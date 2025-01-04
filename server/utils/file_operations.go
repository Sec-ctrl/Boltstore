package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

// handleFileDelete handles the file deletion operation.
func HandleFileDelete(ctx context.Context, metadata *Metadata) error {
	log.Printf("Deleting file: %s", metadata.FileName)
	ctxDel, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	defer ctxDel.Done()

	// In production, ensure the file path is valid, user has permission, etc.
	if err := os.Remove(metadata.FileName); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", metadata.FileName, err)
	}

	log.Printf("File %s successfully deleted", metadata.FileName)

	return nil
}
