package s3ops

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/syasika/miniaws/internal/awsclient"
)
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
		{fmt.Errorf("wrapped: %w", errors.New("connection refused")), true},
	}
	for _, tt := range tests {
		if got := IsConnectionErr(tt.err); got != tt.want {
			t.Errorf("IsConnectionErr(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestConnectionFriendlyErr(t *testing.T) {
	connErr := errors.New("dial tcp: connection refused")
	got := ConnectionFriendlyErr(connErr)
	if !strings.Contains(got.Error(), "cannot reach ministack") {
		t.Errorf("ConnectionFriendlyErr(connErr) = %v, want friendly message", got)
	}

	otherErr := errors.New("AccessDenied")
	got = ConnectionFriendlyErr(otherErr)
	if got != otherErr {
		t.Errorf("ConnectionFriendlyErr(otherErr) should pass through, got %v", got)
	}

	if got := ConnectionFriendlyErr(nil); got != nil {
		t.Errorf("ConnectionFriendlyErr(nil) = %v, want nil", got)
	}
}

func xmlResponse(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` + body
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *s3.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return awsclient.NewS3Client(awsclient.NewConfig(), server.URL)
}

func TestListBuckets(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, xmlResponse(`
			<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
				<Buckets>
					<Bucket><Name>alpha</Name><CreationDate>2024-01-01T00:00:00Z</CreationDate></Bucket>
					<Bucket><Name>beta</Name><CreationDate>2024-01-02T00:00:00Z</CreationDate></Bucket>
				</Buckets>
			</ListAllMyBucketsResult>`))
	})

	buckets, err := ListBuckets(context.Background(), client)
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if len(buckets) != 2 || buckets[0] != "alpha" || buckets[1] != "beta" {
		t.Errorf("ListBuckets = %v, want [alpha beta]", buckets)
	}
}

func TestListBucketsEmpty(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, xmlResponse(`
			<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
				<Buckets></Buckets>
			</ListAllMyBucketsResult>`))
	})

	buckets, err := ListBuckets(context.Background(), client)
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if len(buckets) != 0 {
		t.Errorf("ListBuckets = %v, want empty", buckets)
	}
}

func TestListObjects(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, "/test-bucket") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, xmlResponse(`
			<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
				<Name>test-bucket</Name>
				<Contents>
					<Key>doc.txt</Key>
					<Size>512</Size>
					<LastModified>2024-01-01T00:00:00Z</LastModified>
					<ETag>"abc"</ETag>
					<StorageClass>STANDARD</StorageClass>
				</Contents>
				<Contents>
					<Key>photo.jpg</Key>
					<Size>2048</Size>
					<LastModified>2024-01-02T00:00:00Z</LastModified>
					<ETag>"def"</ETag>
					<StorageClass>STANDARD</StorageClass>
				</Contents>
			</ListBucketResult>`))
	})

	objects, err := ListObjects(context.Background(), client, "test-bucket", "")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("ListObjects returned %d items, want 2", len(objects))
	}
	if objects[0].Key != "doc.txt" || objects[0].Size != 512 {
		t.Errorf("objects[0] = %+v, want {Key:doc.txt Size:512}", objects[0])
	}
	if objects[1].Key != "photo.jpg" || objects[1].Size != 2048 {
		t.Errorf("objects[1] = %+v, want {Key:photo.jpg Size:2048}", objects[1])
	}
}

func TestListObjectsWithCommonPrefixes(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, xmlResponse(`
			<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
				<Name>test-bucket</Name>
				<CommonPrefixes>
					<Prefix>images/</Prefix>
				</CommonPrefixes>
				<Contents>
					<Key>images/logo.png</Key>
					<Size>4096</Size>
					<LastModified>2024-01-02T00:00:00Z</LastModified>
					<ETag>"def"</ETag>
					<StorageClass>STANDARD</StorageClass>
				</Contents>
			</ListBucketResult>`))
	})

	objects, err := ListObjects(context.Background(), client, "test-bucket", "")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("ListObjects returned %d items, want 2", len(objects))
	}
	if objects[0].Key != "images/" || objects[0].Size != 0 {
		t.Errorf("objects[0] = %+v, want CommonPrefixes entry {Key:images/ Size:0}", objects[0])
	}
	if objects[1].Key != "images/logo.png" || objects[1].Size != 4096 {
		t.Errorf("objects[1] = %+v", objects[1])
	}
}

func TestListObjectsEmpty(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, xmlResponse(`
			<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
				<Name>test-bucket</Name>
			</ListBucketResult>`))
	})

	objects, err := ListObjects(context.Background(), client, "test-bucket", "")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(objects) != 0 {
		t.Errorf("ListObjects = %v, want empty", objects)
	}
}

func TestListObjectsReturnsFriendlyError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
			<Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
	})

	_, err := ListObjects(context.Background(), client, "test-bucket", "")
	if err == nil {
		t.Fatal("expected error from server")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("ListObjects error = %v, want access denied message", err)
	}
}

func TestCreateBucket(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/new-bucket" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})

	if err := CreateBucket(context.Background(), client, "new-bucket"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
}

func TestDeleteBucket(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/old-bucket" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := DeleteBucket(context.Background(), client, "old-bucket"); err != nil {
		t.Fatalf("DeleteBucket: %v", err)
	}
}

func TestDeleteObject(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/bucket/file.txt" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := DeleteObject(context.Background(), client, "bucket", "file.txt"); err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}
}

func TestUploadFile(t *testing.T) {
	uploaded := false
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" && r.URL.Path == "/bucket/test.txt" {
			uploaded = true
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UploadFile(context.Background(), client, "bucket", "test.txt", filePath); err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if !uploaded {
		t.Error("UploadFile did not make PUT request")
	}
}

func TestDownloadFile(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/bucket/hello.txt" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Length", "6")
		fmt.Fprint(w, "123456")
	})

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "out.txt")

	written, err := DownloadFile(context.Background(), client, "bucket", "hello.txt", destPath)
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if written != 6 {
		t.Errorf("DownloadFile wrote %d bytes, want 6", written)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "123456" {
		t.Errorf("DownloadFile content = %q, want %q", string(data), "123456")
	}
}

func TestDownloadFileDotPath(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data")
	})

	written, err := DownloadFile(context.Background(), client, "bucket", "remote.txt", ".")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if written != 4 {
		t.Errorf("DownloadFile wrote %d bytes", written)
	}
	os.Remove("remote.txt")
}

func TestDownloadFileGetObjectFails(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
			<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
	})

	_, err := DownloadFile(context.Background(), client, "bucket", "missing.txt", t.TempDir()+"/out.txt")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !strings.Contains(err.Error(), "no such key") && !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("DownloadFile error = %v, want key-not-found message", err)
	}
}

func TestDownloadFileCreateFails(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data")
	})

	_, err := DownloadFile(context.Background(), client, "bucket", "key.txt", "/nonexistent-parent-xyz/file.txt")
	if err == nil {
		t.Fatal("expected error from invalid path")
	}
	if !strings.Contains(err.Error(), "failed to create") {
		t.Errorf("DownloadFile error = %v, want 'failed to create' message", err)
	}
}

func TestDownloadFileBodyReadError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Write partial data then hijack and close to trigger a read error
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if h, ok := w.(http.Hijacker); ok {
			conn, _, _ := h.Hijack()
			conn.Close()
		}
	})

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "out.txt")

	_, err := DownloadFile(context.Background(), client, "bucket", "key.txt", dest)
	if err == nil {
		t.Fatal("expected error from broken body read")
	}

	// Verify the partial file was cleaned up
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("partial file should have been removed after error, stat err = %v", statErr)
	}
}

func TestEmptyBucketEmpty(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, xmlResponse(`
			<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
				<Name>empty-bucket</Name>
			</ListBucketResult>`))
	})

	if err := EmptyBucket(context.Background(), client, "empty-bucket"); err != nil {
		t.Fatalf("EmptyBucket: %v", err)
	}
}

func TestEmptyBucketSinglePage(t *testing.T) {
	var deleteCalled bool
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, xmlResponse(`
				<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
					<Name>bucket</Name>
					<Contents>
						<Key>a.txt</Key>
						<Size>1</Size>
						<LastModified>2024-01-01T00:00:00Z</LastModified>
						<ETag>"a"</ETag>
						<StorageClass>STANDARD</StorageClass>
					</Contents>
					<Contents>
						<Key>b.txt</Key>
						<Size>2</Size>
						<LastModified>2024-01-01T00:00:00Z</LastModified>
						<ETag>"b"</ETag>
						<StorageClass>STANDARD</StorageClass>
					</Contents>
				</ListBucketResult>`))
			return
		}
		if r.Method == "POST" && r.URL.Query().Has("delete") {
			deleteCalled = true
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
				<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
					<Deleted><Key>a.txt</Key></Deleted>
					<Deleted><Key>b.txt</Key></Deleted>
				</DeleteResult>`)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
	})

	if err := EmptyBucket(context.Background(), client, "bucket"); err != nil {
		t.Fatalf("EmptyBucket: %v", err)
	}
	if !deleteCalled {
		t.Error("EmptyBucket did not call DeleteObjects")
	}
}

