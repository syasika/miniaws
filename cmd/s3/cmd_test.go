package s3

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// ---- Helper function tests ----

func TestStripS3Prefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"s3://bucket/key", "bucket/key"},
		{"bucket/key", "bucket/key"},
		{"s3://bucket", "bucket"},
		{"", ""},
		{"s3://", ""},
	}
	for _, tt := range tests {
		got := stripS3Prefix(tt.in)
		if got != tt.want {
			t.Errorf("stripS3Prefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsS3Path(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"s3://bucket/key", true},
		{"s3://bucket", true},
		{"./file.txt", false},
		{"/abs/path", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isS3Path(tt.s)
		if got != tt.want {
			t.Errorf("isS3Path(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestParseS3Path(t *testing.T) {
	tests := []struct {
		s          string
		wantBucket string
		wantKey    string
	}{
		{"s3://bucket/key", "bucket", "key"},
		{"s3://bucket/path/to/obj", "bucket", "path/to/obj"},
		{"s3://bucket", "bucket", ""},
		{"bucket/key", "bucket", "key"},
		{"bucket", "bucket", ""},
	}
	for _, tt := range tests {
		bucket, key := parseS3Path(tt.s)
		if bucket != tt.wantBucket || key != tt.wantKey {
			t.Errorf("parseS3Path(%q) = (%q, %q), want (%q, %q)", tt.s, bucket, key, tt.wantBucket, tt.wantKey)
		}
	}
}

// ---- CLI command tests ----

// newTestServer creates an httptest.Server that acts as an S3 API endpoint,
// and returns the server along with a cleanup function.
func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// executeCommand builds a root → s3 command chain and executes, so
// persistent flags like --endpoint-url are inherited by the s3 subcommand.
// Captures os.Stdout (commands use fmt.Print) via a pipe.
func executeCommand(args ...string) (string, error) {
	// Redirect stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	root := &cobra.Command{Use: "miniaws"}
	root.PersistentFlags().String("endpoint-url", "http://localhost:4566", "")
	root.AddCommand(Cmd())
	root.SetArgs(append([]string{"s3"}, args...))

	err := root.Execute()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	return strings.TrimSpace(buf.String()), err
}

// xmlResponse wraps body in a minimal S3 XML envelope.
func xmlResponse(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` + body
}

func s3BucketListResponse(buckets ...string) string {
	var items string
	for _, b := range buckets {
		items += fmt.Sprintf("<Bucket><Name>%s</Name><CreationDate>2024-01-01T00:00:00Z</CreationDate></Bucket>", b)
	}
	return xmlResponse(fmt.Sprintf(`<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Buckets>%s</Buckets></ListAllMyBucketsResult>`, items))
}

func s3ListObjectsResponse(keys ...string) string {
	var items string
	for _, k := range keys {
		items += fmt.Sprintf(`<Contents><Key>%s</Key><Size>100</Size><LastModified>2024-01-01T00:00:00Z</LastModified><ETag>"a"</ETag><StorageClass>STANDARD</StorageClass></Contents>`, k)
	}
	return xmlResponse(fmt.Sprintf(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>bucket</Name>%s</ListBucketResult>`, items))
}

// ---- s3 ls ----

func TestS3Ls_NoArgs(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, s3BucketListResponse("alpha", "beta"))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	out, err := executeCommand("ls", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 ls failed: %v", err)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Errorf("s3 ls output = %q, want alpha and beta", out)
	}
	if !strings.Contains(out, "Buckets (2)") {
		t.Errorf("s3 ls output = %q, want bucket count", out)
	}
}

func TestS3Ls_NoBuckets(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, s3BucketListResponse())
	})

	out, err := executeCommand("ls", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 ls failed: %v", err)
	}
	if !strings.Contains(out, "No buckets") {
		t.Errorf("s3 ls output = %q, want 'No buckets.'", out)
	}
}

func TestS3Ls_BucketObjects(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/my-bucket" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, s3ListObjectsResponse("file1.txt", "file2.txt"))
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
	})

	out, err := executeCommand("ls", "my-bucket", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 ls failed: %v", err)
	}
	if !strings.Contains(out, "file1.txt") || !strings.Contains(out, "file2.txt") {
		t.Errorf("s3 ls output = %q, want both files", out)
	}
}

func TestS3Ls_BucketObjectsEmpty(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, s3ListObjectsResponse())
	})

	out, err := executeCommand("ls", "my-bucket", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 ls failed: %v", err)
	}
	if !strings.Contains(out, "No objects") {
		t.Errorf("s3 ls output = %q, want 'No objects.'", out)
	}
}

