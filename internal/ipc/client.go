package ipc

import (
	"context"
	"fmt"
	"net"
	"net/rpc"
)

// Call performs a net/rpc call over a Unix socket with context cancellation.
func Call(ctx context.Context, sockPath, method string, req Request) (Response, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", sockPath)
	if err != nil {
		return Response{}, fmt.Errorf("failed to connect to daemon socket %q: %w", sockPath, err)
	}
	defer conn.Close()

	client := rpc.NewClient(conn)
	defer client.Close()

	done := make(chan error, 1)
	resp := Response{}
	go func() {
		done <- client.Call(method, req, &resp)
	}()

	select {
	case <-ctx.Done():
		return Response{}, fmt.Errorf("rpc call %q canceled: %w", method, ctx.Err())
	case err := <-done:
		if err != nil {
			return Response{}, fmt.Errorf("rpc call %q failed: %w", method, err)
		}
		return resp, nil
	}
}