func TestEmptyBucketMultiPage(t *testing.T) {
	var callCount int
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			callCount++
			if callCount == 1 {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, xmlResponse(`
					<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
						<Name>bucket</Name>
						<IsTruncated>true</IsTruncated>
						<NextContinuationToken>tok1</NextContinuationToken>
						<Contents>
							<Key>page1.txt</Key>
							<Size>1</Size>
							<LastModified>2024-01-01T00:00:00Z</LastModified>
							<ETag>"a"</ETag>
							<StorageClass>STANDARD</StorageClass>
						</Contents>
					</ListBucketResult>`))
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, xmlResponse(`
				<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
					<Name>bucket</Name>
					<Contents>
						<Key>page2.txt</Key>
						<Size>2</Size>
						<LastModified>2024-01-01T00:00:00Z</LastModified>
						<ETag>"b"</ETag>
						<StorageClass>STANDARD</StorageClass>
					</Contents>
				</ListBucketResult>`))
			return
		}
		if r.Method == "POST" && r.URL.Query().Has("delete") {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
				<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
					<Deleted><Key>deleted</Key></Deleted>
				</DeleteResult>`)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
	})

	if err := EmptyBucket(context.Background(), client, "bucket"); err != nil {
		t.Fatalf("EmptyBucket: %v", err)
	}
	if callCount != 2 {
		t.Errorf("ListObjectsV2 called %d times, want 2", callCount)
	}
}

func TestEmptyBucketListError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
			<Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
	})

	err := EmptyBucket(context.Background(), client, "bucket")
	if err == nil {
		t.Fatal("expected error from server")
	}
}

func TestEmptyBucketDeleteObjectsErrors(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, xmlResponse(`
				<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
					<Name>bucket</Name>
					<Contents>
						<Key>x.txt</Key>
						<Size>1</Size>
						<LastModified>2024-01-01T00:00:00Z</LastModified>
						<ETag>"x"</ETag>
						<StorageClass>STANDARD</StorageClass>
					</Contents>
				</ListBucketResult>`))
			return
		}
		if r.Method == "POST" && r.URL.Query().Has("delete") {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
				<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
					<Deleted><Key>x.txt</Key></Deleted>
					<Error>
						<Key>locked.txt</Key>
						<Code>AccessDenied</Code>
						<Message>Access Denied</Message>
					</Error>
				</DeleteResult>`)
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL)
	})

	err := EmptyBucket(context.Background(), client, "bucket")
	if err == nil {
		t.Fatal("expected error from DeleteObjects partial failure")
	}
	if !strings.Contains(err.Error(), "failed to delete 1 object") {
		t.Errorf("EmptyBucket error = %v, want 'failed to delete 1 object'", err)
	}
}

func TestEmptyBucketDeleteError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, xmlResponse(`
				<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
					<Name>bucket</Name>
					<Contents>
						<Key>x.txt</Key>
						<Size>1</Size>
						<LastModified>2024-01-01T00:00:00Z</LastModified>
						<ETag>"x"</ETag>
						<StorageClass>STANDARD</StorageClass>
					</Contents>
				</ListBucketResult>`))
			return
		}
		if r.Method == "POST" && r.URL.Query().Has("delete") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
				<Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL)
	})

	err := EmptyBucket(context.Background(), client, "bucket")
	if err == nil {
		t.Fatal("expected error from DeleteObjects")
	}
}

func TestUploadFileNonexistent(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not be called")
	})

	err := UploadFile(context.Background(), client, "bucket", "key", "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServerError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
			<Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
	})

	_, err := ListBuckets(context.Background(), client)
	if err == nil {
		t.Fatal("expected error from server")
	}
}
