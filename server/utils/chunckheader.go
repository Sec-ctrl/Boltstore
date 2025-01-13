package utils

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ChunkHeader is the metadata the client sends at the start of each chunk stream.
type ChunkHeader struct {
	Offset    uint64 // Where to write this chunk
	ChunkSize uint64 // How many bytes in this chunk
}

// EncodeChunkHeader writes the header to w in a fixed-size binary format.
func EncodeChunkHeader(w io.Writer, header ChunkHeader) error {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], header.Offset)
	binary.LittleEndian.PutUint64(buf[8:16], header.ChunkSize)
	_, err := w.Write(buf[:])
	return err
}

// DecodeChunkHeader reads the header from r.
func DecodeChunkHeader(r io.Reader) (ChunkHeader, error) {
	var header ChunkHeader
	var buf [16]byte
	if header.ChunkSize == 0 {
		return header, fmt.Errorf("invalid chunk: size cannot be 0")
	}
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return header, fmt.Errorf("failed to decode chunk header: %w", err)
	}
	header.Offset = binary.LittleEndian.Uint64(buf[0:8])
	header.ChunkSize = binary.LittleEndian.Uint64(buf[8:16])
	return header, nil
}
