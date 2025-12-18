package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/darkprince558/jend/internal/transport"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "jend",
	Short: "JEND: A High-speed, secure P2P file transfer tool",
	Long:  `A CLI tool for sending files directly between peers using QUIC and PAKE encryption.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// We will add subcommands (send/receive) here in the next step
	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(receiveCmd)
}

var sendCmd = &cobra.Command{
	Use:   "send [file]",
	Short: "Send a file or folder",
	Args:  cobra.MinimumNArgs(1), // Ensures the user provides a file path
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		fmt.Printf("Initiating transfer for: %s\n", filePath)
	},
}

var receiveCmd = &cobra.Command{
	Use:   "receive [code]",
	Short: "Receive a file",
	Run: func(cmd *cobra.Command, args []string) {
		code := args[0]
		fmt.Printf("do Using code: %s\n", code)

		// Call our new function from the transport package
		transport.StartReceiver("8080")
	},
}
