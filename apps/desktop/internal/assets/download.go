package assets

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) download(ctx context.Context, url, target, label string) error {
	var err error
	for attempt := range 3 {
		for _, candidate := range githubDownloadURLs(url, m.preferChina) {
			if err = m.downloadOnce(ctx, candidate, target, label); err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			err = fmt.Errorf("model download from %s: %w", candidate, err)
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}
	return err
}

func (m *Manager) downloadOnce(ctx context.Context, url, target, label string) error {
	m.publish(true, label, nil)
	// Limit time without headers or body bytes, rather than the duration of a
	// large model transfer. A blocked source must promptly yield to its mirror.
	attemptCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	timer := time.AfterFunc(m.idleTimeout, func() { cancel(errors.New("model download stalled")) })
	defer timer.Stop()
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "DayBee-Recorder")
	response, err := m.client.Do(req)
	if err != nil {
		if cause := context.Cause(attemptCtx); cause != nil {
			return cause
		}
		return err
	}
	defer response.Body.Close()
	timer.Reset(m.idleTimeout)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("model download: HTTP %d", response.StatusCode)
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/html") {
		return errors.New("model download returned an HTML page instead of model data")
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".download-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	buffer := make([]byte, 256*1024)
	var total int64
	last := time.Time{}
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			timer.Reset(m.idleTimeout)
			if _, err := f.Write(buffer[:n]); err != nil {
				return err
			}
			total += int64(n)
			if response.ContentLength > 0 && time.Since(last) > 100*time.Millisecond {
				percent := float64(total) * 100 / float64(response.ContentLength)
				m.publish(true, label, &percent)
				last = time.Now()
			}
		}
		if cause := context.Cause(attemptCtx); cause != nil {
			return cause
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if total == 0 || (response.ContentLength > 0 && total != response.ContentLength) {
		return io.ErrUnexpectedEOF
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), target)
}

func extractModel(ctx context.Context, archive, target string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := tar.NewReader(bzip2.NewReader(f))
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(header.Name)
		if !filepath.IsLocal(name) || strings.Contains(header.Name, "\\") {
			return errors.New("unsafe model archive path")
		}
		path := filepath.Join(target, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			total += header.Size
			if header.Size < 0 || total > 8<<30 {
				return errors.New("model archive exceeds size limit")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(out, reader, header.Size)
			if err := errors.Join(copyErr, out.Close()); err != nil {
				return err
			}
		default:
			return errors.New("model archive contains unsupported links or entries")
		}
	}
}
