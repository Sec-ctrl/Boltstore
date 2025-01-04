package utils

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
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

// RateLimiter tracks the rate of requests per client IP.
type RateLimiter struct {
	mu          sync.Mutex
	limits      map[string]*rateLimitEntry
	maxRequests int
	window      time.Duration
}

type rateLimitEntry struct {
	count      int
	resetAfter time.Time
}

// NewRateLimiter creates a new RateLimiter with specified limits.
func NewRateLimiter(maxRequests int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		limits:      make(map[string]*rateLimitEntry),
		maxRequests: maxRequests,
		window:      window,
	}
	go rl.cleanup() // Start cleanup goroutine
	return rl
}

// Allow checks if a request from the given client IP is allowed.
func (rl *RateLimiter) Allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, exists := rl.limits[clientIP]
	if !exists || now.After(entry.resetAfter) {
		rl.limits[clientIP] = &rateLimitEntry{
			count:      1,
			resetAfter: now.Add(rl.window),
		}
		return true
	}

	if entry.count < rl.maxRequests {
		entry.count++
		return true
	}

	return false
}

// cleanup periodically removes expired rate limit entries.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		rl.mu.Lock()
		for ip, entry := range rl.limits {
			if now.After(entry.resetAfter) {
				delete(rl.limits, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// decodeMetadata parses metadata from a binary reader.
func DecodeMetadata(r io.Reader) (*Metadata, error) {
	var metadata Metadata

	// Read Operation
	if err := binary.Read(r, binary.LittleEndian, &metadata.Operation); err != nil {
		return nil, fmt.Errorf("failed to read operation: %w", err)
	}

	// Read FileNameLength and FileName
	var fileNameLength uint16
	if err := binary.Read(r, binary.LittleEndian, &fileNameLength); err != nil {
		return nil, fmt.Errorf("failed to read filename length: %w", err)
	}
	if fileNameLength == 0 || fileNameLength > 255 {
		return nil, errors.New("invalid filename length")
	}
	filenameBytes := make([]byte, fileNameLength)
	if _, err := io.ReadFull(r, filenameBytes); err != nil {
		return nil, fmt.Errorf("failed to read filename: %w", err)
	}
	metadata.FileName = string(filenameBytes)

	// Validate FileName
	if !isValidFileName(metadata.FileName) {
		return nil, fmt.Errorf("invalid filename: %s", metadata.FileName)
	}

	// Read FileSize
	if err := binary.Read(r, binary.LittleEndian, &metadata.FileSize); err != nil {
		return nil, fmt.Errorf("failed to read file size: %w", err)
	}

	// Read ChunkSize
	if err := binary.Read(r, binary.LittleEndian, &metadata.ChunkSize); err != nil {
		return nil, fmt.Errorf("failed to read chunk size: %w", err)
	}

	// Read Timestamp
	if err := binary.Read(r, binary.LittleEndian, &metadata.Timestamp); err != nil {
		return nil, fmt.Errorf("failed to read timestamp: %w", err)
	}

	// Read ChecksumType
	if err := binary.Read(r, binary.LittleEndian, &metadata.ChecksumType); err != nil {
		return nil, fmt.Errorf("failed to read checksum type: %w", err)
	}

	// Validate ChecksumType
	checksumLength := getChecksumLength(metadata.ChecksumType)
	if checksumLength == 0 {
		return nil, fmt.Errorf("unsupported checksum type: %d", metadata.ChecksumType)
	}

	// Read Checksum
	metadata.Checksum = make([]byte, checksumLength)
	if _, err := io.ReadFull(r, metadata.Checksum); err != nil {
		return nil, fmt.Errorf("failed to read checksum: %w", err)
	}

	return &metadata, nil
}

// getChecksumLength returns the expected length of the checksum based on its type.
func getChecksumLength(checksumType uint8) int {
	switch checksumType {
	case 1: // CRC32
		return 4
	case 2: // MD5
		return 16
	case 3: // SHA256
		return 32
	default:
		return 0
	}
}

// isValidFileName checks if the file name contains only allowed characters.
func isValidFileName(fileName string) bool {
	for _, char := range fileName {
		if char == '/' || char == '\\' || char == ':' || char == '*' || char == '?' || char == '"' || char == '<' || char == '>' || char == '|' {
			return false
		}
	}
	return len(fileName) > 0 && len(fileName) <= 255
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
