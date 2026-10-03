package barcode

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os/exec"
	"time"

	"imagetoolbox/internal/nativebin"
)

type Decoder interface {
	Decode(context.Context, *image.Gray, []Symbology) ([]RawCode, error)
}

type nativeDecoder struct{ path string }

// cappedBuffer drains output but rejects oversized responses, without unbounded allocation.
type cappedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	n := len(data)
	remaining := b.limit - b.Len()
	if len(data) > remaining {
		data = data[:remaining]
		b.overflow = true
	}
	_, _ = b.Buffer.Write(data)
	return n, nil
}

func (d nativeDecoder) Decode(ctx context.Context, img *image.Gray, formats []Symbology) ([]RawCode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := d.path
	if path == "" {
		var err error
		path, err = nativebin.Ensure(nativebin.ZXingReader)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDecoderUnavailable, err)
		}
	}
	reader, writer := io.Pipe()
	cmd := exec.CommandContext(ctx, path)
	cmd.Stdin = reader
	output := &cappedBuffer{limit: 4 << 20}
	stderr := &cappedBuffer{limit: 4096}
	cmd.Stdout = output
	cmd.Stderr = stderr
	// WaitDelay bounds I/O cleanup even if a faulty child retains inherited pipes.
	cmd.WaitDelay = 2 * time.Second
	frameDone := make(chan error, 1)
	go func() { err := writeFrame(writer, img, formats); _ = writer.CloseWithError(err); frameDone <- err }()
	err := cmd.Run()
	_ = reader.Close()
	frameErr := <-frameDone
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v: %s", ErrDecoderFailure, err, stderr.String())
	}
	if frameErr != nil {
		return nil, fmt.Errorf("%w: transfer: %v", ErrDecoderFailure, frameErr)
	}
	if output.overflow {
		return nil, fmt.Errorf("%w: oversized response", ErrDecoderFailure)
	}
	return parseResponse(bytes.NewReader(output.Bytes()))
}
