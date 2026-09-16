package formatter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func Run(ctx context.Context, path string, req Request) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(path) == "" {
		return "", ErrInvalid
	}
	if err := req.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	if len(payload) > MaxRequestBytes {
		return "", ErrTooLarge
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.Stdin = bytes.NewReader(payload)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr cappedBuffer
	stderr.max = 32 << 10
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, MaxOutputBytes+1))
	if len(data) > MaxOutputBytes {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return "", ErrTooLarge
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return "", ErrTimeout
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("formatter plugin: %s: %w", msg, waitErr)
		}
		return "", fmt.Errorf("formatter plugin: %w", waitErr)
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", readErr
	}
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if len(resp.Text) > MaxOutputBytes {
		return "", ErrTooLarge
	}
	return resp.Text, nil
}

type cappedBuffer struct {
	bytes.Buffer
	max int
}

func (buf *cappedBuffer) Write(p []byte) (int, error) {
	if buf.max <= 0 || buf.Len() >= buf.max {
		return len(p), nil
	}
	if remain := buf.max - buf.Len(); len(p) > remain {
		_, _ = buf.Buffer.Write(p[:remain])
		return len(p), nil
	}
	return buf.Buffer.Write(p)
}
