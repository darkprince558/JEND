package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: check_turn <host:port> [protocol]")
		fmt.Println("Example: check_turn 1.2.3.4:3478 udp")
		os.Exit(1)
	}

	serverAddr := os.Args[1]
	proto := "udp"
	if len(os.Args) > 2 {
		proto = os.Args[2]
	}

	fmt.Printf("Testing connectivity to %s via %s...\n", serverAddr, proto)

	if proto == "tcp" {
		conn, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
		if err != nil {
			fmt.Printf("TCP Connection Failed: %v\n", err)
			os.Exit(1)
		}
		defer conn.Close()
		fmt.Println("TCP Connection Successful! (Coturn is listening)")
		return
	}

	// For UDP, we send a STUN Binding Request
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		fmt.Printf("Error resolving: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		fmt.Printf("Error listening: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// STUN Binding Request
	req := []byte{
		0x00, 0x01, // Type
		0x00, 0x00, // Length
		0x21, 0x12, 0xA4, 0x42, // Cookie
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, // TxID
	}

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

	if n >= 2 && buffer[0] == 0x01 && buffer[1] == 0x01 {
		fmt.Println("SUCCESS: Received STUN Binding Response!")
	} else {
		fmt.Printf("RECEIVED incomplete/unknown: %x\n", buffer[:n])
	}
}
