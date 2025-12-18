package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/darkprince558/jend/pkg/protocol"
)

const ChunkSize = 1024 // 1KB chunks for testing

// Metadata represents the initial handshake payload
type Metadata struct {
	Name string
	Size int64
	Hash string
}

func calculateHash(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func StartReceiver(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("Listen error:", err)
		return
	}
	defer listener.Close()

	fmt.Printf("Listening on port %s...\n", port)

	conn, err := listener.Accept()
	if err != nil {
		fmt.Println("Accept error:", err)
		return
	}
	defer conn.Close()

	var newFile *os.File
	var currentSize int64
	var expectedSize int64
	var meta Metadata

	// The Main Receive Loop
	for {
		// 1. Read the Packet Header
		pType, length, err := protocol.DecodeHeader(conn)
		if err != nil {
			if err == io.EOF {
				fmt.Println("Sender closed connection.")
				break
			}
			fmt.Println("Header decode error:", err)
			return
		}

		// 2. Read the Payload (Body) based on Length
		payload := make([]byte, length)
		_, err = io.ReadFull(conn, payload)
		if err != nil {
			fmt.Println("Payload read error:", err)
			return
		}

		// 3. Handle Packet Type
		switch pType {
		case protocol.TypeHandshake:
			// Parse JSON Metadata
			if err := json.Unmarshal(payload, &meta); err != nil {
				fmt.Println("Handshake parse error:", err)
				return
			}
			expectedSize = meta.Size
			fmt.Printf("Receiving: %s (%d bytes)\n", meta.Name, meta.Size)

			newFile, err = os.Create("received_" + meta.Name)
			if err != nil {
				fmt.Println("File create error:", err)
				return
			}
			// Don't defer close here strictly, we close when done or error

		case protocol.TypeData:
			if newFile == nil {
				fmt.Println("Error: Received data before handshake")
				return
			}
			// Write chunk to disk
			n, err := newFile.Write(payload)
			if err != nil {
				fmt.Println("Disk write error:", err)
				return
			}
			currentSize += int64(n)

			// Send ACK back to Sender
			if err := protocol.EncodeHeader(conn, protocol.TypeAck, 0); err != nil {
				fmt.Println("Ack send error:", err)
				return
			}

			// Visual progress update
			fmt.Printf("\rDownloading... %d / %d bytes", currentSize, expectedSize)
		}

		// Check if transfer is complete
		if currentSize >= expectedSize && expectedSize > 0 {
			fmt.Println("\nTransfer Complete.")
			newFile.Close()
			break
		}
	}

	// Integrity Verification
	receivedHash := calculateHash("received_" + meta.Name)
	if receivedHash == meta.Hash {
		fmt.Println("Integrity Verified: Match.")
	} else {
		fmt.Printf("Integrity Mismatch!\nExpected: %s\nActual:   %s\n", meta.Hash, receivedHash)
	}
}

func StartSender(address string, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("File error:", err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Println("Stat error:", err)
		return
	}

	// Pre-calculate hash
	fmt.Println("Calculating hash...")
	fileHash := calculateHash(filePath)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Println("Connection error:", err)
		return
	}
	defer conn.Close()

	// 1. Send Handshake Packet
	meta := Metadata{
		Name: fileInfo.Name(),
		Size: fileInfo.Size(),
		Hash: fileHash,
	}
	metaBytes, _ := json.Marshal(meta)

	if err := protocol.EncodeHeader(conn, protocol.TypeHandshake, uint32(len(metaBytes))); err != nil {
		fmt.Println("Handshake send error:", err)
		return
	}
	if _, err := conn.Write(metaBytes); err != nil {
		fmt.Println("Handshake body error:", err)
		return
	}

	// 2. Start Chunk Loop
	buffer := make([]byte, ChunkSize)
	var totalSent int64

	fmt.Printf("Sending %s in %d byte chunks...\n", meta.Name, ChunkSize)

	for {
		n, readErr := file.Read(buffer)
		if n > 0 {
			// Send Header
			if err := protocol.EncodeHeader(conn, protocol.TypeData, uint32(n)); err != nil {
				fmt.Println("Header send error:", err)
				return
			}
			// Send Payload
			if _, err := conn.Write(buffer[:n]); err != nil {
				fmt.Println("Data send error:", err)
				return
			}

			// Wait for ACK
			// We expect a header of TypeAck (length 0)
			ackType, _, err := protocol.DecodeHeader(conn)
			if err != nil {
				fmt.Println("Ack receive error:", err)
				return
			}
			if ackType != protocol.TypeAck {
				fmt.Println("Error: Expected ACK, got", ackType)
				return
			}

			totalSent += int64(n)
			fmt.Printf("\rSent: %d / %d bytes", totalSent, meta.Size)
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			fmt.Println("File read error:", readErr)
			return
		}
	}
	fmt.Println("\nFile sent successfully.")
}
