package sqsops

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/syasika/miniaws/internal/awsclient"
)

type Queue struct {
	Name string
	URL  string
}

type Message struct {
	ID           string
	Body         string
	ReceiptHandle string
}

func IsConnectionErr(err error) bool {
	return awsclient.IsConnectionErr(err)
}

func friendlyErr(err error) error {
	return awsclient.FriendlyErr(err, "SQS")
}

func ListQueues(ctx context.Context, client *sqs.Client) ([]Queue, error) {
	var queues []Queue
	paginator := sqs.NewListQueuesPaginator(client, &sqs.ListQueuesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, friendlyErr(err)
		}
		for _, url := range page.QueueUrls {
			queues = append(queues, Queue{
				URL:  url,
				Name: ExtractQueueName(url),
			})
		}
	}
	return queues, nil
}

func CreateQueue(ctx context.Context, client *sqs.Client, name string) (*Queue, error) {
	resp, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(name),
	})
	if err != nil {
		return nil, friendlyErr(err)
	}
	return &Queue{Name: name, URL: *resp.QueueUrl}, nil
}

func DeleteQueue(ctx context.Context, client *sqs.Client, queueURL string) error {
	_, err := client.DeleteQueue(ctx, &sqs.DeleteQueueInput{
		QueueUrl: aws.String(queueURL),
	})
	return friendlyErr(err)
}

func SendMessage(ctx context.Context, client *sqs.Client, queueURL, body string) (string, error) {
	resp, err := client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(body),
	})
	if err != nil {
		return "", friendlyErr(err)
	}
	return *resp.MessageId, nil
}

func ReceiveMessages(ctx context.Context, client *sqs.Client, queueURL string, maxMessages int) ([]Message, error) {
	resp, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: int32(maxMessages),
		WaitTimeSeconds:     2,
	})
	if err != nil {
		return nil, friendlyErr(err)
	}
	msgs := make([]Message, len(resp.Messages))
	for i, m := range resp.Messages {
		msgs[i] = Message{
			ID:            *m.MessageId,
			Body:          *m.Body,
			ReceiptHandle: *m.ReceiptHandle,
		}
	}
	return msgs, nil
}

func DeleteMessage(ctx context.Context, client *sqs.Client, queueURL, receiptHandle string) error {
	_, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	return friendlyErr(err)
}

func ExtractQueueName(queueURL string) string {
	parts := strings.Split(queueURL, "/")
	return parts[len(parts)-1]
}
