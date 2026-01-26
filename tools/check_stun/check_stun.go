package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: check_stun <host:port>")
		os.Exit(1)
	}

	serverAddr := os.Args[1]
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		fmt.Printf("Error resolving address: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		fmt.Printf("Error listening: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Simple Binding Request (STUN)
	// Type: 0x0001 (Binding Request)
	// Length: 0x0000
	// Cookie: 0x2112A442
	// Transaction ID: 12 bytes random
	req := []byte{
		0x00, 0x01, // Type: Binding Request
		0x00, 0x00, // Length: 0
		0x21, 0x12, 0xA4, 0x42, // Magic Cookie
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, // Transaction ID
	}

	fmt.Printf("Sending STUN Binding Request to %s...\n", serverAddr)
	_, err = conn.WriteToUDP(req, udpAddr)
	if err != nil {
		fmt.Printf("Error sending: %v\n", err)
		os.Exit(1)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buffer := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		fmt.Printf("Error reading (timeout?): %v\n", err)
		os.Exit(1)
	}

	// Check response type
	// 0x0101 is Binding Success Response
	if n >= 2 && buffer[0] == 0x01 && buffer[1] == 0x01 {
		fmt.Println("SUCCESS: Received STUN Binding Response!")
	} else {
		fmt.Printf("RECEIVED incomplete or non-success response: %x\n", buffer[:n])
	}
}
