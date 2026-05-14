package awsclient

import (
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

	creds, err := cfg.Credentials.Retrieve(nil)
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
