package server

import (
	"compress/gzip"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	http.ResponseWriter
	writer      *gzip.Writer
	wroteHeader bool
	compress    bool
	statusCode  int
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.statusCode = code
	g.wroteHeader = true
	if shouldCompress(g.ResponseWriter.Header(), code) {
		g.compress = true
		g.Header().Set("Content-Encoding", "gzip")
		g.Header().Del("Content-Length")
		g.writer = gzip.NewWriter(g.ResponseWriter)
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.wroteHeader {
		g.WriteHeader(http.StatusOK)
	}
	if g.compress {
		return g.writer.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipResponseWriter) Flush() {
	if g.compress && g.writer != nil {
		_ = g.writer.Flush()
	}
	if flusher, ok := g.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (g *gzipResponseWriter) Close() error {
	if g.writer != nil {
		return g.writer.Close()
	}
	return nil
}

func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		gzw := &gzipResponseWriter{ResponseWriter: w}
		defer func() { _ = gzw.Close() }()
		next.ServeHTTP(gzw, r)
	})
}

func shouldCompress(h http.Header, status int) bool {
	if status == http.StatusNoContent || status == http.StatusNotModified {
		return false
	}
	if strings.Contains(strings.ToLower(h.Get("Content-Encoding")), "gzip") {
		return false
	}
	contentType := strings.ToLower(h.Get("Content-Type"))
	if contentType == "" {
		return true
	}
	return strings.HasPrefix(contentType, "application/json") || strings.HasPrefix(contentType, "text/")
}
