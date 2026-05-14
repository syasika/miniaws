package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/syasika/miniaws/internal/awsclient"
	"github.com/syasika/miniaws/internal/s3ops"
)

func fetchBuckets(ctx context.Context, cfg aws.Config, endpoint string) tea.Msg {
	client := awsclient.NewS3Client(cfg, endpoint)
	names, err := s3ops.ListBuckets(ctx, client)
	if err != nil {
		if s3ops.IsConnectionErr(err) {
			return bucketsErrMsg{err: "Cannot reach ministack — is the container running?"}
		}
		return bucketsErrMsg{err: fmt.Sprintf("S3 error: %v", err)}
	}
	return bucketsMsg{buckets: names}
}

func fetchObjects(ctx context.Context, cfg aws.Config, endpoint, bucket string) tea.Msg {
	client := awsclient.NewS3Client(cfg, endpoint)
	items, err := s3ops.ListObjects(ctx, client, bucket)
	if err != nil {
		if s3ops.IsConnectionErr(err) {
			return objectsErrMsg{err: "Cannot reach ministack — is the container running?"}
		}
		return objectsErrMsg{err: fmt.Sprintf("S3 error: %v", err)}
	}
	return objectsMsg{objects: items}
}

func doUpload(ctx context.Context, cfg aws.Config, endpoint, bucket, localPath string) tea.Msg {
	client := awsclient.NewS3Client(cfg, endpoint)
	key := filepath.Base(localPath)
	if err := s3ops.UploadFile(ctx, client, bucket, key, localPath); err != nil {
		return resultMsg{desc: fmt.Sprintf("Upload failed: %v", err)}
	}
	return resultMsg{desc: fmt.Sprintf("Uploaded %s → s3://%s/%s", localPath, bucket, key), refresh: true}
}

func doDownload(ctx context.Context, cfg aws.Config, endpoint, bucket, key string) tea.Msg {
	client := awsclient.NewS3Client(cfg, endpoint)
	written, err := s3ops.DownloadFile(ctx, client, bucket, key, ".")
	if err != nil {
		return resultMsg{desc: fmt.Sprintf("Download failed: %v", err)}
	}
	localName := filepath.Base(key)
	return resultMsg{desc: fmt.Sprintf("Downloaded s3://%s/%s → %s (%d bytes)", bucket, key, localName, written)}
}

func doDelete(ctx context.Context, cfg aws.Config, endpoint, bucket, key string) tea.Msg {
	client := awsclient.NewS3Client(cfg, endpoint)
	if err := s3ops.DeleteObject(ctx, client, bucket, key); err != nil {
		return resultMsg{desc: fmt.Sprintf("Delete failed: %v", err)}
	}
	return resultMsg{desc: fmt.Sprintf("Deleted s3://%s/%s", bucket, key), refresh: true}
}
