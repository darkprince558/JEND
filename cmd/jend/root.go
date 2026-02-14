package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	// Persistent Flags
	headless   bool
	timeoutStr string

	// App Version (injected at build time)
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// RootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "jend",
	Short: "Secure, direct file transfer tool",
	Long: `JEND is a secure, direct file transfer tool.
It allows you to send files, text, and directories directly between devices
on the same network or over the internet using a simple code.`,
	// No Run function as root command just holds subcommands
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of JEND",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("JEND v%s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built:  %s\n", date)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)

	// Persistent Flags (Available to all commands)
	rootCmd.PersistentFlags().BoolVar(&headless, "headless", false, "Run in headless mode (no TUI)")
	rootCmd.PersistentFlags().StringVar(&timeoutStr, "timeout", "10m", "Global timeout duration (e.g. 10m)")
}

func getTimeout() time.Duration {
	d, err := time.ParseDuration(timeoutStr)
	if err != nil {
		fmt.Printf("Invalid timeout format: %v\n", err)
		os.Exit(1)
	}
	return d
}
