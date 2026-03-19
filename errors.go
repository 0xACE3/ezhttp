package fetch

import (
	"fmt"
	"time"
)

// ResponseError is returned for non-2xx HTTP responses.
// Compatible with errors.As for structured error handling.
type ResponseError struct {
	Status     int
	Body       []byte
	RetryAfter time.Duration
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("fetch: HTTP %d", e.Status)
}
