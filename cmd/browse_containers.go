package cmd

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func fetchContainerStatus(ctx context.Context, cli *client.Client) tea.Msg {
	if cli == nil {
		return containerStatusMsg{status: "Docker unavailable", name: "-"}
	}
	config, err := LoadConfig()
	if err != nil {
		return errMsg{err}
	}
	if config == nil {
		return containerStatusMsg{status: "not initialized", name: "-"}
	}
	ci, err := cli.ContainerInspect(ctx, config.ContainerName)
	if err != nil {
		return containerStatusMsg{status: "not found", name: config.ContainerName}
	}
	return containerStatusMsg{status: ci.State.Status, name: config.ContainerName}
}

func startContainer(ctx context.Context, cli *client.Client) tea.Msg {
	if cli == nil {
		return actionMsg{description: "Docker unavailable"}
	}
	config, err := LoadConfig()
	if err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to load config: %v", err)}
	}
	if config == nil {
		return actionMsg{description: "Not initialized — run 'miniaws init' first"}
	}
	if err := cli.ContainerStart(ctx, config.ContainerName, container.StartOptions{}); err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to start: %v", err)}
	}
	return actionMsg{description: fmt.Sprintf("Container '%s' started", config.ContainerName)}
}

func stopContainer(ctx context.Context, cli *client.Client) tea.Msg {
	if cli == nil {
		return actionMsg{description: "Docker unavailable"}
	}
	config, err := LoadConfig()
	if err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to load config: %v", err)}
	}
	if config == nil {
		return actionMsg{description: "Not initialized"}
	}
	if err := cli.ContainerStop(ctx, config.ContainerName, container.StopOptions{}); err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to stop: %v", err)}
	}
	return actionMsg{description: fmt.Sprintf("Container '%s' stopped", config.ContainerName)}
}
