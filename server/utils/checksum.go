package utils

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
)

func FileMD5Checksum(filePath string) (string, error) {
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Initialize hasher
	hasher := md5.New()

	// Use a buffer to reduce system calls
	const bufferSize = 32 * 1024 // 32 KB buffer
	buf := make([]byte, bufferSize)
	if _, err := io.CopyBuffer(hasher, file, buf); err != nil {
		return "", err
	}

	// Return checksum as hex string
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
