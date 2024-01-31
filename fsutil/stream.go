package fsutil

import (
	"context"
	"io"
)

type cancellable struct {
	ctx context.Context
	r   io.Reader
	err error
}

// NewCancellable wraps ctx and r and returns io.Reader that reads from r.
// The returned reader stores a first error encountered,
// including io.EOF and context cancellation,
// and returns the error without actually reading from r in any subsequent Read calls.
//
// Cancelling ctx prevents any subsequent Read from succeeding and makes it return immediately.
// However, already blocking Read call may still continue to block since it is not cancelling the reader itself.
// If r is possible to block long and you wish to unblock it in that case,
// r itself must be cancellable by its own mean.
// For files, you can use os.Pipe and establish the platform-specific poll (e.g. epoll for linux)
// between the file descriptor of the file and the pipe as canceller.
//
// The returned io.Reader is not goroutine safe.
// Calling Read multiple times simultaneously may cause undefined behaviors.
func NewCancellable(ctx context.Context, r io.Reader) io.Reader {
	return &cancellable{
		ctx: ctx,
		r:   r,
	}
}

func (c *cancellable) Read(p []byte) (n int, err error) {
	if c.err != nil {
		return 0, c.err
	}
	if c.ctx.Err() != nil {
		c.err = c.ctx.Err()
		return 0, c.err
	}
	n, err = c.r.Read(p)
	if err != nil {
		c.err = err
	}
	return n, err
}
