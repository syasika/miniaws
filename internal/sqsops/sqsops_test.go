package sqsops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/syasika/miniaws/internal/awsclient"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *sqs.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return awsclient.NewSQSClient(awsclient.NewConfig(), srv.URL)
}

func target(name string) string {
	return "AmazonSQS." + name
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code, msg string) {
	w.Header().Set("X-Amzn-ErrorType", code)
	w.WriteHeader(http.StatusBadRequest)
	writeJSON(w, map[string]string{"__type": code, "Message": msg})
}

func readBody(r *http.Request) string {
	data, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return ""
	}
	return string(data)
}

// --- IsConnectionErr ---

func TestIsConnectionErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"connection refused", errors.New("connection refused"), true},
		{"no such host", errors.New("no such host"), true},
		{"i/o timeout", errors.New("i/o timeout"), true},
		{"broken pipe", errors.New("broken pipe"), true},
		{"dial tcp", errors.New("dial tcp 127.0.0.1:4566: connect: connection refused"), true},
		{"other error", errors.New("AccessDenied"), false},
		{"wrapped connection refused", fmt.Errorf("wrapped: %w", errors.New("connection refused")), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConnectionErr(tt.err); got != tt.want {
				t.Errorf("IsConnectionErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// --- ExtractQueueName ---

func TestExtractQueueName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"http://sqs.us-east-1.localhost:4566/000000000000/test-queue", "test-queue"},
		{"http://sqs.us-east-1.localhost:4566/000000000000/my.queue", "my.queue"},
		{"http://127.0.0.1:4566/queue/alpha", "alpha"},
		{"http://localhost/123/abc", "abc"},
		{"https://sqs.amazonaws.com/123456789012/my-queue", "my-queue"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := ExtractQueueName(tt.url); got != tt.want {
				t.Errorf("ExtractQueueName(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// --- ListQueues ---

func TestListQueues(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != target("ListQueues") {
			t.Errorf("unexpected target: %s", r.Header.Get("X-Amz-Target"))
		}
		writeJSON(w, map[string]interface{}{
			"QueueUrls": []string{
				"http://sqs.us-east-1.localhost:4566/000000000000/alpha",
				"http://sqs.us-east-1.localhost:4566/000000000000/beta",
			},
		})
	})

	queues, err := ListQueues(context.Background(), client)
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	if len(queues) != 2 {
		t.Fatalf("got %d queues, want 2", len(queues))
	}
	if queues[0].Name != "alpha" || queues[0].URL != "http://sqs.us-east-1.localhost:4566/000000000000/alpha" {
		t.Errorf("queues[0] = %+v", queues[0])
	}
	if queues[1].Name != "beta" || queues[1].URL != "http://sqs.us-east-1.localhost:4566/000000000000/beta" {
		t.Errorf("queues[1] = %+v", queues[1])
	}
}

func TestListQueuesEmpty(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"QueueUrls": []string{},
		})
	})

	queues, err := ListQueues(context.Background(), client)
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	if len(queues) != 0 {
		t.Errorf("got %d queues, want 0", len(queues))
	}
}

func TestListQueuesMultiPage(t *testing.T) {
	var callCount int
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			writeJSON(w, map[string]interface{}{
				"QueueUrls": []string{"http://localhost/000000000000/page1"},
				"NextToken": "token1",
			})
			return
		}
		writeJSON(w, map[string]interface{}{
			"QueueUrls": []string{"http://localhost/000000000000/page2"},
		})
	})

	queues, err := ListQueues(context.Background(), client)
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	if len(queues) != 2 {
		t.Fatalf("got %d queues, want 2", len(queues))
	}
	if queues[0].Name != "page1" || queues[1].Name != "page2" {
		t.Errorf("queues = %+v", queues)
	}
	if callCount != 2 {
		t.Errorf("ListQueues called %d times, want 2", callCount)
	}
}

func TestListQueuesError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "OverLimit", "over limit")
	})
	_, err := ListQueues(context.Background(), client)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- CreateQueue ---

func TestCreateQueue(t *testing.T) {
	var callTarget string
	var reqBody string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callTarget = r.Header.Get("X-Amz-Target")
		reqBody = readBody(r)
		writeJSON(w, map[string]string{
			"QueueUrl": "http://sqs.us-east-1.localhost:4566/000000000000/my-queue",
		})
	})

	q, err := CreateQueue(context.Background(), client, "my-queue")
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	if callTarget != target("CreateQueue") {
		t.Errorf("target = %s", callTarget)
	}
	if !strings.Contains(reqBody, `"QueueName"`) || !strings.Contains(reqBody, `"my-queue"`) {
		t.Errorf("request body missing QueueName: %s", reqBody)
	}
	if q.Name != "my-queue" || q.URL != "http://sqs.us-east-1.localhost:4566/000000000000/my-queue" {
		t.Errorf("Queue = %+v", q)
	}
}

func TestCreateQueueError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "QueueAlreadyExists", "Queue already exists")
	})
	_, err := CreateQueue(context.Background(), client, "dup")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- DeleteQueue ---

func TestDeleteQueue(t *testing.T) {
	var callTarget string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callTarget = r.Header.Get("X-Amz-Target")
		writeJSON(w, map[string]interface{}{})
	})

	if err := DeleteQueue(context.Background(), client, "http://localhost/q"); err != nil {
		t.Fatalf("DeleteQueue: %v", err)
	}
	if callTarget != target("DeleteQueue") {
		t.Errorf("target = %s", callTarget)
	}
}

func TestDeleteQueueError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "QueueDoesNotExist", "Queue does not exist")
	})
	err := DeleteQueue(context.Background(), client, "http://localhost/q")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- SendMessage ---

