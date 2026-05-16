package awsclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
)

func TestNewConfigReturnsNonZeroConfig(t *testing.T) {
	cfg := NewConfig()
	if cfg.Region != "us-east-1" {
		t.Errorf("Region = %q, want %q", cfg.Region, "us-east-1")
	}
}

func TestNewConfigCredentials(t *testing.T) {
	cfg := NewConfig()
	if cfg.Credentials == nil {
		t.Fatal("Credentials provider is nil")
	}

	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Retrieve credentials: %v", err)
	}
	if creds.AccessKeyID != "miniaws" {
		t.Errorf("AccessKeyID = %q, want %q", creds.AccessKeyID, "miniaws")
	}
	if creds.SecretAccessKey != "miniaws" {
		t.Errorf("SecretAccessKey = %q, want %q", creds.SecretAccessKey, "miniaws")
	}
	if creds.Expired() {
		t.Error("Static credentials should not be expired")
	}
}

func TestNewConfigRetryer(t *testing.T) {
	cfg := NewConfig()
	if cfg.Retryer == nil {
		t.Fatal("Retryer is nil")
	}

	retrier := cfg.Retryer()
	if retrier == nil {
		t.Fatal("Retryer() returned nil")
	}

	standard, ok := retrier.(*retry.Standard)
	if !ok {
		t.Fatalf("Retryer type = %T, want *retry.Standard", retrier)
	}

	maxAttempts := standard.MaxAttempts()
	if maxAttempts != 1 {
		t.Errorf("MaxAttempts = %d, want 1", maxAttempts)
	}
}

func TestNewConfigReturnsCopy(t *testing.T) {
	a := NewConfig()
	b := NewConfig()

	a.Region = "eu-west-1"
	if b.Region != "us-east-1" {
		t.Error("NewConfig should return independent copies")
	}
}

func TestNewConfigImplementsRetryer(t *testing.T) {
	cfg := NewConfig()
	var r aws.Retryer = cfg.Retryer()
	_ = r
}

func TestNewS3Client(t *testing.T) {
	cfg := NewConfig()
	c := NewS3Client(cfg, "http://localhost:4566")
	if c == nil {
		t.Fatal("NewS3Client returned nil")
	}
}

func TestNewSSMClient(t *testing.T) {
	cfg := NewConfig()
	c := NewSSMClient(cfg, "http://localhost:4566")
	if c == nil {
		t.Fatal("NewSSMClient returned nil")
	}
}

func TestNewSQSClient(t *testing.T) {
	cfg := NewConfig()
	c := NewSQSClient(cfg, "http://localhost:4566")
	if c == nil {
		t.Fatal("NewSQSClient returned nil")
	}
}

func TestIsConnectionErr(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("connection refused"), true},
		{errors.New("no such host"), true},
		{errors.New("i/o timeout"), true},
		{errors.New("broken pipe"), true},
		{errors.New("dial tcp 127.0.0.1:4566: connect: connection refused"), true},
		{errors.New("S3: AccessDenied"), false},
		{errors.New("bucket not found"), false},
		{errors.New(""), false},
		{errors.New("something else"), false},
	}
	for _, tt := range tests {
		if got := IsConnectionErr(tt.err); got != tt.want {
			t.Errorf("IsConnectionErr(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestFriendlyErr_Nil(t *testing.T) {
	if got := FriendlyErr(nil, "S3"); got != nil {
		t.Errorf("FriendlyErr(nil, _) = %v, want nil", got)
	}
}

func TestFriendlyErr_ConnectionError(t *testing.T) {
	err := FriendlyErr(errors.New("dial tcp: connection refused"), "S3")
	if err == nil {
		t.Fatal("FriendlyErr returned nil")
	}
	if !strings.Contains(err.Error(), "cannot reach ministack") {
		t.Errorf("FriendlyErr = %q, want 'cannot reach ministack'", err)
	}
}

func TestFriendlyErr_APIServerError(t *testing.T) {
	// Test the "api error " parsing path
	err := FriendlyErr(errors.New("api error AccessDenied"), "S3")
	if err == nil {
		t.Fatal("FriendlyErr returned nil")
	}
	if !strings.Contains(err.Error(), "S3 API error") {
		t.Errorf("FriendlyErr = %q, want to contain 'S3 API error'", err)
	}
	if !strings.Contains(err.Error(), "accessdenied") {
		t.Errorf("FriendlyErr = %q, want to contain 'accessdenied'", err)
	}
}

func TestFriendlyErr_APIServerErrorTrailingSpace(t *testing.T) {
	// Verify the strings.TrimSpace path in the api error branch
	err := FriendlyErr(errors.New("api error  InvalidParameterValue  "), "SSM")
	if err == nil {
		t.Fatal("FriendlyErr returned nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "SSM API error") {
		t.Errorf("FriendlyErr = %q, want 'SSM API error'", msg)
	}
	if strings.Contains(msg, "  ") {
		t.Errorf("FriendlyErr = %q, should not contain double spaces (TrimSpace issue)", msg)
	}
}

func TestFriendlyErr_Passthrough(t *testing.T) {
	orig := errors.New("some random error")
	got := FriendlyErr(orig, "SQS")
	if got != orig {
		t.Errorf("FriendlyErr should pass through non-connection, non-api errors, got %v", got)
	}
}

func TestFriendlyErr_ConnectionRefusedIsConnectionErr(t *testing.T) {
	// Verify that IsConnectionErr is called and returned as friendly error
	err := FriendlyErr(errors.New("connection refused"), "S3")
	if err == nil {
		t.Fatal("FriendlyErr returned nil")
	}
	if !strings.Contains(err.Error(), "cannot reach ministack") {
		t.Errorf("FriendlyErr = %q, want 'cannot reach ministack'", err)
	}
}
