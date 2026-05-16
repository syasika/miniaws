package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/syasika/miniaws/cmd/browse"
	"github.com/syasika/miniaws/cmd/container"
	"github.com/syasika/miniaws/cmd/s3"
	"github.com/syasika/miniaws/cmd/sqs"
	"github.com/syasika/miniaws/cmd/ssm"
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
	rootCmd.AddCommand(container.Cmd())
	rootCmd.AddCommand(s3.Cmd())
	rootCmd.AddCommand(ssm.Cmd())
	rootCmd.AddCommand(sqs.Cmd())
	rootCmd.AddCommand(browse.Cmd())

	rootCmd.PersistentFlags().String("endpoint-url", "http://localhost:4566", "Ministack endpoint URL")
}
