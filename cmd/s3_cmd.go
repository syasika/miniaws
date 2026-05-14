package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/syasika/miniaws/internal/awsclient"
	"github.com/syasika/miniaws/internal/s3ops"
)

var (
	bucketStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	objectStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	folderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	sizeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33")).Padding(0, 1)
	emptyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
)

var s3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "Manage S3 resources on ministack",
	Long:  `List buckets, create/delete buckets, copy files, and list objects via the ministack S3 API.`,
}

func stripS3Prefix(s string) string {
	return strings.TrimPrefix(s, "s3://")
}

func isS3Path(s string) bool {
	return strings.HasPrefix(s, "s3://")
}

func parseS3Path(s string) (bucket, key string) {
	s = stripS3Prefix(s)
	parts := strings.SplitN(s, "/", 2)
	if len(parts) < 2 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

var s3LsCmd = &cobra.Command{
	Use:   "ls [bucket/prefix]",
	Short: "List buckets or objects",
	Long: `With no arguments, list all buckets.
With a bucket or s3://bucket argument, list objects.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewS3Client(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		if len(args) == 0 {
			buckets, err := s3ops.ListBuckets(ctx, client)
			if err != nil {
				return err
			}
			if len(buckets) == 0 {
				fmt.Println(emptyStyle.Render("No buckets."))
				return nil
			}
			fmt.Println(headerStyle.Render(fmt.Sprintf(" Buckets (%d) ", len(buckets))))
			for _, b := range buckets {
				fmt.Printf("  📦 %s\n", bucketStyle.Render(b))
			}
			return nil
		}

		parts := strings.SplitN(stripS3Prefix(args[0]), "/", 2)
		bucketName := parts[0]
		prefix := ""
		if len(parts) > 1 {
			prefix = parts[1]
		}

		objects, err := s3ops.ListObjects(ctx, client, bucketName)
		if err != nil {
			return err
		}

		if len(objects) == 0 {
			fmt.Println(emptyStyle.Render("No objects."))
			return nil
		}

		label := fmt.Sprintf(" s3://%s/", bucketName)
		if prefix != "" {
			label += prefix
		}
		fmt.Println(headerStyle.Render(label))

		for _, obj := range objects {
			if strings.HasSuffix(obj.Key, "/") {
				fmt.Printf("  📁 %s\n", folderStyle.Render(obj.Key))
			} else if obj.Size == 0 {
				fmt.Printf("  📄 %s\n", objectStyle.Render(obj.Key))
			} else {
				fmt.Printf("  📄 %s  %s\n", objectStyle.Render(obj.Key), sizeStyle.Render(fmt.Sprintf("(%d bytes)", obj.Size)))
			}
		}
		return nil
	},
}

var s3MbCmd = &cobra.Command{
	Use:   "mb <bucket>",
	Short: "Create an S3 bucket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewS3Client(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		bucket := stripS3Prefix(args[0])
		if err := s3ops.CreateBucket(ctx, client, bucket); err != nil {
			return err
		}
		fmt.Printf("%s Bucket '%s' created\n", successStyle.Render("✓"), bucket)
		return nil
	},
}

var s3RbCmd = &cobra.Command{
	Use:   "rb <bucket>",
	Short: "Remove an S3 bucket",
	Long:  `Remove a bucket. Must be empty unless --force is used.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewS3Client(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		bucket := stripS3Prefix(args[0])
		force, _ := cmd.Flags().GetBool("force")

		if force {
			if err := s3ops.EmptyBucket(ctx, client, bucket); err != nil {
				return err
			}
		}
		if err := s3ops.DeleteBucket(ctx, client, bucket); err != nil {
			return err
		}
		fmt.Printf("%s Bucket '%s' removed\n", successStyle.Render("✓"), bucket)
		return nil
	},
}

var s3CpCmd = &cobra.Command{
	Use:   "cp <source> <destination>",
	Short: "Copy files between local and S3",
	Long: `Upload or download files.

  Upload:   miniaws s3 cp ./file.txt s3://bucket/key
  Download: miniaws s3 cp s3://bucket/key ./file.txt`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		src, dst := args[0], args[1]
		endpoint, _ := cmd.Flags().GetString("endpoint-url")
		client := awsclient.NewS3Client(awsclient.NewConfig(), endpoint)
		ctx := context.Background()

		if isS3Path(src) && !isS3Path(dst) {
			bucket, key := parseS3Path(src)
			if key == "" {
				return fmt.Errorf("invalid s3 path: %s (expected s3://bucket/key)", src)
			}
			written, err := s3ops.DownloadFile(ctx, client, bucket, key, dst)
			if err != nil {
				return err
			}
			fmt.Printf("%s Downloaded s3://%s/%s → %s (%d bytes)\n", successStyle.Render("✓"), bucket, key, dst, written)
		} else if !isS3Path(src) && isS3Path(dst) {
			bucket, key := parseS3Path(dst)
			if key == "" {
				return fmt.Errorf("invalid s3 path: %s (expected s3://bucket/key)", dst)
			}
			if err := s3ops.UploadFile(ctx, client, bucket, key, src); err != nil {
				return err
			}
			fmt.Printf("%s Uploaded %s → s3://%s/%s\n", successStyle.Render("✓"), src, bucket, key)
		} else {
			return fmt.Errorf("one argument must be an s3:// path and the other a local path")
		}
		return nil
	},
}

func init() {
	s3Cmd.AddCommand(s3LsCmd)
	s3Cmd.AddCommand(s3MbCmd)
	s3Cmd.AddCommand(s3RbCmd)
	s3Cmd.AddCommand(s3CpCmd)

	s3RbCmd.Flags().BoolP("force", "f", false, "Empty the bucket before removing it")
}
