package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

const logChunkMaxBytes = 32 * 1024

type logStreamWriter struct {
	w   io.WriteCloser
	mu  sync.Mutex
	buf bytes.Buffer
	seq int64
	ch  chan []byte
	wg  sync.WaitGroup
}

func newLogStreamWriter(w io.WriteCloser) *logStreamWriter {
	ls := &logStreamWriter{
		w:  w,
		ch: make(chan []byte, 256),
	}
	ls.wg.Add(1)
	go func() {
		defer ls.wg.Done()
		defer ls.w.Close()
		for data := range ls.ch {
			if _, err := ls.w.Write(data); err != nil {
				return
			}
		}
	}()
	return ls
}

func (l *logStreamWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.Write(p)
	for {
		data := l.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx == -1 {
			if l.buf.Len() >= logChunkMaxBytes {
				if err := l.emitChunk(string(l.buf.Bytes())); err != nil {
					return len(p), err
				}
				l.buf.Reset()
			}
			break
		}
		line := string(data[:idx])
		l.buf.Next(idx + 1)
		if strings.TrimSpace(line) == "" {
			continue
		}
		if err := l.emitChunk(line); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

func (l *logStreamWriter) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buf.Len() > 0 {
		if err := l.emitChunk(string(l.buf.Bytes())); err != nil {
			close(l.ch)
			l.wg.Wait()
			return err
		}
		l.buf.Reset()
	}
	close(l.ch)
	l.wg.Wait()
	return nil
}

func (l *logStreamWriter) emitChunk(content string) error {
	l.seq++
	payload := map[string]any{
		"seq":       l.seq,
		"content":   content,
		"timestamp": time.Now().Unix(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	select {
	case l.ch <- data:
	default:
	}
	return nil
}

func (w *Worker) openLogStream(ctx context.Context, job runner.Job, attempt int) io.WriteCloser {
	if w.Reporter == nil || w.Reporter.BaseURL == "" {
		log.Printf("log stream disabled for %s@%s: reporter base URL empty", job.Name, job.Version)
		return nil
	}
	name := url.PathEscape(job.Name)
	version := url.PathEscape(job.Version)
	endpoint := fmt.Sprintf("%s/api/logs/stream/%s/%s", strings.TrimRight(w.Reporter.BaseURL, "/"), name, version)
	q := url.Values{}
	if attempt > 0 {
		q.Set("attempt", strconv.Itoa(attempt))
	}
	if qs := q.Encode(); qs != "" {
		endpoint = endpoint + "?" + qs
	}
	log.Printf("opening log stream for %s@%s attempt=%d endpoint=%s", job.Name, job.Version, attempt, endpoint)
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		log.Printf("log stream request build failed for %s@%s: %v", job.Name, job.Version, err)
		_ = pr.Close()
		_ = pw.Close()
		return nil
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	if w.Reporter.Token != "" {
		req.Header.Set("X-Worker-Token", w.Reporter.Token)
	}
	client := w.Reporter.Client
	if client == nil {
		client = http.DefaultClient
	}
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("log stream send failed for %s@%s: %v", job.Name, job.Version, err)
			_ = pr.CloseWithError(err)
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			log.Printf("log stream failed for %s@%s: status=%s", job.Name, job.Version, resp.Status)
			return
		}
		log.Printf("log stream completed for %s@%s: status=%s", job.Name, job.Version, resp.Status)
	}()
	return newLogStreamWriter(pw)
}

func writeBootstrapLog(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	log.Printf("bootstrap log: %s", msg)
	_, _ = fmt.Fprintf(w, "%s\n", msg)
}
