package awsclient

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

func NewConfig() aws.Config {
	return aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("miniaws", "miniaws", ""),
		Retryer: func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = 1
				o.MaxBackoff = 50 * time.Millisecond
			})
		},
	}
}

func NewS3Client(cfg aws.Config, endpoint string) *s3.Client {
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
}

func NewSSMClient(cfg aws.Config, endpoint string) *ssm.Client {
	return ssm.NewFromConfig(cfg, func(o *ssm.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

func NewSQSClient(cfg aws.Config, endpoint string) *sqs.Client {
	return sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

func IsConnectionErr(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "dial tcp")
}

func FriendlyErr(err error, service string) error {
	if err == nil {
		return nil
	}
	if IsConnectionErr(err) {
		return fmt.Errorf("cannot reach ministack — is the container running?")
	}
	errStr := err.Error()
	if strings.Contains(errStr, "api error ") {
		parts := strings.SplitN(errStr, "api error ", 2)
		if len(parts) == 2 {
			return fmt.Errorf("%s API error: %s", service, strings.ToLower(strings.TrimSpace(parts[1])))
		}
	}
	return err
}