func TestS3Ls_S3Prefix(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Should strip the s3:// prefix
		if r.URL.Path == "/bucket" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, s3ListObjectsResponse("key.txt"))
			return
		}
		t.Errorf("unexpected: %s %s (path should not have s3:// prefix)", r.Method, r.URL.Path)
	})

	out, err := executeCommand("ls", "s3://bucket", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 ls failed: %v", err)
	}
	if !strings.Contains(out, "key.txt") {
		t.Errorf("s3 ls output = %q, want key.txt", out)
	}
}

func TestS3Ls_WithPrefix(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// ListObjectsV2 with prefix
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, s3ListObjectsResponse("subdir/file.txt"))
	})

	out, err := executeCommand("ls", "my-bucket/subdir", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 ls with prefix failed: %v", err)
	}
	if !strings.Contains(out, "s3://my-bucket/subdir") {
		t.Errorf("s3 ls output = %q, want header with prefix", out)
	}
	if !strings.Contains(out, "subdir/file.txt") {
		t.Errorf("s3 ls output = %q, want file with prefix path", out)
	}
}

func TestS3Ls_FolderAndZeroSizeObjects(t *testing.T) {
	// Return a folder (trailing /) and a zero-size object to exercise display branches
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, xmlResponse(`
			<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
				<Name>bucket</Name>
				<CommonPrefixes><Prefix>images/</Prefix></CommonPrefixes>
				<Contents><Key>zero.txt</Key><Size>0</Size><LastModified>2024-01-01T00:00:00Z</LastModified><ETag>"a"</ETag><StorageClass>STANDARD</StorageClass></Contents>
				<Contents><Key>normal.txt</Key><Size>200</Size><LastModified>2024-01-01T00:00:00Z</LastModified><ETag>"b"</ETag><StorageClass>STANDARD</StorageClass></Contents>
			</ListBucketResult>`))
	})

	out, err := executeCommand("ls", "bucket", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 ls failed: %v", err)
	}
	if !strings.Contains(out, "📁") {
		t.Errorf("s3 ls output = %q, want folder icon", out)
	}
	if !strings.Contains(out, "zero.txt") {
		t.Errorf("s3 ls output = %q, want zero.txt", out)
	}
	if !strings.Contains(out, "normal.txt") {
		t.Errorf("s3 ls output = %q, want normal.txt", out)
	}
	if !strings.Contains(out, "200 bytes") {
		t.Errorf("s3 ls output = %q, want size for normal.txt", out)
	}
}

func TestS3Ls_ServerError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
	})

	_, err := executeCommand("ls", "--endpoint-url", srv.URL)
	if err == nil {
		t.Fatal("expected error from server")
	}
}

func TestS3Ls_BucketError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist</Message></Error>`)
	})

	_, err := executeCommand("ls", "unknown-bucket", "--endpoint-url", srv.URL)
	if err == nil {
		t.Fatal("expected error for unknown bucket")
	}
}

// ---- s3 mb ----

func TestS3Mb(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" && r.URL.Path == "/new-bucket" {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
	})

	out, err := executeCommand("mb", "new-bucket", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 mb failed: %v", err)
	}
	if !strings.Contains(out, "created") {
		t.Errorf("s3 mb output = %q, want 'created'", out)
	}
}

func TestS3Mb_S3Prefix(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prefixed-bucket" {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	})

	_, err := executeCommand("mb", "s3://prefixed-bucket", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 mb failed: %v", err)
	}
}

func TestS3Mb_NoArgs(t *testing.T) {
	_, err := executeCommand("mb")
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}

func TestS3Mb_ServerError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>BucketAlreadyExists</Code><Message>Bucket already exists</Message></Error>`)
	})

	_, err := executeCommand("mb", "existing-bucket", "--endpoint-url", srv.URL)
	if err == nil {
		t.Fatal("expected error for existing bucket")
	}
}

// ---- s3 rb ----

func TestS3Rb(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/old-bucket" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
	})

	out, err := executeCommand("rb", "old-bucket", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 rb failed: %v", err)
	}
	if !strings.Contains(out, "removed") {
		t.Errorf("s3 rb output = %q, want 'removed'", out)
	}
}

func TestS3Rb_Force(t *testing.T) {
	listCall := 0
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// List objects (EmptyBucket)
			listCall++
			if listCall == 1 {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, s3ListObjectsResponse("obj.txt"))
				return
			}
			t.Errorf("unexpected list: %s %s", r.Method, r.URL.Path)
			return
		}
		if r.Method == "POST" && r.URL.Query().Has("delete") {
			// DeleteObjects (EmptyBucket)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Deleted><Key>obj.txt</Key></Deleted></DeleteResult>`)
			return
		}
		if r.Method == "DELETE" {
			// DeleteBucket
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
	})

	out, err := executeCommand("rb", "bucket", "--force", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 rb --force failed: %v", err)
	}
	if !strings.Contains(out, "removed") {
		t.Errorf("s3 rb output = %q, want 'removed'", out)
	}
	if listCall != 1 {
		t.Errorf("EmptyBucket list called %d times, want 1", listCall)
	}
}

