package llm

import (
	"errors"
)

var (
	ErrTimeout = errors.New("request timeout")
	ErrRateLimit = errors.New("rate limit exceeded")
)