func TestSendMessage(t *testing.T) {
	var callTarget string
	var reqBody string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callTarget = r.Header.Get("X-Amz-Target")
		reqBody = readBody(r)
		writeJSON(w, map[string]interface{}{
			"MessageId": "msg-001",
		})
	})

	msgID, err := SendMessage(context.Background(), client, "http://localhost/q", "hello")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if callTarget != target("SendMessage") {
		t.Errorf("target = %s", callTarget)
	}
	if !strings.Contains(reqBody, `"MessageBody"`) || !strings.Contains(reqBody, `"hello"`) {
		t.Errorf("request body missing MessageBody: %s", reqBody)
	}
	if msgID != "msg-001" {
		t.Errorf("MessageId = %q, want msg-001", msgID)
	}
}

func TestSendMessageError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "UnsupportedOperation", "unsupported")
	})
	_, err := SendMessage(context.Background(), client, "http://localhost/q", "body")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ReceiveMessages ---

func TestReceiveMessages(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != target("ReceiveMessage") {
			t.Errorf("unexpected target: %s", r.Header.Get("X-Amz-Target"))
		}
		writeJSON(w, map[string]interface{}{
			"Messages": []map[string]string{
				{"MessageId": "m1", "Body": "body1", "ReceiptHandle": "rh1"},
				{"MessageId": "m2", "Body": "body2", "ReceiptHandle": "rh2"},
			},
		})
	})

	msgs, err := ReceiveMessages(context.Background(), client, "http://localhost/q", 10)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].ID != "m1" || msgs[0].Body != "body1" || msgs[0].ReceiptHandle != "rh1" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].ID != "m2" || msgs[1].Body != "body2" || msgs[1].ReceiptHandle != "rh2" {
		t.Errorf("msgs[1] = %+v", msgs[1])
	}
}

func TestReceiveMessagesEmpty(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"Messages": []interface{}{},
		})
	})

	msgs, err := ReceiveMessages(context.Background(), client, "http://localhost/q", 10)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("got %d messages, want 0", len(msgs))
	}
}

func TestReceiveMessagesError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "OverLimit", "over limit")
	})
	_, err := ReceiveMessages(context.Background(), client, "http://localhost/q", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- DeleteMessage ---

func TestDeleteMessage(t *testing.T) {
	var callTarget string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callTarget = r.Header.Get("X-Amz-Target")
		writeJSON(w, map[string]interface{}{})
	})

	if err := DeleteMessage(context.Background(), client, "http://localhost/q", "rh1"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if callTarget != target("DeleteMessage") {
		t.Errorf("target = %s", callTarget)
	}
}

func TestDeleteMessageError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "InvalidAddress", "invalid address")
	})
	err := DeleteMessage(context.Background(), client, "http://localhost/q", "rh1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Connection error handling (exercises friendlyErr via IsConnectionErr) ---

func TestListQueuesConnectionError(t *testing.T) {
	client := awsclient.NewSQSClient(awsclient.NewConfig(), "http://127.0.0.1:1")
	_, err := ListQueues(context.Background(), client)
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "cannot reach ministack") {
		t.Errorf("error = %v, want friendly message", err)
	}
}

func TestCreateQueueConnectionError(t *testing.T) {
	client := awsclient.NewSQSClient(awsclient.NewConfig(), "http://127.0.0.1:1")
	_, err := CreateQueue(context.Background(), client, "test")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "cannot reach ministack") {
		t.Errorf("error = %v, want friendly message", err)
	}
}

func TestDeleteQueueConnectionError(t *testing.T) {
	client := awsclient.NewSQSClient(awsclient.NewConfig(), "http://127.0.0.1:1")
	err := DeleteQueue(context.Background(), client, "http://localhost/q")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "cannot reach ministack") {
		t.Errorf("error = %v, want friendly message", err)
	}
}

func TestSendMessageConnectionError(t *testing.T) {
	client := awsclient.NewSQSClient(awsclient.NewConfig(), "http://127.0.0.1:1")
	_, err := SendMessage(context.Background(), client, "http://localhost/q", "body")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "cannot reach ministack") {
		t.Errorf("error = %v, want friendly message", err)
	}
}

func TestReceiveMessagesConnectionError(t *testing.T) {
	client := awsclient.NewSQSClient(awsclient.NewConfig(), "http://127.0.0.1:1")
	_, err := ReceiveMessages(context.Background(), client, "http://localhost/q", 10)
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "cannot reach ministack") {
		t.Errorf("error = %v, want friendly message", err)
	}
}

func TestDeleteMessageConnectionError(t *testing.T) {
	client := awsclient.NewSQSClient(awsclient.NewConfig(), "http://127.0.0.1:1")
	err := DeleteMessage(context.Background(), client, "http://localhost/q", "rh1")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "cannot reach ministack") {
		t.Errorf("error = %v, want friendly message", err)
	}
}

// --- Friendly API error formatting via GenericAPIError fallthrough ---

func TestCreateQueueFriendlyAPIError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Amzn-ErrorType", "UnknownFailure")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"__type": "UnknownFailure",
			"Message": "something went wrong",
		})
	})
	_, err := CreateQueue(context.Background(), client, "fail")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "sqs api error") {
		t.Errorf("error = %v, want friendly API error message containing 'SQS API error'", err)
	}
}

func TestDeleteQueueFriendlyAPIError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Amzn-ErrorType", "UnknownFailure")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"__type": "UnknownFailure",
			"Message": "internal error",
		})
	})
	err := DeleteQueue(context.Background(), client, "http://localhost/q")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "sqs api error") {
		t.Errorf("error = %v, want friendly API error message", err)
	}
}
