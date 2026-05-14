package cmd

import (
	"context"
	"fmt"
	"os"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

var (
	containerLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	containerValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	containerStatusOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	containerStatusWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	containerStatusBad  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	containerSuccess    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
)

var containerCmd = &cobra.Command{
	Use:   "container",
	Short: "Manage the ministack container",
	Long:  `Start, stop, remove, or check the status of the ministack Docker container.`,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the ministack container",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if config == nil {
			fmt.Println("Not initialized. Run 'miniaws init' first.")
			return nil
		}

		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return fmt.Errorf("failed to connect to Docker: %w", err)
		}

		ci, err := inspectContainer(ctx, cli, config.ContainerName)
		if err != nil {
			if cerrdefs.IsNotFound(err) {
				fmt.Printf("Container '%s' not found\n", config.ContainerName)
				return nil
			}
			return fmt.Errorf("failed to inspect container: %w", err)
		}

		statusStyle := containerStatusOK
		statusSymbol := "● running"
		if ci.State.Status != "running" {
			statusStyle = containerStatusWarn
			statusSymbol = "● " + ci.State.Status
		}

		fmt.Printf("%s  %s\n", containerLabelStyle.Render("Container:"), containerValueStyle.Render(config.ContainerName))
		fmt.Printf("%s  %s\n", containerLabelStyle.Render("Image:"), containerValueStyle.Render(config.ImageName))
		fmt.Printf("%s  %s\n", containerLabelStyle.Render("Status:"), statusStyle.Render(statusSymbol))
		if ci.State.Status == "running" {
			fmt.Printf("%s  %s\n", containerLabelStyle.Render("Started:"), containerValueStyle.Render(ci.State.StartedAt))
		}
		return nil
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the ministack container",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if config == nil {
			return fmt.Errorf("not initialized, run 'miniaws init' first")
		}

		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return fmt.Errorf("failed to connect to Docker: %w", err)
		}

		if err := cli.ContainerStart(ctx, config.ContainerName, container.StartOptions{}); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}

		fmt.Printf("%s Container '%s' started\n", containerSuccess.Render("✓"), config.ContainerName)
		return nil
	},
}

var containerRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove the ministack container and reset configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if config == nil {
			return fmt.Errorf("not initialized, run 'miniaws init' first")
		}

		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return fmt.Errorf("failed to connect to Docker: %w", err)
		}

		force, _ := cmd.Flags().GetBool("force")

		if err := cli.ContainerRemove(ctx, config.ContainerName, container.RemoveOptions{Force: force}); err != nil {
			return fmt.Errorf("failed to remove container: %w", err)
		}

		path, err := configFilePath()
		if err != nil {
			return fmt.Errorf("failed to get config path: %w", err)
		}
		os.Remove(path)

		fmt.Printf("%s Container '%s' removed\n", containerSuccess.Render("✓"), config.ContainerName)
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the ministack container",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if config == nil {
			return fmt.Errorf("not initialized, run 'miniaws init' first")
		}

		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return fmt.Errorf("failed to connect to Docker: %w", err)
		}

		if err := cli.ContainerStop(ctx, config.ContainerName, container.StopOptions{}); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}

		fmt.Printf("%s Container '%s' stopped\n", containerSuccess.Render("✓"), config.ContainerName)
		return nil
	},
}

func init() {
	containerCmd.AddCommand(statusCmd)
	containerCmd.AddCommand(startCmd)
	containerCmd.AddCommand(stopCmd)
	containerCmd.AddCommand(containerRemoveCmd)

	containerRemoveCmd.Flags().BoolP("force", "f", false, "Force remove even if running")
}
