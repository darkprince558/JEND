package protocol

import (
	"encoding/binary"
	"io"
)

// Packet Types
const (
	TypePAKE      = 0 // PAKE authentication message
	TypeHandshake = 1 // Initial metadata (Filename, Size, Hash)
	TypeData      = 2 // File chunk data
	TypeAck       = 3 // Acknowledgment of receipt
	TypeError     = 4 // Error signal
	TypeCancel    = 5 // Sender cancellation signal
	TypeRangeReq  = 6 // Parallel stream range request
)

// PacketHeader represents the fixed-size header for every packet
type PacketHeader struct {
	Type   uint8  // 1 byte
	Length uint32 // 4 bytes
}

// EncodeHeader writes the binary representation of the header to the writer
func EncodeHeader(w io.Writer, pType uint8, length uint32) error {
	if err := binary.Write(w, binary.LittleEndian, pType); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, length); err != nil {
		return err
	}
	return nil
}

// DecodeHeader reads binary data from the reader and returns the header fields
func DecodeHeader(r io.Reader) (uint8, uint32, error) {
	var pType uint8
	var length uint32

	if err := binary.Read(r, binary.LittleEndian, &pType); err != nil {
		return 0, 0, err
	}
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return 0, 0, err
	}

	return pType, length, nil
}
