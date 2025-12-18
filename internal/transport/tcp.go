package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
)

// calculateHash creates the SHA-256 fingerprint of a file
func calculateHash(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening file for hash:", err)
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

	fmt.Printf("Listening on port %s... Waiting for sender.\n", port)

	conn, err := listener.Accept()
	if err != nil {
		fmt.Println("Accept error:", err)
		return
	}
	defer conn.Close()
	fmt.Println("Connection established!")

	// 1. Read Metadata (The Handshake)
	var fileSize int64
	var fileName string
	var senderProvidedHash string

	// Read Size, Name, and Hash in order
	fmt.Fscanf(conn, "%d\n", &fileSize)
	fmt.Fscanf(conn, "%s\n", &fileName)
	fmt.Fscanf(conn, "%s\n", &senderProvidedHash)

	fmt.Printf("Receiving: %s (%d bytes)\n", fileName, fileSize)

	// 2. Prepare the destination file
	saveName := "received_" + fileName
	newFile, err := os.Create(saveName)
	if err != nil {
		fmt.Println("Create file error:", err)
		return
	}
	defer newFile.Close()

	// 3. Stream the file data
	fmt.Println("Downloading...")
	_, err = io.Copy(newFile, io.LimitReader(conn, fileSize))
	if err != nil {
		fmt.Println("Transfer error:", err)
		return
	}

	fmt.Printf("Successfully received %s\n", fileName)

	// 4. Integrity Check
	receivedHash := calculateHash(saveName)
	if receivedHash == senderProvidedHash {
		fmt.Println("Integrity Verified: Match.")
	} else {
		fmt.Println("CRITICAL ERROR: File corrupted or hash mismatch.")
		fmt.Printf("Expected: %s\nActual:   %s\n", senderProvidedHash, receivedHash)
	}
}

func StartSender(address string, filePath string) {
	// 1. Open the file and calculate its "fingerprint"
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
	fileSize := fileInfo.Size()
	fileHash := calculateHash(filePath)

	// 2. Connect to receiver
	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Println("Connection error:", err)
		return
	}
	defer conn.Close()

	// 3. Send the "Header"
	// Send Size, Name, and Hash separated by newlines
	fmt.Fprintf(conn, "%d\n", fileSize)
	fmt.Fprintf(conn, "%s\n", fileInfo.Name())
	fmt.Fprintf(conn, "%s\n", fileHash)

	fmt.Printf("Sending %s (%d bytes)...\n", fileInfo.Name(), fileSize)

	// 4. Stream the actual file bytes
	bytesSent, err := io.Copy(conn, file)
	if err != nil {
		fmt.Println("Send error:", err)
		return
	}

	fmt.Printf("Successfully sent %d bytes\n", bytesSent)
}
