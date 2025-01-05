package utils

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Metadata represents the metadata sent by the client.
type Metadata struct {
	Operation    uint8
	FileName     string
	FileSize     uint64
	ChunkSize    uint32
	Timestamp    uint64
	ChecksumType uint8
	Checksum     []byte
}

// EncodeMetadata serializes the Metadata struct into a binary format
// matching the format expected by the server.
func EncodeAndSendMetadata(w io.Writer, metadata *Metadata) error {
	// Optional: Validate metadata before sending (re-using server’s IsValidMetadata).
	if !IsValidMetadata(metadata) {
		return fmt.Errorf("invalid metadata")
	}

	// 1. Write Operation (uint8)
	if err := binary.Write(w, binary.LittleEndian, metadata.Operation); err != nil {
		return fmt.Errorf("failed to write operation: %w", err)
	}

	// 2. Write FileName length (uint16) and then the file name (bytes)
	fileNameLen := uint16(len(metadata.FileName))
	if err := binary.Write(w, binary.LittleEndian, fileNameLen); err != nil {
		return fmt.Errorf("failed to write filename length: %w", err)
	}

	if _, err := w.Write([]byte(metadata.FileName)); err != nil {
		return fmt.Errorf("failed to write filename: %w", err)
	}

	// 3. Write FileSize (uint64)
	if err := binary.Write(w, binary.LittleEndian, metadata.FileSize); err != nil {
		return fmt.Errorf("failed to write file size: %w", err)
	}

	// 4. Write ChunkSize (uint32)
	if err := binary.Write(w, binary.LittleEndian, metadata.ChunkSize); err != nil {
		return fmt.Errorf("failed to write chunk size: %w", err)
	}

	// 5. Write Timestamp (uint64)
	if err := binary.Write(w, binary.LittleEndian, metadata.Timestamp); err != nil {
		return fmt.Errorf("failed to write timestamp: %w", err)
	}

	// 6. Write ChecksumType (uint8)
	if err := binary.Write(w, binary.LittleEndian, metadata.ChecksumType); err != nil {
		return fmt.Errorf("failed to write checksum type: %w", err)
	}

	// 7. Write Checksum (variable length based on ChecksumType)
	if _, err := w.Write(metadata.Checksum); err != nil {
		return fmt.Errorf("failed to write checksum: %w", err)
	}

	return nil
}

// isValidMetadata validates the contents of the metadata.
func IsValidMetadata(metadata *Metadata) bool {
	if metadata.FileSize <= 0 || metadata.FileSize > 1<<40 { // Limit to 1 TB
		return false
	}
	if len(metadata.FileName) == 0 || len(metadata.FileName) > 255 {
		return false
	}
	return true
}
