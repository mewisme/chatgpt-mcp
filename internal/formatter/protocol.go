package formatter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	MaxSourceBytes  = 256 << 10
	MaxRequestBytes = MaxSourceBytes + 4<<10
	MaxOutputBytes  = 1 << 20
	Timeout         = 3 * time.Second
)

var (
	ErrTooLarge = errors.New("formatter payload exceeds size limit")
	ErrTimeout  = errors.New("formatter process timed out")
	ErrInvalid  = errors.New("formatter request is invalid")
)

type Request struct {
	Source   string `json:"source"`
	Width    int    `json:"width"`
	Terminal bool   `json:"terminal"`
	Style    string `json:"style,omitempty"`
}

type Response struct {
	Text string `json:"text"`
}

func (req Request) Validate() error {
	if len(req.Source) > MaxSourceBytes {
		return ErrTooLarge
	}
	if req.Width < 0 {
		return fmt.Errorf("%w: width", ErrInvalid)
	}
	return nil
}

func DecodeRequest(reader io.Reader) (Request, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxRequestBytes+1))
	if err != nil {
		return Request{}, err
	}
	if len(data) > MaxRequestBytes {
		return Request{}, ErrTooLarge
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return Request{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := req.Validate(); err != nil {
		return Request{}, err
	}
	return req, nil
}

func EncodeResponse(writer io.Writer, resp Response) error {
	if len(resp.Text) > MaxOutputBytes {
		return ErrTooLarge
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if len(data) > MaxOutputBytes+64 {
		return ErrTooLarge
	}
	_, err = writer.Write(append(data, '\n'))
	return err
}

func Fallback(source string) string {
	return strings.TrimSpace(source) + "\n"
}
