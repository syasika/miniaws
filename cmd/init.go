package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/spf13/cobra"

	"github.com/syasika/miniaws/internal/config"
)

var initSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize miniaws: check/start ministack container and gather setup details",
	Long: `This command checks if a ministack container is running.
If not, it prompts for setup details and starts one.`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg != nil {
		if cfg.Port == "" {
			cfg.Port = "4566"
		}
		if cfg.EndpointURL == "" {
			cfg.EndpointURL = "http://localhost:" + cfg.Port
		}
		ci, err := inspectContainer(ctx, cli, cfg.ContainerName)
		if err != nil {
			if !cerrdefs.IsNotFound(err) {
				return fmt.Errorf("failed to inspect container: %w", err)
			}
		} else {
			switch ci.State.Status {
			case "running":
				fmt.Printf("%s Container '%s' is already running (image: %s)\n", initSuccess.Render("✓"), cfg.ContainerName, cfg.ImageName)
				return nil
			case "exited":
				fmt.Printf("Container '%s' exists but is stopped. Starting it...\n", cfg.ContainerName)
				if err := cli.ContainerStart(ctx, cfg.ContainerName, container.StartOptions{}); err != nil {
					return fmt.Errorf("failed to start container: %w", err)
				}
				fmt.Printf("%s Container '%s' started\n", initSuccess.Render("✓"), cfg.ContainerName)
				return nil
			case "paused":
				fmt.Printf("Container '%s' is paused. Unpausing...\n", cfg.ContainerName)
				if err := cli.ContainerUnpause(ctx, cfg.ContainerName); err != nil {
					return fmt.Errorf("failed to unpause container: %w", err)
				}
				fmt.Printf("%s Container '%s' unpaused\n", initSuccess.Render("✓"), cfg.ContainerName)
				return nil
			default:
				return fmt.Errorf("unexpected container state: %s", ci.State.Status)
			}
		}
	}

	fmt.Println("No running ministack container found. Let's set one up.")
	cfg, err = promptSetup()
	if err != nil {
		return fmt.Errorf("setup cancelled: %w", err)
	}

	if err := ensureContainer(ctx, cli, cfg); err != nil {
		return fmt.Errorf("failed to set up container: %w", err)
	}

	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("%s Container '%s' is running (image: %s)\n", initSuccess.Render("✓"), cfg.ContainerName, cfg.ImageName)
	return nil
}

func inspectContainer(ctx context.Context, cli *client.Client, name string) (container.InspectResponse, error) {
	return cli.ContainerInspect(ctx, name)
}

func promptSetup() (*config.Config, error) {
	reader := bufio.NewReader(os.Stdin)

	var containerName string
	for {
		fmt.Print("Container name [ministack]: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			containerName = "ministack"
		} else {
			containerName = input
		}
		if containerName != "" {
			break
		}
	}

	var imageName string
	for {
		fmt.Print("Docker image [ministackorg/ministack]: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			imageName = "ministackorg/ministack"
		} else {
			imageName = input
		}
		if imageName != "" {
			break
		}
	}

	port := "4566"
	endpointURL := "http://localhost:" + port

	return &config.Config{
		ContainerName: containerName,
		ImageName:     imageName,
		Port:          port,
		EndpointURL:   endpointURL,
	}, nil
}

func ensureContainer(ctx context.Context, cli *client.Client, cfg *config.Config) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		_, err := inspectContainer(ctx, cli, cfg.ContainerName)
		if err == nil {
			fmt.Printf("%s Container '%s' already exists. Starting it...\n", initSuccess.Render("✓"), cfg.ContainerName)
			return cli.ContainerStart(ctx, cfg.ContainerName, container.StartOptions{})
		}

		if !cerrdefs.IsNotFound(err) {
			return fmt.Errorf("failed to inspect container: %w", err)
		}

		fmt.Printf("Pulling image '%s'...\n", cfg.ImageName)
		pullReader, err := cli.ImagePull(ctx, cfg.ImageName, image.PullOptions{})
		if err != nil {
			fmt.Printf("⚠ Failed to pull image: %s\n", err)
			fmt.Print("Enter a different Docker image (or press Ctrl+C to cancel): ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				return fmt.Errorf("no image provided")
			}
			cfg.ImageName = input
			continue
		}
		_, err = io.Copy(io.Discard, pullReader)
		pullReader.Close()
		if err != nil {
			return fmt.Errorf("failed to pull image: %w", err)
		}

		fmt.Printf("Creating container '%s'...\n", cfg.ContainerName)

		if cfg.Port == "" {
			cfg.Port = "4566"
		}
		port := nat.Port(cfg.Port + "/tcp")
		resp, err := cli.ContainerCreate(ctx, &container.Config{
			Image:        cfg.ImageName,
			ExposedPorts: nat.PortSet{port: struct{}{}},
		}, &container.HostConfig{
			PortBindings: nat.PortMap{
				port: []nat.PortBinding{{HostPort: cfg.Port, HostIP: "0.0.0.0"}},
			},
		}, nil, nil, cfg.ContainerName)
		if err != nil {
			return fmt.Errorf("failed to create container: %w", err)
		}

		return cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
	}
}
