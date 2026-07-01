package logging

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
)

type AccessLogger struct {
	handler http.Handler
	logBody bool
}

func NewAccessLogger(handler http.Handler) *AccessLogger {
	return &AccessLogger{
		handler: handler,
		logBody: false,
	}
}

func (l *AccessLogger) EnableBodyLogging() {
	l.logBody = true
}

func (l *AccessLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestBody := ""
	if l.logBody {
		requestBody = string(l.bufferRequestBody(r))
	}
	slog.Info("Request", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery, "body", requestBody)

	l.handler.ServeHTTP(w, r)
}

func (l *AccessLogger) bufferRequestBody(r *http.Request) []byte {
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}

	r.Body = io.NopCloser(bytes.NewReader(buf))
	return buf
}
