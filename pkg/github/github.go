// Copyright 2023 Henrik Hedlund. All rights reserved.
// Use of this source code is governed by the GNU Affero
// GPL license that can be found in the LICENSE file.

package github

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/reMarkable/orbit/pkg/auth"
)

const (
	apiVersion  = "2022-11-28"
	contentType = "application/vnd.github+json"
	tagsPerPage = 100
)

type Config struct {
	Repositories map[string][]string `envconfig:"REPOSITORIES"`
	OrgMappings  map[string]string   `envconfig:"ORG_MAPPINGS"`
	Token        string              `envconfig:"TOKEN"`
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func New(cfg Config, c HTTPClient) *Service {
	return &Service{
		cfg:    cfg,
		client: c,
	}
}

type Service struct {
	cfg    Config
	client HTTPClient
}

// https://docs.github.com/en/rest/repos/repos?apiVersion=2022-11-28#list-repository-tags
func (s *Service) ListVersions(ctx context.Context, system, repo, module string) ([]string, error) {
	owner := s.mapOrg(system)
	if err := s.validRepo(owner, repo); err != nil {
		return nil, err
	}

	var (
		page     = 1
		prefix   = module + "/"
		versions = []string{}
	)
	for {
		uri := fmt.Sprintf("repos/%s/%s/tags?per_page=%d&page=%d", owner, repo, tagsPerPage, page)
		res, err := s.makeRequest(ctx, uri)
		if err != nil {
			return nil, err
		}

		var tags []struct {
			Name string `json:"name"`
		}
		err = json.NewDecoder(res).Decode(&tags)
		cerr := res.Close()
		if cerr != nil {
			return nil, fmt.Errorf("closing response: %w", cerr)
		}

		if err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		for _, tag := range tags {
			if strings.HasPrefix(tag.Name, prefix) {
				versions = append(versions, strings.TrimPrefix(tag.Name, prefix))
			}
		}

		if len(tags) < tagsPerPage {
			break
		}
		page++
	}
	return versions, nil
}

func (s *Service) ProxyDownload(ctx context.Context, system, repo, module, version string, w io.Writer) error {
	owner := s.mapOrg(system)
	if err := s.validRepo(owner, repo); err != nil {
		return err
	}

	uri := fmt.Sprintf("repos/%s/%s/tarball/refs/tags/%s/%s", owner, repo, module, version)
	body, err := s.makeRequest(ctx, uri)
	if err != nil {
		return err
	}
	defer func() {
		if err := body.Close(); err != nil {
			slog.Warn("error closing response body", "err", err)
		}
	}()

	zr, err := gzip.NewReader(body)
	if err != nil {
		return fmt.Errorf("read gzip: %w", err)
	}
	defer func() {
		if err := zr.Close(); err != nil {
			slog.Warn("error closing gzip reader", "err", err)
		}
	}()

	zw := gzip.NewWriter(w)
	defer func() {
		if err := zw.Close(); err != nil {
			slog.Warn("error closing gzip writer:", "err", err)
		}
	}()

	tr := tar.NewReader(zr)
	tw := tar.NewWriter(zw)
	defer func() {
		if err := tw.Close(); err != nil {
			slog.Warn("error closing tar writer:", "err", err)
		}
	}()

	prefix := fmt.Sprintf("^%s-%s-[^/]+/%s/(.+)", owner, repo, module)
	if err := copy(prefix, tw, tr); err != nil {
		return err
	}
	return nil
}

func (s *Service) makeRequest(ctx context.Context, uri string) (io.ReadCloser, error) {
	url := fmt.Sprintf("https://api.github.com/%s", uri)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Add("Accept", contentType)
	req.Header.Add("X-GitHub-Api-Version", apiVersion)

	if token := auth.GetToken(ctx, s.cfg.Token); token != "" {
		req.Header.Add("Authorization", "Bearer "+token)
	}

	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, &httpErr{
			code: res.StatusCode,
			msg:  slurp(res.Body),
		}
	}

	return res.Body, nil
}

func (s *Service) mapOrg(system string) string {
	if owner, ok := s.cfg.OrgMappings[system]; ok {
		return owner
	}
	return system
}

func (s *Service) validRepo(owner, repo string) error {
	if len(s.cfg.Repositories) == 0 {
		return nil
	}

	if repos, ok := s.cfg.Repositories[owner]; ok {
		if slices.Contains(repos, repo) {
			return nil
		}
	}

	return &httpErr{
		code: http.StatusForbidden,
		msg:  "not a valid repository",
	}
}

func copy(prefix string, w *tar.Writer, r *tar.Reader) error {
	re, err := regexp.Compile(prefix)
	if err != nil {
		return fmt.Errorf("compile prefix regexp: %w", err)
	}

	for {
		hdr, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		match := re.FindStringSubmatch(hdr.Name)
		if len(match) == 2 {
			hdr.Name = match[1]
			if err := w.WriteHeader(hdr); err != nil {
				return fmt.Errorf("writing header: %w", err)
			}
			if _, err := io.Copy(w, r); err != nil {
				slog.Error("error copying tar entry", "name", hdr.Name, "err", err)
			}
		}
	}
}

func slurp(r io.ReadCloser) string {
	defer func() {
		if err := r.Close(); err != nil {
			slog.Warn("error closing response body", "err", err)
		}
	}()

	b, err := io.ReadAll(r)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

type httpErr struct {
	code int
	msg  string
}

func (e *httpErr) Error() string {
	return e.msg
}

func (e *httpErr) StatusCode() int {
	return e.code
}
