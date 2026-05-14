package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/syasika/miniaws/internal/awsclient"
	"github.com/syasika/miniaws/internal/sqsops"
)

func fetchCurrentView(ctx context.Context, cfg aws.Config, endpoint string, mode viewMode, bucket, queueURL string) tea.Msg {
	switch mode {
	case viewSSM:
		return fetchParams(ctx, cfg, endpoint, "")
	case viewSQS, viewQueueMessages:
		return fetchQueues(ctx, cfg, endpoint)
	default:
		return fetchBuckets(ctx, cfg, endpoint)
	}
}

func fetchQueues(ctx context.Context, cfg aws.Config, endpoint string) tea.Msg {
	client := awsclient.NewSQSClient(cfg, endpoint)
	queues, err := sqsops.ListQueues(ctx, client)
	if err != nil {
		if sqsops.IsConnectionErr(err) {
			return queuesErrMsg{err: "Cannot reach ministack — is the container running?"}
		}
		return queuesErrMsg{err: fmt.Sprintf("SQS error: %v", err)}
	}
	return queuesMsg{queues: queues}
}

func fetchMessages(ctx context.Context, cfg aws.Config, endpoint, queueURL string) tea.Msg {
	client := awsclient.NewSQSClient(cfg, endpoint)
	msgs, err := sqsops.ReceiveMessages(ctx, client, queueURL, 10)
	if err != nil {
		if sqsops.IsConnectionErr(err) {
			return messagesErrMsg{err: "Cannot reach ministack — is the container running?"}
		}
		return messagesErrMsg{err: fmt.Sprintf("SQS error: %v", err)}
	}
	return messagesMsg{messages: msgs}
}

func doCreateQueue(ctx context.Context, cfg aws.Config, endpoint, name string) tea.Msg {
	client := awsclient.NewSQSClient(cfg, endpoint)
	q, err := sqsops.CreateQueue(ctx, client, name)
	if err != nil {
		return resultMsg{desc: fmt.Sprintf("Create failed: %v", err)}
	}
	return resultMsg{desc: fmt.Sprintf("Queue '%s' created", q.Name), refresh: true}
}

func doDeleteQueue(ctx context.Context, cfg aws.Config, endpoint, queueURL string) tea.Msg {
	client := awsclient.NewSQSClient(cfg, endpoint)
	if err := sqsops.DeleteQueue(ctx, client, queueURL); err != nil {
		return resultMsg{desc: fmt.Sprintf("Delete failed: %v", err)}
	}
	return resultMsg{desc: fmt.Sprintf("Queue '%s' deleted", sqsops.ExtractQueueName(queueURL)), refresh: true}
}

func doSendMessage(ctx context.Context, cfg aws.Config, endpoint, queueURL, body string) tea.Msg {
	client := awsclient.NewSQSClient(cfg, endpoint)
	msgID, err := sqsops.SendMessage(ctx, client, queueURL, body)
	if err != nil {
		return resultMsg{desc: fmt.Sprintf("Send failed: %v", err)}
	}
	return resultMsg{desc: fmt.Sprintf("Message sent (id: %s)", msgID), refresh: true}
}

func doDeleteMessage(ctx context.Context, cfg aws.Config, endpoint, queueURL, receiptHandle string) tea.Msg {
	client := awsclient.NewSQSClient(cfg, endpoint)
	if err := sqsops.DeleteMessage(ctx, client, queueURL, receiptHandle); err != nil {
		return resultMsg{desc: fmt.Sprintf("Delete failed: %v", err)}
	}
	return resultMsg{desc: "Message deleted", refresh: true}
}
