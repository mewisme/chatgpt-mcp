package oauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type callbackResult struct {
	Credential Credential
	Err        error
}

func (s *Store) Login(ctx context.Context, config LoginConfig, options LoginOptions) (Credential, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Credential{}, fmt.Errorf("open OAuth callback listener: %w", err)
	}
	defer listener.Close()

	flows := NewFlowManager(s)
	session, err := flows.Begin(ctx, config, "http://"+listener.Addr().String()+"/callback", options.ExtraScope)
	if err != nil {
		return Credential{}, err
	}
	defer flows.Cancel(session.ID)

	resultCh := make(chan callbackResult, 1)
	serverErrCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback/", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		credential, err := flows.Complete(request.Context(), strings.TrimPrefix(request.URL.Path, "/callback/"), query.Get("state"), query.Get("code"), query.Get("iss"), query.Get("error"), query.Get("error_description"))
		select {
		case resultCh <- callbackResult{Credential: credential, Err: err}:
		default:
		}
		if err != nil {
			http.Error(writer, "OAuth authorization failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write([]byte("Authorization received. You can close this window."))
	})
	callbackServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := callbackServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = callbackServer.Shutdown(shutdownCtx)
		cancel()
	}()

	if options.OnURL != nil {
		if err := options.OnURL(session.AuthorizationURL); err != nil {
			return Credential{}, err
		}
	}
	select {
	case <-ctx.Done():
		return Credential{}, ctx.Err()
	case err := <-serverErrCh:
		return Credential{}, fmt.Errorf("OAuth callback server: %w", err)
	case result := <-resultCh:
		return result.Credential, result.Err
	}
}
