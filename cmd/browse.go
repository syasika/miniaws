package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"

	"github.com/syasika/miniaws/internal/awsclient"
	"github.com/syasika/miniaws/internal/s3ops"
	"github.com/syasika/miniaws/internal/sqsops"
)

var browseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Interactive TUI dashboard for ministack",
	Long:  `Launch a terminal UI to browse container status, S3 buckets, SSM parameters, and SQS queues.`,
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
	viewSQS
	viewQueueMessages
	viewCreateQueue
	viewSendMessage
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

	// SQS fields
	queues          []sqsops.Queue
	queuesErr       string
	queueCursor     int
	currentQueueURL string
	messages        []sqsops.Message
	messagesErr     string
	msgCursor       int

	uploadInput textinput.Model
	createInput textinput.Model

	awsCfg     aws.Config
	dockerCli  *client.Client
	confirming *confirmState
	ctx        context.Context
	cancel     context.CancelFunc

	statusLine string
	width      int
	height     int
}

type ssmParam struct {
	Name    string
	Type    string
	Value   string
	Version int64
}

type confirmState struct {
	prompt string
	onYes  tea.Cmd
}

func initialModel(endpoint string) model {
	ti := textinput.New()
	ti.Placeholder = "path/to/file.txt"
	ti.Width = 50
	ti.CharLimit = 256

	ci := textinput.New()
	ci.Placeholder = "queue-name"
	ci.Width = 50
	ci.CharLimit = 256

	dockerCli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	ctx, cancel := context.WithCancel(context.Background())

	return model{
		state:       statusLoading,
		endpoint:    endpoint,
		uploadInput: ti,
		createInput: ci,
		awsCfg:      awsclient.NewConfig(),
		dockerCli:   dockerCli,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
		func() tea.Msg { return fetchBuckets(m.ctx, m.awsCfg, m.endpoint) },
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.confirming != nil {
			switch msg.String() {
			case "y", "Y":
				cmd := m.confirming.onYes
				m.confirming = nil
				return m, cmd
			default:
				m.confirming = nil
				m.statusLine = "Cancelled"
				return m, nil
			}
		}

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
				return m, func() tea.Msg { return doUpload(m.ctx, m.awsCfg, m.endpoint, m.currentBucket, path) }
			default:
				var cmd tea.Cmd
				m.uploadInput, cmd = m.uploadInput.Update(msg)
				return m, cmd
			}
		}

		if m.mode == viewCreateQueue {
			switch msg.String() {
			case "esc":
				m.mode = viewSQS
				m.createInput.SetValue("")
				m.createInput.Blur()
				return m, nil
			case "enter":
				name := m.createInput.Value()
				m.mode = viewSQS
				m.createInput.SetValue("")
				m.createInput.Blur()
				if name == "" {
					m.statusLine = "Create cancelled"
					return m, nil
				}
				m.statusLine = fmt.Sprintf("Creating queue %s...", name)
				return m, func() tea.Msg { return doCreateQueue(m.ctx, m.awsCfg, m.endpoint, name) }
			default:
				var cmd tea.Cmd
				m.createInput, cmd = m.createInput.Update(msg)
				return m, cmd
			}
		}

		if m.mode == viewSendMessage {
			switch msg.String() {
			case "esc":
				m.mode = viewQueueMessages
				m.uploadInput.SetValue("")
				m.uploadInput.Blur()
				return m, nil
			case "enter":
				body := m.uploadInput.Value()
				m.mode = viewQueueMessages
				m.uploadInput.SetValue("")
				m.uploadInput.Blur()
				if body == "" {
					m.statusLine = "Send cancelled"
					return m, nil
				}
				m.statusLine = "Sending message..."
				return m, func() tea.Msg { return doSendMessage(m.ctx, m.awsCfg, m.endpoint, m.currentQueueURL, body) }
			default:
				var cmd tea.Cmd
				m.uploadInput, cmd = m.uploadInput.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit

		case "esc":
			switch m.mode {
			case viewObjects:
				m.mode = viewBuckets
				m.objects = nil
				m.objectsErr = ""
				m.statusLine = ""
			case viewQueueMessages:
				m.mode = viewSQS
				m.messages = nil
				m.messagesErr = ""
				m.statusLine = ""
			default:
				return m, tea.Quit
			}
			return m, nil

		case "up", "k":
			switch m.mode {
			case viewBuckets:
				if m.cursor > 0 {
					m.cursor--
				}
			case viewObjects:
				if m.objCursor > 0 {
					m.objCursor--
				}
			case viewSQS:
				if m.queueCursor > 0 {
					m.queueCursor--
				}
			case viewQueueMessages:
				if m.msgCursor > 0 {
					m.msgCursor--
				}
			}

		case "down", "j":
			switch m.mode {
			case viewBuckets:
				if m.cursor < len(m.buckets)-1 {
					m.cursor++
				}
			case viewObjects:
				if m.objCursor < len(m.objects)-1 {
					m.objCursor++
				}
			case viewSQS:
				if m.queueCursor < len(m.queues)-1 {
					m.queueCursor++
				}
			case viewQueueMessages:
				if m.msgCursor < len(m.messages)-1 {
					m.msgCursor++
				}
			}

		case "1":
			m.mode = viewBuckets
			m.buckets = nil
			m.bucketsErr = ""
			m.state = statusLoading
			return m, tea.Batch(
				func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
				func() tea.Msg { return fetchBuckets(m.ctx, m.awsCfg, m.endpoint) },
			)

		case "2":
			m.mode = viewSSM
			m.params = nil
			m.paramsErr = ""
			m.requestToken = ""
			m.nextToken = ""
			m.prevTokens = nil
			m.state = statusLoading
			return m, tea.Batch(
				func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
				func() tea.Msg { return fetchParams(m.ctx, m.awsCfg, m.endpoint, "") },
			)

		case "3":
			m.mode = viewSQS
			m.queues = nil
			m.queuesErr = ""
			m.state = statusLoading
			return m, tea.Batch(
				func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
				func() tea.Msg { return fetchQueues(m.ctx, m.awsCfg, m.endpoint) },
			)

		case "[", "left":
			if m.mode == viewSSM && len(m.prevTokens) > 0 {
				m.nextToken = m.requestToken
				m.requestToken = m.prevTokens[len(m.prevTokens)-1]
				m.prevTokens = m.prevTokens[:len(m.prevTokens)-1]
				m.state = statusLoading
				m.paramsErr = ""
				return m, func() tea.Msg { return fetchParams(m.ctx, m.awsCfg, m.endpoint, m.requestToken) }
			}

		case "]", "right":
			if m.mode == viewSSM && m.nextToken != "" {
				m.prevTokens = append(m.prevTokens, m.requestToken)
				m.requestToken = m.nextToken
				m.nextToken = ""
				m.state = statusLoading
				m.paramsErr = ""
				return m, func() tea.Msg { return fetchParams(m.ctx, m.awsCfg, m.endpoint, m.requestToken) }
			}

		case "enter":
			if m.mode == viewBuckets && m.cursor < len(m.buckets) {
				m.currentBucket = m.buckets[m.cursor]
				m.mode = viewObjects
				m.objects = nil
				m.objectsErr = ""
				m.objCursor = 0
				m.statusLine = "Loading objects..."
				return m, func() tea.Msg { return fetchObjects(m.ctx, m.awsCfg, m.endpoint, m.currentBucket) }
			}
			if m.mode == viewSSM && m.paramCursor < len(m.params) {
				p := m.params[m.paramCursor]
				m.statusLine = fmt.Sprintf("Getting value for %s...", p.Name)
				return m, func() tea.Msg { return fetchParamValue(m.ctx, m.awsCfg, m.endpoint, p.Name) }
			}
			if m.mode == viewSQS && m.queueCursor < len(m.queues) {
				q := m.queues[m.queueCursor]
				m.currentQueueURL = q.URL
				m.mode = viewQueueMessages
				m.messages = nil
				m.messagesErr = ""
				m.msgCursor = 0
				m.statusLine = "Receiving messages..."
				return m, func() tea.Msg { return fetchMessages(m.ctx, m.awsCfg, m.endpoint, q.URL) }
			}
			if m.mode == viewQueueMessages && m.msgCursor < len(m.messages) {
				msg := m.messages[m.msgCursor]
				body := msg.Body
				if len(body) > 200 {
					body = body[:200] + "..."
				}
				m.statusLine = fmt.Sprintf("[%s] %s", msg.ID, body)
				return m, nil
			}

		case "u":
			if m.mode == viewObjects {
				m.uploadInput.Placeholder = "path/to/file.txt"
				m.uploadInput.Focus()
				m.mode = viewUpload
				return m, textinput.Blink
			}

		case "d":
			if m.mode == viewObjects && m.objCursor < len(m.objects) {
				obj := m.objects[m.objCursor]
				if !strings.HasSuffix(obj.Key, "/") {
					m.statusLine = fmt.Sprintf("Downloading %s...", obj.Key)
					return m, func() tea.Msg { return doDownload(m.ctx, m.awsCfg, m.endpoint, m.currentBucket, obj.Key) }
				}
			}

		case "c":
			if m.mode == viewSQS {
				m.createInput.Focus()
				m.mode = viewCreateQueue
				return m, textinput.Blink
			}

		case "delete", "backspace":
			if m.mode == viewObjects && m.objCursor < len(m.objects) {
				obj := m.objects[m.objCursor]
				if !strings.HasSuffix(obj.Key, "/") {
					m.confirming = &confirmState{
						prompt: fmt.Sprintf("Delete s3://%s/%s? (y/N)", m.currentBucket, obj.Key),
						onYes:  func() tea.Msg { return doDelete(m.ctx, m.awsCfg, m.endpoint, m.currentBucket, obj.Key) },
					}
					return m, nil
				}
			}
			if m.mode == viewSSM && m.paramCursor < len(m.params) {
				p := m.params[m.paramCursor]
				m.confirming = &confirmState{
					prompt: fmt.Sprintf("Delete parameter '%s'? (y/N)", p.Name),
					onYes:  func() tea.Msg { return doDeleteParam(m.ctx, m.awsCfg, m.endpoint, p.Name) },
				}
				return m, nil
			}
			if m.mode == viewSQS && m.queueCursor < len(m.queues) {
				q := m.queues[m.queueCursor]
				m.confirming = &confirmState{
					prompt: fmt.Sprintf("Delete queue '%s'? (y/N)", q.Name),
					onYes:  func() tea.Msg { return doDeleteQueue(m.ctx, m.awsCfg, m.endpoint, q.URL) },
				}
				return m, nil
			}
			if m.mode == viewQueueMessages && m.msgCursor < len(m.messages) {
				msg := m.messages[m.msgCursor]
				m.confirming = &confirmState{
					prompt: fmt.Sprintf("Delete message %s? (y/N)", msg.ID),
					onYes:  func() tea.Msg { return doDeleteMessage(m.ctx, m.awsCfg, m.endpoint, m.currentQueueURL, msg.ReceiptHandle) },
				}
				return m, nil
			}

		case "r":
			m.state = statusLoading
			switch m.mode {
			case viewSSM:
				m.nextToken = ""
				m.prevTokens = nil
				return m, tea.Batch(
					func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
					func() tea.Msg { return fetchParams(m.ctx, m.awsCfg, m.endpoint, "") },
				)
			case viewSQS:
				return m, tea.Batch(
					func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
					func() tea.Msg { return fetchQueues(m.ctx, m.awsCfg, m.endpoint) },
				)
			case viewQueueMessages:
				return m, tea.Batch(
					func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
					func() tea.Msg { return fetchMessages(m.ctx, m.awsCfg, m.endpoint, m.currentQueueURL) },
				)
			case viewBuckets, viewObjects:
				return m, tea.Batch(
					func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
					func() tea.Msg { return fetchBuckets(m.ctx, m.awsCfg, m.endpoint) },
				)
			default:
				return m, tea.Batch(
					func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
					func() tea.Msg { return fetchObjects(m.ctx, m.awsCfg, m.endpoint, m.currentBucket) },
				)
			}

		case "s":
			if m.mode == viewQueueMessages {
				m.uploadInput.Placeholder = "message body"
				m.uploadInput.Focus()
				m.mode = viewSendMessage
				return m, textinput.Blink
			}
			if !strings.Contains(m.containerStatus, "running") {
				m.statusLine = "Starting container..."
				return m, func() tea.Msg { return startContainer(m.ctx, m.dockerCli) }
			}

		case "x":
			if strings.Contains(m.containerStatus, "running") {
				m.confirming = &confirmState{
					prompt: fmt.Sprintf("Stop container '%s'? (y/N)", m.containerName),
					onYes:  func() tea.Msg { return stopContainer(m.ctx, m.dockerCli) },
				}
				return m, nil
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

	case queuesMsg:
		m.queues = msg.queues
		m.queuesErr = ""
		m.queueCursor = 0
		m.state = statusReady

	case queuesErrMsg:
		m.queuesErr = msg.err
		m.queues = nil
		m.state = statusReady

	case messagesMsg:
		m.messages = msg.messages
		m.messagesErr = ""
		m.msgCursor = 0
		m.statusLine = ""
		m.state = statusReady

	case messagesErrMsg:
		m.messagesErr = msg.err
		m.messages = nil
		m.statusLine = ""
		m.state = statusReady

	case resultMsg:
		m.statusLine = msg.desc
		if msg.refresh {
			switch m.mode {
			case viewSSM:
				return m, func() tea.Msg { return fetchParams(m.ctx, m.awsCfg, m.endpoint, "") }
			case viewBuckets:
				return m, func() tea.Msg { return fetchBuckets(m.ctx, m.awsCfg, m.endpoint) }
			case viewQueueMessages:
				return m, func() tea.Msg { return fetchMessages(m.ctx, m.awsCfg, m.endpoint, m.currentQueueURL) }
			case viewObjects:
				return m, func() tea.Msg { return fetchObjects(m.ctx, m.awsCfg, m.endpoint, m.currentBucket) }
			case viewSQS:
				return m, func() tea.Msg { return fetchQueues(m.ctx, m.awsCfg, m.endpoint) }
			default:
				return m, func() tea.Msg { return fetchQueues(m.ctx, m.awsCfg, m.endpoint) }
			}
		}

	case actionMsg:
		m.statusLine = msg.description
		return m, tea.Batch(
			func() tea.Msg { return fetchContainerStatus(m.ctx, m.dockerCli) },
			func() tea.Msg { return fetchCurrentView(m.ctx, m.awsCfg, m.endpoint, m.mode, m.currentBucket, m.currentQueueURL) },
		)

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
	return m.dashboardView()
}

func init() {
	rootCmd.AddCommand(browseCmd)
}
