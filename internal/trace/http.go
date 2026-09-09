package trace

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	httptracepkg "net/http/httptrace"
	"sync"
	"time"
)

type httpTimings struct {
	mu           sync.Mutex
	started      time.Time
	dnsStarted   time.Time
	connectStart time.Time
	tlsStart     time.Time
	dns          time.Duration
	connect      time.Duration
	tls          time.Duration
	ttfb         time.Duration
	remoteAddr   string
	reused       bool
}

type tracedResponseBody struct {
	body       io.ReadCloser
	span       *Span
	response   *http.Response
	timings    *httpTimings
	redirects  *int
	requestOut *countingReadCloser
	bytesRead  int64
	once       sync.Once
}

type countingReadCloser struct {
	io.ReadCloser
	bytes int64
}

func (body *countingReadCloser) Read(buffer []byte) (int, error) {
	n, err := body.ReadCloser.Read(buffer)
	body.bytes += int64(n)
	return n, err
}

func DoHTTP(client *http.Client, request *http.Request) (*http.Response, error) {
	if request == nil {
		return nil, http.ErrNotSupported
	}
	if client == nil {
		client = http.DefaultClient
	}
	ctx := request.Context()
	span := Start(ctx, "HTTP", "http.request", "HTTP request", String("method", request.Method), URL("url", request.URL.String()), Int64("timeout_ms", requestTimeoutMS(ctx, client)))
	timings := &httpTimings{started: time.Now()}
	request = request.Clone(httptracepkg.WithClientTrace(ctx, timings.clientTrace()))
	var requestBody *countingReadCloser
	if request.Body != nil {
		requestBody = &countingReadCloser{ReadCloser: request.Body}
		request.Body = requestBody
	}
	redirects := 0
	requestClient := *client
	previousRedirect := client.CheckRedirect
	requestClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		redirects = len(via)
		if previousRedirect != nil {
			return previousRedirect(next, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	response, err := requestClient.Do(request)
	if err != nil {
		fields := []Field{String("method", request.Method), URL("url", request.URL.String())}
		fields = append(fields, timings.fields()...)
		fields = append(fields, Int("redirect_count", redirects))
		if requestBody != nil {
			fields = append(fields, Int64("bytes_written", requestBody.bytes))
		}
		span.FailMessage("HTTP request failed", err, fields...)
		return nil, err
	}
	response.Body = &tracedResponseBody{body: response.Body, span: span, response: response, timings: timings, redirects: &redirects, requestOut: requestBody}
	return response, nil
}

func (body *tracedResponseBody) Read(buffer []byte) (int, error) {
	n, err := body.body.Read(buffer)
	body.bytesRead += int64(n)
	if err == io.EOF {
		body.finish()
	}
	return n, err
}

func (body *tracedResponseBody) Close() error {
	err := body.body.Close()
	body.finish()
	return err
}

func (body *tracedResponseBody) finish() {
	body.once.Do(func() {
		fields := []Field{
			String("method", body.response.Request.Method),
			URL("url", body.response.Request.URL.String()),
			Int("status", body.response.StatusCode),
			String("status_text", http.StatusText(body.response.StatusCode)),
			Int64("content_length", body.response.ContentLength),
			Int64("bytes_read", body.bytesRead),
			Int("redirect_count", *body.redirects),
		}
		if body.requestOut != nil {
			fields = append(fields, Int64("bytes_written", body.requestOut.bytes))
		}
		fields = append(fields, body.timings.fields()...)
		body.span.EndMessage("HTTP response", fields...)
	})
}

func (timings *httpTimings) clientTrace() *httptracepkg.ClientTrace {
	return &httptracepkg.ClientTrace{
		DNSStart: func(httptracepkg.DNSStartInfo) {
			timings.mu.Lock()
			timings.dnsStarted = time.Now()
			timings.mu.Unlock()
		},
		DNSDone: func(httptracepkg.DNSDoneInfo) {
			timings.mu.Lock()
			if !timings.dnsStarted.IsZero() {
				timings.dns = time.Since(timings.dnsStarted)
			}
			timings.mu.Unlock()
		},
		ConnectStart: func(_, _ string) {
			timings.mu.Lock()
			timings.connectStart = time.Now()
			timings.mu.Unlock()
		},
		ConnectDone: func(_, _ string, _ error) {
			timings.mu.Lock()
			if !timings.connectStart.IsZero() {
				timings.connect = time.Since(timings.connectStart)
			}
			timings.mu.Unlock()
		},
		TLSHandshakeStart: func() {
			timings.mu.Lock()
			timings.tlsStart = time.Now()
			timings.mu.Unlock()
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			timings.mu.Lock()
			if !timings.tlsStart.IsZero() {
				timings.tls = time.Since(timings.tlsStart)
			}
			timings.mu.Unlock()
		},
		GotConn: func(info httptracepkg.GotConnInfo) {
			timings.mu.Lock()
			timings.reused = info.Reused
			if info.Conn != nil && info.Conn.RemoteAddr() != nil {
				timings.remoteAddr = info.Conn.RemoteAddr().String()
			}
			timings.mu.Unlock()
		},
		GotFirstResponseByte: func() {
			timings.mu.Lock()
			timings.ttfb = time.Since(timings.started)
			timings.mu.Unlock()
		},
	}
}

func (timings *httpTimings) fields() []Field {
	timings.mu.Lock()
	defer timings.mu.Unlock()
	fields := []Field{Bool("reused_connection", timings.reused)}
	if timings.remoteAddr != "" {
		fields = append(fields, String("remote_addr", timings.remoteAddr))
	}
	if timings.dns > 0 {
		fields = append(fields, DurationMS("dns_ms", timings.dns))
	}
	if timings.connect > 0 {
		fields = append(fields, DurationMS("connect_ms", timings.connect))
	}
	if timings.tls > 0 {
		fields = append(fields, DurationMS("tls_ms", timings.tls))
	}
	if timings.ttfb > 0 {
		fields = append(fields, DurationMS("ttfb_ms", timings.ttfb))
	}
	return fields
}

func requestTimeoutMS(ctx context.Context, client *http.Client) int64 {
	timeout := client.Timeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if timeout <= 0 || remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return 0
	}
	return timeout.Milliseconds()
}
