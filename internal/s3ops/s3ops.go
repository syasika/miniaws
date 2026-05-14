package s3ops

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Object struct {
	Key  string
	Size int64
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

func ConnectionFriendlyErr(err error) error {
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
			msg := strings.ToLower(strings.TrimSpace(parts[1]))
			return fmt.Errorf("S3 API error: %s", msg)
		}
	}
	return err
}

func ListBuckets(ctx context.Context, client *s3.Client) ([]string, error) {
	resp, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, ConnectionFriendlyErr(err)
	}
	names := make([]string, len(resp.Buckets))
	for i, b := range resp.Buckets {
		names[i] = *b.Name
	}
	return names, nil
}

func ListObjects(ctx context.Context, client *s3.Client, bucket string) ([]Object, error) {
	resp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return nil, ConnectionFriendlyErr(err)
	}

	var items []Object
	for _, p := range resp.CommonPrefixes {
		items = append(items, Object{Key: *p.Prefix, Size: 0})
	}
	for _, obj := range resp.Contents {
		items = append(items, Object{Key: *obj.Key, Size: *obj.Size})
	}
	return items, nil
}

func CreateBucket(ctx context.Context, client *s3.Client, name string) error {
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(name),
	})
	return ConnectionFriendlyErr(err)
}

func DeleteBucket(ctx context.Context, client *s3.Client, name string) error {
	_, err := client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(name),
	})
	return ConnectionFriendlyErr(err)
}

func EmptyBucket(ctx context.Context, client *s3.Client, bucket string) error {
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		if len(page.Contents) == 0 {
			continue
		}
		oids := make([]types.ObjectIdentifier, len(page.Contents))
		for i, obj := range page.Contents {
			oids[i] = types.ObjectIdentifier{Key: obj.Key}
		}
		if _, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{Objects: oids},
		}); err != nil {
			return err
		}
	}
	return nil
}

func UploadFile(ctx context.Context, client *s3.Client, bucket, key, localPath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", localPath, err)
	}
	defer file.Close()

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	return ConnectionFriendlyErr(err)
}

func DownloadFile(ctx context.Context, client *s3.Client, bucket, key, localPath string) (int64, error) {
	resp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, ConnectionFriendlyErr(err)
	}
	defer resp.Body.Close()

	if localPath == "." || localPath == "./" {
		localPath = filepath.Base(key)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create %s: %w", localPath, err)
	}
	defer out.Close()

	return io.Copy(out, resp.Body)
}

func DeleteObject(ctx context.Context, client *s3.Client, bucket, key string) error {
	_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return ConnectionFriendlyErr(err)
}
