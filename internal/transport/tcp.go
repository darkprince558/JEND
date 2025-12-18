package transport

import (
	"fmt"
	"net"
)

func StartReceiver(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer listener.Close()

	fmt.Println("Listening on port: ", port)
	fmt.Println("Waiting for connection...")

	conn, err := listener.Accept()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()
	fmt.Println("Connection established!")

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)

	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Received: ", string(buffer[:n]))
}

func StartSender(adress string, message string) {
	conn, err := net.Dial("tcp", adress)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected to server!")

	_, err = conn.Write([]byte(message))

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Message sent!")

}
