package browse

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/syasika/miniaws/internal/sqsops"
)

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

	title := "☁  miniaws — ministack dashboard"
	if m.state == statusLoading {
		title += "  ⟳ refreshing..."
	}
	b.WriteString(headerStyle.Render(title))
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

	switch {
	case m.mode == viewSSM:
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

	case m.mode == viewObjects && m.currentBucket != "":
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

	case m.mode == viewSQS || m.mode == viewQueueMessages || m.mode == viewCreateQueue || m.mode == viewSendMessage:
		if m.mode == viewQueueMessages && m.currentQueueURL != "" {
			qName := sqsops.ExtractQueueName(m.currentQueueURL)
			b.WriteString(sectionStyle.Render("Messages — " + qName))
			b.WriteString("\n")

			if m.messagesErr != "" {
				b.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(m.messagesErr)))
			} else if len(m.messages) == 0 {
				b.WriteString("  (empty)\n")
			} else {
				activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
				inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
				idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
				for i, msg := range m.messages {
					cursor := " "
					style := inactiveStyle
					if i == m.msgCursor {
						cursor = "▸"
						style = activeStyle
					}
					body := msg.Body
					if len(body) > 80 {
						body = body[:80] + "..."
					}
					b.WriteString(fmt.Sprintf("  %s %s  %s\n", cursor, style.Render(body), idStyle.Render(fmt.Sprintf("(%s)", msg.ID))))
				}
			}
			b.WriteString("\n")
		} else {
			b.WriteString(sectionStyle.Render("SQS Queues"))
			b.WriteString("\n")

			if m.queuesErr != "" {
				b.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(m.queuesErr)))
			} else if len(m.queues) == 0 {
				b.WriteString("  (empty)\n")
			} else {
				activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
				inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
				for i, q := range m.queues {
					cursor := " "
					style := inactiveStyle
					if i == m.queueCursor {
						cursor = "▸"
						style = activeStyle
					}
					b.WriteString(fmt.Sprintf("  %s 📦 %s\n", cursor, style.Render(q.Name)))
				}
			}
			b.WriteString("\n")
		}

	default:
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

	if m.confirming != nil {
		confirmStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
		b.WriteString(fmt.Sprintf("  %s  (y/N):  ", confirmStyle.Render(m.confirming.prompt)))
		b.WriteString("\n\n")
	} else if m.mode == viewUpload {
		b.WriteString(fmt.Sprintf("  Upload path: %s\n\n", m.uploadInput.View()))
	} else if m.mode == viewCreateQueue {
		b.WriteString(fmt.Sprintf("  Queue name: %s\n\n", m.createInput.View()))
	} else if m.mode == viewSendMessage {
		b.WriteString(fmt.Sprintf("  Message body: %s\n\n", m.uploadInput.View()))
	}

	if m.statusLine != "" {
		statusLineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Italic(true)
		b.WriteString(fmt.Sprintf("  %s\n\n", statusLineStyle.Render(m.statusLine)))
	}

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	serviceBar := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render(" [1] S3  [2] SSM  [3] SQS ")
	b.WriteString(fmt.Sprintf("  %s\n\n", serviceBar))

	help := "  ↑/↓ navigate"
	switch m.mode {
	case viewBuckets:
		help += " · enter browse · r refresh"
	case viewSSM:
		help += " · enter value · del delete · [ ] page · r refresh"
	case viewObjects:
		help += " · u upload · d download · del delete · esc back · r refresh"
	case viewUpload:
		help += " · enter confirm · esc cancel"
	case viewSQS:
		help += " · enter messages · c create · del delete · r refresh"
	case viewQueueMessages:
		help += " · s send · del delete · esc back · r refresh"
	case viewCreateQueue:
		help += " · enter confirm · esc cancel"
	case viewSendMessage:
		help += " · enter confirm · esc cancel"
	}
	if strings.Contains(m.containerStatus, "running") {
		help += " · x stop"
	} else if m.containerStatus != "not initialized" && m.containerStatus != "not found" && m.mode != viewQueueMessages {
		help += " · s start"
	}
	help += " · q quit"
	b.WriteString(helpStyle.Render(help))
	b.WriteString("\n")

	return b.String()
}
