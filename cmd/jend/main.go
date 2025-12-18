package main

import (
	"fmt"
	"os"

	"github.com/darkprince558/jend/internal/transport"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "jend",
	Short: "JEND: P2P file transfer tool",
	Long:  `A CLI tool for sending files directly between peers.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(receiveCmd)
}

var sendCmd = &cobra.Command{
	Use:   "send [file]",
	Short: "Send a file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		transport.StartSender("localhost:8080", filePath)
	},
}

var receiveCmd = &cobra.Command{
	Use:   "receive [code]",
	Short: "Receive a file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		code := args[0]
		fmt.Printf("Using transfer code: %s\n", code)

		// Start the listener
		transport.StartReceiver("8080")
	},
}
