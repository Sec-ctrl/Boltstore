package utils

import (
	"crypto/md5"
	"io"
	"os"
)

func ComputeChecksum(filePath string) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	hasher := md5.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}
