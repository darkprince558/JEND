package main

import (
	"fmt"

	"github.com/darkprince558/jend/internal/audit"
	"github.com/spf13/cobra"
)

var (
	historyClear bool
)

var historyCmd = &cobra.Command{
	Use:   "history [code]",
	Short: "View transfer history",
	Long: `View a list of past transfers or detailed info about a specific transfer.
Example:
  jend history
  jend history partial-red-panda
  jend history --clear`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if historyClear {
			if err := audit.ClearHistory(); err != nil {
				fmt.Printf("Error clearing history: %v\n", err)
			} else {
				fmt.Println("History cleared.")
			}
			return
		}

		if len(args) > 0 {
			// Detail view
			audit.ShowDetail(args[0])
		} else {
			// List view
			audit.ShowHistory()
		}
	},
}

func init() {
	rootCmd.AddCommand(historyCmd)
	historyCmd.Flags().BoolVar(&historyClear, "clear", false, "Clear history")
}
