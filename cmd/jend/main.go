package transport

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/darkprince558/jend/pkg/protocol"
)

const ChunkSize = 1024 * 64 // 64KB chunks for better throughput

// Metadata represents the initial handshake payload
type Metadata struct {
	Name string
	Size int64
	Hash string
}

// calculateHash generates a SHA-256 fingerprint for the given file
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

// StartReceiver listens for incoming connections and handles file reception
func StartReceiver(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Printf("Failed to bind port %s: %v\n", port, err)
		return
	}
	defer listener.Close()
	fmt.Printf("Listening on port %s...\n", port)

	conn, err := listener.Accept()
	if err != nil {
		fmt.Printf("Connection error: %v\n", err)
		return
	}
	defer conn.Close()

	var newFile *os.File
	var currentSize int64
	var expectedSize int64
	var meta Metadata

	// 1. Handshake Phase
	pType, length, err := protocol.DecodeHeader(conn)
	if err != nil || pType != protocol.TypeHandshake {
		fmt.Println("Handshake failed")
		return
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		fmt.Println("Failed to read handshake payload")
		return
	}

	if err := json.Unmarshal(payload, &meta); err != nil {
		fmt.Println("Invalid metadata format")
		return
	}
	expectedSize = meta.Size
	savePath := "received_" + meta.Name

	// 2. Check for existing file (Resume Logic)
	fileStat, err := os.Stat(savePath)
	if err == nil {
		currentSize = fileStat.Size()
		fmt.Printf("Resuming %s from byte %d (%.1f%%)\n", meta.Name, currentSize, float64(currentSize)/float64(expectedSize)*100)
		newFile, err = os.OpenFile(savePath, os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		fmt.Printf("Receiving new file: %s (%d bytes)\n", meta.Name, expectedSize)
		newFile, err = os.Create(savePath)
	}

	if err != nil {
		fmt.Printf("File system error: %v\n", err)
		return
	}
	defer newFile.Close()

	// 3. Send Resume Offset (8 bytes)
	offsetBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(offsetBuf, uint64(currentSize))

	if err := protocol.EncodeHeader(conn, protocol.TypeAck, 8); err != nil {
		return
	}
	if _, err := conn.Write(offsetBuf); err != nil {
		return
	}

	// 4. Data Transfer Loop
	buf := make([]byte, ChunkSize) // Reused buffer for incoming data
	for {
		pType, length, err := protocol.DecodeHeader(conn)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("Protocol error: %v\n", err)
			return
		}

		// Ensure buffer is large enough for the incoming chunk
		if uint32(len(buf)) < length {
			buf = make([]byte, length)
		}

		// Read exactly 'length' bytes
		if _, err := io.ReadFull(conn, buf[:length]); err != nil {
			break
		}

		if pType == protocol.TypeData {
			if _, err := newFile.Write(buf[:length]); err != nil {
				fmt.Printf("Disk write error: %v\n", err)
				return
			}
			currentSize += int64(length)

			// Send simple ACK (0 payload length)
			protocol.EncodeHeader(conn, protocol.TypeAck, 0)

			// Simple progress indicator
			fmt.Printf("\rDownloading: %d / %d bytes", currentSize, expectedSize)
		}

		if currentSize >= expectedSize {
			fmt.Println("\nTransfer complete.")
			break
		}
	}

	// 5. Integrity Verification
	fmt.Println("Verifying integrity...")
	if calculateHash(savePath) == meta.Hash {
		fmt.Println("Success: File hash matches.")
	} else {
		fmt.Println("Error: Integrity check failed. File may be corrupted.")
	}
}

// StartSender connects to a receiver and sends a file with resume capability
func StartSender(address string, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Could not open file: %v\n", err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Println("Could not stat file")
		return
	}

	fmt.Println("Calculating file hash...")
	fileHash := calculateHash(filePath)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		return
	}
	defer conn.Close()

	// 1. Send Handshake
	meta := Metadata{
		Name: fileInfo.Name(),
		Size: fileInfo.Size(),
		Hash: fileHash,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return
	}

	if err := protocol.EncodeHeader(conn, protocol.TypeHandshake, uint32(len(metaBytes))); err != nil {
		return
	}
	if _, err := conn.Write(metaBytes); err != nil {
		return
	}

	// 2. Receive Resume Offset
	pType, length, err := protocol.DecodeHeader(conn)
	if err != nil || pType != protocol.TypeAck || length != 8 {
		fmt.Println("Handshake rejected or invalid offset received")
		return
	}

	offsetBuf := make([]byte, 8)
	if _, err := io.ReadFull(conn, offsetBuf); err != nil {
		return
	}

	resumeOffset := int64(binary.LittleEndian.Uint64(offsetBuf))
	if resumeOffset > 0 {
		fmt.Printf("Resuming from byte %d\n", resumeOffset)
		if _, err := file.Seek(resumeOffset, 0); err != nil {
			fmt.Printf("Seek error: %v\n", err)
			return
		}
	}

	// 3. Send Loop
	buffer := make([]byte, ChunkSize)
	totalSent := resumeOffset

	fmt.Printf("Sending data...\n")
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			// Send Header + Data
			if err := protocol.EncodeHeader(conn, protocol.TypeData, uint32(n)); err != nil {
				return
			}
			if _, err := conn.Write(buffer[:n]); err != nil {
				return
			}

			// Wait for ACK
			if _, _, err := protocol.DecodeHeader(conn); err != nil {
				return
			}

			totalSent += int64(n)
			fmt.Printf("\rSent: %d / %d bytes", totalSent, meta.Size)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("\nRead error: %v\n", err)
			return
		}
	}
	fmt.Println("\nFile sent successfully.")
}