func TestS3Rb_NoArgs(t *testing.T) {
	_, err := executeCommand("rb")
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}

func TestS3Rb_DeleteBucketError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>BucketNotEmpty</Code><Message>Bucket is not empty</Message></Error>`)
	})

	_, err := executeCommand("rb", "nonempty-bucket", "--endpoint-url", srv.URL)
	if err == nil {
		t.Fatal("expected error for non-empty bucket")
	}
}

func TestS3Rb_ForceEmptyBucketError(t *testing.T) {
	listCalled := false
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			listCalled = true
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
	})

	_, err := executeCommand("rb", "bucket", "--force", "--endpoint-url", srv.URL)
	if err == nil {
		t.Fatal("expected error from EmptyBucket")
	}
	if !listCalled {
		t.Error("EmptyBucket was not called")
	}
}

// ---- s3 cp ----

func TestS3Cp_Upload(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	uploadCalled := false
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" && r.URL.Path == "/bucket/key.txt" {
			uploadCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
	})

	out, err := executeCommand("cp", srcFile, "s3://bucket/key.txt", "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 cp failed: %v", err)
	}
	if !uploadCalled {
		t.Error("s3 cp did not upload file")
	}
	if !strings.Contains(out, "Uploaded") {
		t.Errorf("s3 cp output = %q, want 'Uploaded'", out)
	}
}

func TestS3Cp_Download(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "out.txt")

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/bucket/key.txt" {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Length", "5")
			fmt.Fprint(w, "world")
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
	})

	out, err := executeCommand("cp", "s3://bucket/key.txt", dest, "--endpoint-url", srv.URL)
	if err != nil {
		t.Fatalf("s3 cp download failed: %v", err)
	}
	if !strings.Contains(out, "Downloaded") {
		t.Errorf("s3 cp output = %q, want 'Downloaded'", out)
	}
	if !strings.Contains(out, "5 bytes") {
		t.Errorf("s3 cp output = %q, want byte count", out)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "world" {
		t.Errorf("downloaded content = %q, want %q", string(data), "world")
	}
}

func TestS3Cp_DownloadInvalidS3Path(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "out.txt")

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not be called")
	})

	_, err := executeCommand("cp", "s3://bucket", dest, "--endpoint-url", srv.URL)
	if err == nil {
		t.Fatal("expected error for bucket-only s3 path")
	}
}

func TestS3Cp_BothLocalPaths(t *testing.T) {
	_, err := executeCommand("cp", "file1.txt", "file2.txt")
	if err == nil {
		t.Fatal("expected error for two local paths")
	}
}

func TestS3Cp_UploadNonexistent(t *testing.T) {
	_, err := executeCommand("cp", "/nonexistent/path", "s3://bucket/key", "--endpoint-url", "http://localhost:1")
	if err == nil {
		t.Fatal("expected error for nonexistent source file")
	}
}

func TestS3Cp_UploadServerError(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
	})

	_, err := executeCommand("cp", srcFile, "s3://bucket/key.txt", "--endpoint-url", srv.URL)
	if err == nil {
		t.Fatal("expected upload error from server")
	}
}

func TestS3Cp_DownloadServerError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
	})

	_, err := executeCommand("cp", "s3://bucket/missing.txt", "/tmp/out.txt", "--endpoint-url", srv.URL)
	if err == nil {
		t.Fatal("expected download error from server")
	}
}

func TestS3Cp_UploadInvalidS3Path(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not be called")
	})

	_, err := executeCommand("cp", "file.txt", "s3://bucket", "--endpoint-url", srv.URL)
	if err == nil {
		t.Fatal("expected error for bucket-only s3 path")
	}
}

// ---- s3 command structure ----

func TestCmdReturnsNonNil(t *testing.T) {
	if Cmd() == nil {
		t.Fatal("Cmd() returned nil")
	}
}

func TestCmdUse(t *testing.T) {
	if Cmd().Use != "s3" {
		t.Errorf("Cmd().Use = %q, want 's3'", Cmd().Use)
	}
}

func TestCmdSubcommands(t *testing.T) {
	cmds := Cmd().Commands()
	names := make(map[string]bool)
	for _, c := range cmds {
		names[c.Name()] = true
	}
	for _, want := range []string{"ls", "mb", "rb", "cp"} {
		if !names[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
}
