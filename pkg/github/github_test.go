package github

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/reMarkable/orbit/pkg/mcache"
)

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func TestService_ListVersions(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/repos/test-org/test-repo/tags" {
				body := `[
					{"name": "module/v1.0.0"},
					{"name": "module/v1.1.0"},
					{"name": "other/v2.0.0"}
				]`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(body))),
				}, nil
			}
			return nil, errors.New("unexpected request")
		},
	}

	cfg := Config{
		OrgMappings: map[string]string{"test-system": "test-org"},
	}
	service := New(cfg, mockClient, mcache.New[string, []string](mcache.NoExpiration))

	versions, err := service.ListVersions(context.Background(), "test-system", "test-repo", "module")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"v1.0.0", "v1.1.0"}
	if len(versions) != len(expected) {
		t.Fatalf("expected %d versions, got %d", len(expected), len(versions))
	}
	for i, v := range versions {
		if v != expected[i] {
			t.Errorf("expected version %q, got %q", expected[i], v)
		}
	}
}

func TestService_ProxyDownload(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/repos/test-org/test-repo/tarball/refs/tags/module/v1.0.0" {
				var buf bytes.Buffer
				gz := gzip.NewWriter(&buf)
				tw := tar.NewWriter(gz)

				// Add a fake file to the tar archive with the expected structure
				header := &tar.Header{
					Name: "test-org-test-repo-branch/module/testfile.txt",
					Mode: 0600,
					Size: int64(len("fake tarball content")),
				}
				if err := tw.WriteHeader(header); err != nil {
					return nil, err
				}

				if _, err := tw.Write([]byte("fake tarball content")); err != nil {
					return nil, err
				}

				// Close the tar and gzip writers
				_ = tw.Close()
				_ = gz.Close()

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(&buf),
				}, nil

			}
			return nil, errors.New("unexpected request")
		},
	}

	cfg := Config{
		OrgMappings: map[string]string{"test-system": "test-org"},
	}
	service := New(cfg, mockClient, mcache.New[string, []string](mcache.NoExpiration))

	var buf bytes.Buffer
	err := service.ProxyDownload(context.Background(), "test-system", "test-repo", "module", "v1.0.0", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Decompress the output to validate its content
	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer func() {
		if err := gz.Close(); err != nil {
			panic(err)
		}
	}()

	var tarBuf bytes.Buffer
	if _, err := io.Copy(&tarBuf, gz); err != nil {
		t.Fatalf("failed to decompress gzip: %v", err)
	}

	expectedContent := "fake tarball content"
	if !bytes.Contains(tarBuf.Bytes(), []byte(expectedContent)) {
		t.Errorf("expected tarball to contain %q, but it was %q", expectedContent, tarBuf.Bytes())
	}
}

func TestService_ListVersions_CachesTagsPerRepo(t *testing.T) {
	var calls int
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/repos/test-org/test-repo/tags" {
				calls++
				body := `[
					{"name": "module/v1.0.0"},
					{"name": "other/v2.0.0"}
				]`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(body))),
				}, nil
			}
			return nil, errors.New("unexpected request")
		},
	}

	cfg := Config{
		OrgMappings: map[string]string{"test-system": "test-org"},
	}
	service := New(cfg, mockClient, mcache.New[string, []string](mcache.NoExpiration))

	// First call for "module" should hit the API.
	if _, err := service.ListVersions(context.Background(), "test-system", "test-repo", "module"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call for a different module in the same repo should be served from
	// the cache without hitting the API again.
	other, err := service.ListVersions(context.Background(), "test-system", "test-repo", "other")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected tags endpoint to be called once, got %d", calls)
	}

	if len(other) != 1 || other[0] != "v2.0.0" {
		t.Errorf("expected cached tags to yield [v2.0.0], got %v", other)
	}
}

func TestService_ListVersions_Pagination(t *testing.T) {
	// Build a first page with exactly tagsPerPage entries to force a second request.
	var firstPage bytes.Buffer
	firstPage.WriteString("[")
	for i := 0; i < tagsPerPage; i++ {
		if i > 0 {
			firstPage.WriteString(",")
		}
		fmt.Fprintf(&firstPage, `{"name": "filler/v0.0.%d"}`, i)
	}
	firstPage.WriteString("]")

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/repos/test-org/test-repo/tags" {
				return nil, errors.New("unexpected request")
			}
			switch req.URL.Query().Get("page") {
			case "1":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(firstPage.Bytes())),
				}, nil
			case "2":
				body := `[{"name": "module/v1.0.0"}]`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(body))),
				}, nil
			default:
				return nil, errors.New("unexpected page")
			}
		},
	}

	cfg := Config{
		OrgMappings: map[string]string{"test-system": "test-org"},
	}
	service := New(cfg, mockClient, mcache.New[string, []string](mcache.NoExpiration))

	versions, err := service.ListVersions(context.Background(), "test-system", "test-repo", "module")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(versions) != 1 || versions[0] != "v1.0.0" {
		t.Errorf("expected [v1.0.0] across pages, got %v", versions)
	}
}
