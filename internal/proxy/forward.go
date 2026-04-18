package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/ridakaddir/apitwin/internal/logger"
)

// newReverseProxy creates a reverse proxy that forwards requests to target.
func newReverseProxy(target string) (*httputil.ReverseProxy, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	rp := httputil.NewSingleHostReverseProxy(u)

	// Rewrite the request before forwarding.
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = u.Host
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
		req.Header.Del("X-Mockr-File-Missing")
		// Disable compression so the response body is always plain text.
		// This ensures record mode captures readable JSON without decompressing.
		req.Header.Set("Accept-Encoding", "identity")
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("proxy error", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  "upstream unreachable",
			"detail": err.Error(),
		})
	}

	return rp, nil
}

// forwardRequest proxies the request to upstream and optionally records the response.
func forwardRequest(w http.ResponseWriter, r *http.Request, rp *httputil.ReverseProxy, bodyBytes []byte, recorder responseRecorder) {
	// Restore the body so the proxy can forward it.
	if bodyBytes != nil {
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))
	}

	logger.SetSource(w, logger.SourceProxy)

	if recorder != nil {
		// Wrap writer to capture response for recording.
		rec := &capturingWriter{ResponseWriter: w}
		start := time.Now()
		rp.ServeHTTP(rec, r)
		latency := time.Since(start)
		recorder(r, rec.status, rec.header, rec.body.Bytes(), latency)
		return
	}

	rp.ServeHTTP(w, r)
}

// responseRecorder is a callback invoked after a proxied response is received.
// latency is the wall-clock time the upstream took to respond.
type responseRecorder func(r *http.Request, status int, header http.Header, body []byte, latency time.Duration)

// capturingWriter captures a response so it can be inspected after the fact.
type capturingWriter struct {
	http.ResponseWriter
	status int
	header http.Header
	body   bytes.Buffer
}

func (cw *capturingWriter) WriteHeader(code int) {
	cw.status = code
	// Snapshot the response headers before flushing so the recorder can
	// inspect Content-Type, Content-Encoding, etc. Header() returns the
	// map the upstream handler populated; Clone() makes a safe copy.
	cw.header = cw.ResponseWriter.Header().Clone()
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *capturingWriter) Write(b []byte) (int, error) {
	cw.body.Write(b)
	return cw.ResponseWriter.Write(b)
}

// Unwrap allows logger.SetSource to traverse through this wrapper.
func (cw *capturingWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
}
