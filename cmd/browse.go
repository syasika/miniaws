package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"

	"github.com/syasika/miniaws/internal/awsclient"
	"github.com/syasika/miniaws/internal/s3ops"
	"github.com/syasika/miniaws/internal/ssmops"
)

var browseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Interactive TUI dashboard for ministack",
	Long:  `Launch a terminal UI to browse container status, S3 buckets and their objects.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		p := tea.NewProgram(initialModel(endpoint), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

type viewMode int

const (
	viewBuckets viewMode = iota
	viewObjects
	viewUpload
	viewSSM
)

type status string

const (
	statusLoading status = "loading"
	statusReady   status = "ready"
)

type model struct {
	state    status
	endpoint string

	containerStatus string
	containerName   string

	// S3 fields
	buckets    []string
	bucketsErr string
	cursor     int

	mode          viewMode
	currentBucket string
	objects       []s3ops.Object
	objectsErr    string
	objCursor     int

	// SSM fields
	params        []ssmParam
	paramsErr     string
	paramCursor   int
	requestToken  string
	nextToken     string
	prevTokens    []string
	pageLabel     string

	uploadInput textinput.Model

	statusLine string
	width      int
	height     int
}

type ssmParam struct {
	Name         string
	Type         string
	Value        string
	Version      int64
}

func initialModel(endpoint string) model {
	ti := textinput.New()
	ti.Placeholder = "path/to/file.txt"
	ti.Width = 50
	ti.CharLimit = 256

	return model{
		state:       statusLoading,
		endpoint:    endpoint,
		uploadInput: ti,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchContainerStatus, func() tea.Msg { return fetchBuckets(m.endpoint) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.mode == viewUpload {
			switch msg.String() {
			case "esc":
				m.mode = viewObjects
				m.uploadInput.SetValue("")
				m.uploadInput.Blur()
				return m, nil
			case "enter":
				path := m.uploadInput.Value()
				m.mode = viewObjects
				m.uploadInput.SetValue("")
				m.uploadInput.Blur()
				if path == "" {
					m.statusLine = "Upload cancelled"
					return m, nil
				}
				m.statusLine = fmt.Sprintf("Uploading %s...", path)
				return m, func() tea.Msg { return doUpload(m.endpoint, m.currentBucket, path) }
			default:
				var cmd tea.Cmd
				m.uploadInput, cmd = m.uploadInput.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.mode == viewObjects {
				m.mode = viewBuckets
				m.objects = nil
				m.objectsErr = ""
				m.statusLine = ""
			} else {
				return m, tea.Quit
			}
			return m, nil

		case "up", "k":
			if m.mode == viewBuckets && m.cursor > 0 {
				m.cursor--
			} else if m.mode == viewObjects && m.objCursor > 0 {
				m.objCursor--
			}

		case "down", "j":
			if m.mode == viewBuckets && m.cursor < len(m.buckets)-1 {
				m.cursor++
			} else if m.mode == viewObjects && m.objCursor < len(m.objects)-1 {
				m.objCursor++
			}

		case "1":
			m.mode = viewBuckets
			m.buckets = nil
			m.bucketsErr = ""
			m.state = statusLoading
			return m, tea.Batch(fetchContainerStatus, func() tea.Msg { return fetchBuckets(m.endpoint) })

		case "2":
			m.mode = viewSSM
			m.params = nil
			m.paramsErr = ""
			m.requestToken = ""
			m.nextToken = ""
			m.prevTokens = nil
			m.state = statusLoading
			return m, tea.Batch(fetchContainerStatus, func() tea.Msg { return fetchParams(m.endpoint, "") })

		case "[", "left":
			if m.mode == viewSSM && len(m.prevTokens) > 0 {
				m.nextToken = m.requestToken
				m.requestToken = m.prevTokens[len(m.prevTokens)-1]
				m.prevTokens = m.prevTokens[:len(m.prevTokens)-1]
				m.state = statusLoading
				m.paramsErr = ""
				return m, func() tea.Msg { return fetchParams(m.endpoint, m.requestToken) }
			}

		case "]", "right":
			if m.mode == viewSSM && m.nextToken != "" {
				m.prevTokens = append(m.prevTokens, m.requestToken)
				m.requestToken = m.nextToken
				m.nextToken = ""
				m.state = statusLoading
				m.paramsErr = ""
				return m, func() tea.Msg { return fetchParams(m.endpoint, m.requestToken) }
			}

		case "enter":
			if m.mode == viewBuckets && m.cursor < len(m.buckets) {
				m.currentBucket = m.buckets[m.cursor]
				m.mode = viewObjects
				m.objects = nil
				m.objectsErr = ""
				m.objCursor = 0
				m.statusLine = "Loading objects..."
				return m, func() tea.Msg { return fetchObjects(m.endpoint, m.currentBucket) }
			}
			if m.mode == viewSSM && m.paramCursor < len(m.params) {
				p := m.params[m.paramCursor]
				m.statusLine = fmt.Sprintf("Getting value for %s...", p.Name)
				return m, func() tea.Msg { return fetchParamValue(m.endpoint, p.Name) }
			}

		case "u":
			if m.mode == viewObjects {
				m.uploadInput.Focus()
				m.mode = viewUpload
				return m, textinput.Blink
			}

		case "d":
			if m.mode == viewObjects && m.objCursor < len(m.objects) {
				obj := m.objects[m.objCursor]
				if !strings.HasSuffix(obj.Key, "/") {
					m.statusLine = fmt.Sprintf("Downloading %s...", obj.Key)
					return m, func() tea.Msg { return doDownload(m.endpoint, m.currentBucket, obj.Key) }
				}
			}

		case "delete", "backspace":
			if m.mode == viewObjects && m.objCursor < len(m.objects) {
				obj := m.objects[m.objCursor]
				if !strings.HasSuffix(obj.Key, "/") {
					m.statusLine = fmt.Sprintf("Deleting %s...", obj.Key)
					return m, func() tea.Msg { return doDelete(m.endpoint, m.currentBucket, obj.Key) }
				}
			}
			if m.mode == viewSSM && m.paramCursor < len(m.params) {
				p := m.params[m.paramCursor]
				m.statusLine = fmt.Sprintf("Deleting %s...", p.Name)
				return m, func() tea.Msg { return doDeleteParam(m.endpoint, p.Name) }
			}

		case "r":
			m.state = statusLoading
			if m.mode == viewSSM {
				m.nextToken = ""
				m.prevTokens = nil
				return m, tea.Batch(fetchContainerStatus, func() tea.Msg { return fetchParams(m.endpoint, "") })
			}
			if m.mode == viewBuckets || m.mode == viewObjects {
				return m, tea.Batch(fetchContainerStatus, func() tea.Msg { return fetchBuckets(m.endpoint) })
			}
			return m, tea.Batch(fetchContainerStatus, func() tea.Msg { return fetchObjects(m.endpoint, m.currentBucket) })

		case "s":
			if !strings.Contains(m.containerStatus, "running") {
				m.statusLine = "Starting container..."
				return m, startContainer
			}

		case "x":
			if strings.Contains(m.containerStatus, "running") {
				m.statusLine = "Stopping container..."
				return m, stopContainer
			}
		}

	case containerStatusMsg:
		m.containerStatus = msg.status
		m.containerName = msg.name
		m.state = statusReady

	case bucketsMsg:
		m.buckets = msg.buckets
		m.bucketsErr = ""
		m.cursor = 0
		m.state = statusReady

	case bucketsErrMsg:
		m.bucketsErr = msg.err
		m.buckets = nil
		m.state = statusReady

	case paramsMsg:
		m.params = msg.params
		m.requestToken = msg.requestToken
		m.nextToken = msg.nextToken
		m.pageLabel = msg.label
		m.paramsErr = ""
		m.paramCursor = 0
		m.state = statusReady

	case paramsErrMsg:
		m.paramsErr = msg.err
		m.params = nil
		m.state = statusReady

	case paramValueMsg:
		m.statusLine = msg.desc

	case objectsMsg:
		m.objects = msg.objects
		m.objectsErr = ""
		m.objCursor = 0
		m.statusLine = ""
		m.state = statusReady

	case objectsErrMsg:
		m.objectsErr = msg.err
		m.objects = nil
		m.statusLine = ""
		m.state = statusReady

	case resultMsg:
		m.statusLine = msg.desc
		if msg.refresh {
			return m, func() tea.Msg { return fetchObjects(m.endpoint, m.currentBucket) }
		}

	case actionMsg:
		m.statusLine = msg.description
		return m, tea.Batch(fetchContainerStatus, func() tea.Msg { return fetchBuckets(m.endpoint) })

	case errMsg:
		m.statusLine = fmt.Sprintf("Error: %v", msg.err)
		m.state = statusReady
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		m.width = 80
	}
	switch m.state {
	case statusLoading:
		return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Padding(1).Render("Loading ministack info...")
	case statusReady:
		return m.dashboardView()
	default:
		return "unknown state"
	}
}

func (m model) dashboardView() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Width(m.width).
		Align(lipgloss.Center).
		Padding(0, 1)

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("33")).
		Padding(0, 1)

	b.WriteString(headerStyle.Render("☁  miniaws — ministack dashboard"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("Container"))
	b.WriteString("\n")

	statusColor := lipgloss.Color("10")
	statusLabel := "● running"
	if !strings.Contains(m.containerStatus, "running") {
		statusColor = lipgloss.Color("11")
		statusLabel = "● " + m.containerStatus
	}
	b.WriteString(fmt.Sprintf("  %s  ", lipgloss.NewStyle().Foreground(statusColor).Render(statusLabel)))
	b.WriteString(fmt.Sprintf("(%s)\n\n", m.containerName))

	if m.mode == viewSSM {
		sectionLabel := "SSM Parameters"
		if m.pageLabel != "" {
			sectionLabel += "  " + m.pageLabel
		}
		b.WriteString(sectionStyle.Render(sectionLabel))
		b.WriteString("\n")

		if m.paramsErr != "" {
			b.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(m.paramsErr)))
		} else if len(m.params) == 0 {
			b.WriteString("  (empty)\n")
		} else {
			activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
			inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
			typeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
			for i, p := range m.params {
				cursor := " "
				style := inactiveStyle
				if i == m.paramCursor {
					cursor = "▸"
					style = activeStyle
				}
				b.WriteString(fmt.Sprintf("  %s %s  %s\n", cursor, style.Render(p.Name), typeStyle.Render(fmt.Sprintf("(%s, v%d)", p.Type, p.Version))))
			}
		}
		b.WriteString("\n")
	} else if m.mode == viewObjects && m.currentBucket != "" {
		b.WriteString(sectionStyle.Render("Objects — " + m.currentBucket))
		b.WriteString("\n")

		if m.objectsErr != "" {
			b.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(m.objectsErr)))
		} else if len(m.objects) == 0 {
			b.WriteString("  (empty)\n")
		} else {
			activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
			inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
			objKeyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
			folderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
			szStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

			for i, obj := range m.objects {
				cursor := " "
				style := inactiveStyle
				if i == m.objCursor {
					cursor = "▸"
					style = activeStyle
				}
				icon := "📄"
				keyStyle := objKeyStyle
				if strings.HasSuffix(obj.Key, "/") {
					icon = "📁"
					keyStyle = folderStyle
				}
				line := fmt.Sprintf("  %s %s", cursor, style.Render(fmt.Sprintf("%s %s", icon, keyStyle.Render(obj.Key))))
				if obj.Size > 0 && !strings.HasSuffix(obj.Key, "/") {
					line += szStyle.Render(fmt.Sprintf("  (%d bytes)", obj.Size))
				}
				b.WriteString(line + "\n")
			}
		}
		b.WriteString("\n")
	} else {
		b.WriteString(sectionStyle.Render("S3 Buckets"))
		b.WriteString("\n")

		if m.bucketsErr != "" {
			b.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(m.bucketsErr)))
		} else if len(m.buckets) == 0 {
			b.WriteString("  No buckets.\n")
		} else {
			activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
			inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
			for i, bucket := range m.buckets {
				cursor := " "
				style := inactiveStyle
				if i == m.cursor {
					cursor = "▸"
					style = activeStyle
				}
				b.WriteString(fmt.Sprintf("  %s %s\n", cursor, style.Render(bucket)))
			}
		}
		b.WriteString("\n")
	}

	if m.mode == viewUpload {
		b.WriteString(fmt.Sprintf("  Upload path: %s\n\n", m.uploadInput.View()))
	}

	if m.statusLine != "" {
		statusLineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Italic(true)
		b.WriteString(fmt.Sprintf("  %s\n\n", statusLineStyle.Render(m.statusLine)))
	}

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	serviceBar := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render(" [1] S3  [2] SSM ")
	b.WriteString(fmt.Sprintf("  %s\n\n", serviceBar))

	help := "  ↑/↓ navigate"
	if m.mode == viewBuckets {
		help += " · enter browse · r refresh"
	} else if m.mode == viewSSM {
		help += " · enter value · del delete · [ ] page · r refresh"
	} else if m.mode == viewObjects {
		help += " · u upload · d download · del delete · esc back · r refresh"
	} else if m.mode == viewUpload {
		help += " · enter confirm · esc cancel"
	}
	if strings.Contains(m.containerStatus, "running") {
		help += " · x stop"
	} else if m.containerStatus != "not initialized" && m.containerStatus != "not found" {
		help += " · s start"
	}
	help += " · q quit"
	b.WriteString(helpStyle.Render(help))
	b.WriteString("\n")

	return b.String()
}

type containerStatusMsg struct {
	status string
	name   string
}

type bucketsMsg struct {
	buckets []string
}

type bucketsErrMsg struct {
	err string
}

type objectsMsg struct {
	objects []s3ops.Object
}

type objectsErrMsg struct {
	err string
}

type resultMsg struct {
	desc    string
	refresh bool
}

type actionMsg struct {
	description string
}

type errMsg struct {
	err error
}

type paramsMsg struct {
	params       []ssmParam
	requestToken string
	nextToken    string
	label        string
}

type paramsErrMsg struct {
	err string
}

type paramValueMsg struct {
	desc string
}

func fetchContainerStatus() tea.Msg {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return errMsg{err}
	}

	config, err := LoadConfig()
	if err != nil {
		return errMsg{err}
	}
	if config == nil {
		return containerStatusMsg{status: "not initialized", name: "-"}
	}

	ci, err := cli.ContainerInspect(context.Background(), config.ContainerName)
	if err != nil {
		return containerStatusMsg{status: "not found", name: config.ContainerName}
	}

	return containerStatusMsg{status: ci.State.Status, name: config.ContainerName}
}

func startContainer() tea.Msg {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to connect: %v", err)}
	}
	config, err := LoadConfig()
	if err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to load config: %v", err)}
	}
	if config == nil {
		return actionMsg{description: "Not initialized — run 'miniaws init' first"}
	}
	if err := cli.ContainerStart(context.Background(), config.ContainerName, container.StartOptions{}); err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to start: %v", err)}
	}
	return actionMsg{description: fmt.Sprintf("Container '%s' started", config.ContainerName)}
}

func stopContainer() tea.Msg {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to connect: %v", err)}
	}
	config, err := LoadConfig()
	if err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to load config: %v", err)}
	}
	if config == nil {
		return actionMsg{description: "Not initialized"}
	}
	if err := cli.ContainerStop(context.Background(), config.ContainerName, container.StopOptions{}); err != nil {
		return actionMsg{description: fmt.Sprintf("Failed to stop: %v", err)}
	}
	return actionMsg{description: fmt.Sprintf("Container '%s' stopped", config.ContainerName)}
}

func fetchBuckets(endpoint string) tea.Msg {
	client := awsclient.NewS3Client(awsclient.NewConfig(), endpoint)
	names, err := s3ops.ListBuckets(context.Background(), client)
	if err != nil {
		if s3ops.IsConnectionErr(err) {
			return bucketsErrMsg{err: "Cannot reach ministack — is the container running?"}
		}
		return bucketsErrMsg{err: fmt.Sprintf("S3 error: %v", err)}
	}
	return bucketsMsg{buckets: names}
}

func fetchObjects(endpoint, bucket string) tea.Msg {
	client := awsclient.NewS3Client(awsclient.NewConfig(), endpoint)
	items, err := s3ops.ListObjects(context.Background(), client, bucket)
	if err != nil {
		if s3ops.IsConnectionErr(err) {
			return objectsErrMsg{err: "Cannot reach ministack — is the container running?"}
		}
		return objectsErrMsg{err: fmt.Sprintf("S3 error: %v", err)}
	}
	return objectsMsg{objects: items}
}

func doUpload(endpoint, bucket, localPath string) tea.Msg {
	client := awsclient.NewS3Client(awsclient.NewConfig(), endpoint)
	key := filepath.Base(localPath)
	if err := s3ops.UploadFile(context.Background(), client, bucket, key, localPath); err != nil {
		return resultMsg{desc: fmt.Sprintf("Upload failed: %v", err)}
	}
	return resultMsg{desc: fmt.Sprintf("Uploaded %s → s3://%s/%s", localPath, bucket, key), refresh: true}
}

func doDownload(endpoint, bucket, key string) tea.Msg {
	client := awsclient.NewS3Client(awsclient.NewConfig(), endpoint)
	written, err := s3ops.DownloadFile(context.Background(), client, bucket, key, ".")
	if err != nil {
		return resultMsg{desc: fmt.Sprintf("Download failed: %v", err)}
	}
	localName := filepath.Base(key)
	return resultMsg{desc: fmt.Sprintf("Downloaded s3://%s/%s → %s (%d bytes)", bucket, key, localName, written)}
}

func doDelete(endpoint, bucket, key string) tea.Msg {
	client := awsclient.NewS3Client(awsclient.NewConfig(), endpoint)
	if err := s3ops.DeleteObject(context.Background(), client, bucket, key); err != nil {
		return resultMsg{desc: fmt.Sprintf("Delete failed: %v", err)}
	}
	return resultMsg{desc: fmt.Sprintf("Deleted s3://%s/%s", bucket, key), refresh: true}
}

func fetchParams(endpoint, requestToken string) tea.Msg {
	client := awsclient.NewSSMClient(awsclient.NewConfig(), endpoint)
	var tok *string
	if requestToken != "" {
		tok = &requestToken
	}
	page, err := ssmops.ListParameters(context.Background(), client, tok, 20)
	if err != nil {
		if ssmops.IsConnectionErr(err) {
			return paramsErrMsg{err: "Cannot reach ministack — is the container running?"}
		}
		return paramsErrMsg{err: fmt.Sprintf("SSM error: %v", err)}
	}
	params := make([]ssmParam, len(page.Parameters))
	for i, p := range page.Parameters {
		params[i] = ssmParam{Name: p.Name, Type: p.Type, Version: p.Version}
	}
	var nt string
	if page.NextToken != nil {
		nt = *page.NextToken
	}
	label := fmt.Sprintf("(%d)", len(params))
	if nt != "" {
		label += " [ more ]"
	}
	return paramsMsg{params: params, requestToken: requestToken, nextToken: nt, label: label}
}

func fetchParamValue(endpoint, name string) tea.Msg {
	client := awsclient.NewSSMClient(awsclient.NewConfig(), endpoint)
	p, err := ssmops.GetParameter(context.Background(), client, name)
	if err != nil {
		return paramValueMsg{desc: fmt.Sprintf("Get failed: %v", err)}
	}
	return paramValueMsg{desc: fmt.Sprintf("%s = %s  (%s, v%d)", p.Name, p.Value, p.Type, p.Version)}
}

func doDeleteParam(endpoint, name string) tea.Msg {
	client := awsclient.NewSSMClient(awsclient.NewConfig(), endpoint)
	if err := ssmops.DeleteParameter(context.Background(), client, name); err != nil {
		return resultMsg{desc: fmt.Sprintf("Delete failed: %v", err)}
	}
	return resultMsg{desc: fmt.Sprintf("Deleted parameter '%s'", name), refresh: true}
}

func init() {
	rootCmd.AddCommand(browseCmd)
}
