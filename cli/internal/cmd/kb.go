package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read [id]",
	Short: "Read a KB document by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("leo read %s: not yet implemented\n", args[0])
		return nil
	},
}

var writeCmd = &cobra.Command{
	Use:   "write [file]",
	Short: "Write a document to the KB",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("leo write %s: not yet implemented\n", args[0])
		return nil
	},
}

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query KB documents by tags, type, or scope",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("leo query: not yet implemented")
		return nil
	},
}

func init() {
	queryCmd.Flags().StringSlice("tags", nil, "Filter by tags")
	queryCmd.Flags().String("type", "", "Filter by type")
	queryCmd.Flags().String("scope", "", "Filter by scope")
}

var deleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a KB document by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("leo delete %s: not yet implemented\n", args[0])
		return nil
	},
}

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Rebuild the KB index from docs",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("leo reindex: not yet implemented")
		return nil
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate all KB documents against the schema",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("leo validate: not yet implemented")
		return nil
	},
}
