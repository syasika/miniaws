package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/syasika/miniaws/internal/awsclient"
	"github.com/syasika/miniaws/internal/ssmops"
)

var ssmCmd = &cobra.Command{
	Use:   "ssm",
	Short: "Manage SSM Parameter Store",
	Long:  `List, get, put, and delete parameters in the ministack SSM Parameter Store.`,
}

var ssmLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List parameters",
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewSSMClient(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		params, err := ssmops.ListAllParameters(ctx, client)
		if err != nil {
			return err
		}
		if len(params) == 0 {
			fmt.Println(emptyStyle.Render("No parameters."))
			return nil
		}
		fmt.Println(headerStyle.Render(fmt.Sprintf(" Parameters (%d) ", len(params))))
		for _, p := range params {
			fmt.Printf("  %s  (%s, v%d)\n", bucketStyle.Render(p.Name), sizeStyle.Render(p.Type), p.Version)
		}
		return nil
	},
}

var ssmGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get a parameter value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewSSMClient(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		p, err := ssmops.GetParameter(ctx, client, args[0])
		if err != nil {
			return err
		}
		fmt.Printf("%s  %s\n", containerLabelStyle.Render("Name:"), containerValueStyle.Render(p.Name))
		fmt.Printf("%s  %s\n", containerLabelStyle.Render("Type:"), containerValueStyle.Render(p.Type))
		fmt.Printf("%s  %s\n", containerLabelStyle.Render("Value:"), containerValueStyle.Render(p.Value))
		fmt.Printf("%s  v%d\n", containerLabelStyle.Render("Version:"), p.Version)
		return nil
	},
}

var ssmPutCmd = &cobra.Command{
	Use:   "put <name> <value>",
	Short: "Put a parameter",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewSSMClient(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		paramType, _ := cmd.Flags().GetString("type")
		if paramType == "" {
			paramType = "String"
		}
		name := args[0]
		value := strings.Join(args[1:], " ")

		if err := ssmops.PutParameter(ctx, client, name, value, paramType); err != nil {
			return err
		}
		fmt.Printf("%s Parameter '%s' saved\n", successStyle.Render("✓"), name)
		return nil
	},
}

var ssmRmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Delete a parameter",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewSSMClient(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		if err := ssmops.DeleteParameter(ctx, client, args[0]); err != nil {
			return err
		}
		fmt.Printf("%s Parameter '%s' deleted\n", successStyle.Render("✓"), args[0])
		return nil
	},
}

func init() {
	ssmCmd.AddCommand(ssmLsCmd)
	ssmCmd.AddCommand(ssmGetCmd)
	ssmCmd.AddCommand(ssmPutCmd)
	ssmCmd.AddCommand(ssmRmCmd)
	ssmPutCmd.Flags().String("type", "String", "Parameter type (String, StringList, SecureString)")
}
