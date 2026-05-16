package browse

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"github.com/syasika/miniaws/internal/config"
)

func fetchContainerStatus(ctx context.Context, cli *client.Client) tea.Msg {
	if cli == nil {
		return containerStatusMsg{status: "Docker unavailable", name: "-"}
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return errMsg{err}
	}
	if cfg == nil {
		return containerStatusMsg{status: "not initialized", name: "-"}
	}
	ci, err := cli.ContainerInspect(ctx, cfg.ContainerName)
	if err != nil {
		return containerStatusMsg{status: "not found", name: cfg.ContainerName}
	}
	return containerStatusMsg{status: ci.State.Status, name: cfg.ContainerName}
}

func startContainer(ctx context.Context, cli *client.Client) tea.Msg {
	if cli == nil {
		return actionMsg{description: "Docker unavailable"}
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to load config: %v", err)}
	}
	if cfg == nil {
		return actionMsg{description: "Not initialized — run 'miniaws init' first"}
	}
	if err := cli.ContainerStart(ctx, cfg.ContainerName, container.StartOptions{}); err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to start: %v", err)}
	}
	return actionMsg{description: fmt.Sprintf("Container '%s' started", cfg.ContainerName)}
}

func stopContainer(ctx context.Context, cli *client.Client) tea.Msg {
	if cli == nil {
		return actionMsg{description: "Docker unavailable"}
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to load config: %v", err)}
	}
	if cfg == nil {
		return actionMsg{description: "Not initialized"}
	}
	if err := cli.ContainerStop(ctx, cfg.ContainerName, container.StopOptions{}); err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to stop: %v", err)}
	}
	return actionMsg{description: fmt.Sprintf("Container '%s' stopped", cfg.ContainerName)}
}
