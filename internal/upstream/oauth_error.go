package upstream

import (
	"fmt"
)

type HTTPStatusError struct {
	StatusCode      int
	Status          string
	WWWAuthenticate []string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "upstream HTTP error"
	}
	return fmt.Sprintf("upstream HTTP %s", e.Status)
}
