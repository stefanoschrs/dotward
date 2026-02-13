package ipc

import "time"

// Request is the RPC request payload for file watch operations.
type Request struct {
	Path string
	TTL  time.Duration
}

// Response is the RPC response payload.
type Response struct {
	Success bool
	Error   string
}
