package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "miniaws",
	Short: "Miniaws CLI - Manage ministack containers and AWS resources",
	Long:  `A CLI utility to spin up a ministack container and manage AWS resources (S3, etc.) via the local endpoint.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(containerCmd)
	rootCmd.AddCommand(s3Cmd)
	rootCmd.AddCommand(ssmCmd)
	rootCmd.AddCommand(sqsCmd)

	rootCmd.PersistentFlags().String("endpoint-url", "http://localhost:4566", "Ministack endpoint URL")
}
