// Package sqs provides the miniaws sqs CLI subcommand.
package sqs

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/syasika/miniaws/internal/awsclient"
	"github.com/syasika/miniaws/internal/sqsops"
)

var (
	bucketStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	objectStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	sizeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33")).Padding(0, 1)
	emptyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
)

// Cmd returns the sqs command.
func Cmd() *cobra.Command { return sqsCmd }

var sqsCmd = &cobra.Command{
	Use:   "sqs",
	Short: "Manage SQS queues",
	Long:  `List, create, delete queues and send/receive/delete messages.`,
}

var sqsLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List queues",
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewSQSClient(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		queues, err := sqsops.ListQueues(ctx, client)
		if err != nil {
			return err
		}
		if len(queues) == 0 {
			fmt.Println(emptyStyle.Render("No queues."))
			return nil
		}
		fmt.Println(headerStyle.Render(fmt.Sprintf(" Queues (%d) ", len(queues))))
		for _, q := range queues {
			fmt.Printf("  📦 %s\n", bucketStyle.Render(q.Name))
		}
		return nil
	},
}

var sqsCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a queue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewSQSClient(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		q, err := sqsops.CreateQueue(ctx, client, args[0])
		if err != nil {
			return err
		}
		fmt.Printf("%s Queue '%s' created\n", successStyle.Render("✓"), q.Name)
		return nil
	},
}

var sqsRmCmd = &cobra.Command{
	Use:   "rm <url>",
	Short: "Delete a queue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewSQSClient(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		if err := sqsops.DeleteQueue(ctx, client, args[0]); err != nil {
			return err
		}
		fmt.Printf("%s Queue deleted\n", successStyle.Render("✓"))
		return nil
	},
}

var sqsSendCmd = &cobra.Command{
	Use:   "send <url> <message>",
	Short: "Send a message to a queue",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewSQSClient(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		msgID, err := sqsops.SendMessage(ctx, client, args[0], strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
		fmt.Printf("%s Message sent (id: %s)\n", successStyle.Render("✓"), msgID)
		return nil
	},
}

var sqsRecvCmd = &cobra.Command{
	Use:   "recv <url>",
	Short: "Receive messages from a queue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewSQSClient(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		max, _ := cmd.Flags().GetInt("max")
		msgs, err := sqsops.ReceiveMessages(ctx, client, args[0], max)
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			fmt.Println(emptyStyle.Render("No messages."))
			return nil
		}
		fmt.Println(headerStyle.Render(fmt.Sprintf(" Messages (%d) ", len(msgs))))
		for _, m := range msgs {
			fmt.Printf("  [%s] %s\n", sizeStyle.Render(m.ID), objectStyle.Render(m.Body))
		}
		return nil
	},
}

func init() {
	sqsCmd.AddCommand(sqsLsCmd)
	sqsCmd.AddCommand(sqsCreateCmd)
	sqsCmd.AddCommand(sqsRmCmd)
	sqsCmd.AddCommand(sqsSendCmd)
	sqsCmd.AddCommand(sqsRecvCmd)

	sqsRecvCmd.Flags().Int("max", 10, "Max messages to receive")
}
