package stream

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestCancellable(t *testing.T) {
	buf := make([]byte, 1024)
	t.Run("read_all", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancellable := NewCancellable(ctx, bytes.NewReader(randomBytes))
		bin, err := io.ReadAll(cancellable)
		assertErrorsIs(t, err, nil)
		assertBool(t, bytes.Equal(randomBytes, bin), "bytes.Equal returned false")
		cancel()
		// first error encountered is remembered.
		n, err := cancellable.Read(buf)
		assertEq(t, 0, n)
		assertErrorsIs(t, err, io.EOF)
	})

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancellable := NewCancellable(ctx, bytes.NewReader(randomBytes))
		_, err := cancellable.Read(buf)
		assertErrorsIs(t, err, nil)
		cancel()
		for i := 0; i < 5; i++ {
			_, err = cancellable.Read(buf)
			assertErrorsIs(t, err, ctx.Err())
		}
	})
}
