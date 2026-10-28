// Package grayscale converts static images to opaque Gray8 pixels.
package grayscale

import (
	"errors"
	"fmt"
	"strings"

	"imagetoolbox/internal/imageio"
)

// ErrInvalidOptions identifies domain parameter errors for transport adapters.
var ErrInvalidOptions = errors.New("invalid grayscale options")

const (
	DefaultLevels     = 256
	DefaultQuality    = 90
	DefaultBackground = "#FFFFFF"
)

// Options controls luma conversion and output encoding. Zero values use defaults.
type Options struct {
	Levels     int
	Dither     Dither
	Background string
	Quality    int
	Lossless   bool
}

// Normalize applies defaults shared by CLI, HTTP and direct callers.
func (o *Options) Normalize() {
	if o.Dither == "" {
		o.Dither = DitherNone
	}
	if o.Levels == 0 {
		o.Levels = DefaultLevels
	}
	if o.Quality == 0 {
		o.Quality = DefaultQuality
	}
	if strings.TrimSpace(o.Background) == "" {
		o.Background = DefaultBackground
	}
}

// Validate checks normalized domain options.
func (o Options) Validate() (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", ErrInvalidOptions, err)
		}
	}()
	if o.Levels != 2 && o.Levels != 4 && o.Levels != 16 && o.Levels != 256 {
		return fmt.Errorf("levels must be 2, 4, 16 or 256")
	}
	if o.Dither != DitherNone && o.Dither != DitherFS && o.Dither != DitherOrdered {
		return fmt.Errorf("dither must be none, fs or ordered")
	}
	if o.Levels == 256 && o.Dither != DitherNone {
		return fmt.Errorf("dither requires levels below 256")
	}
	if o.Quality < 1 || o.Quality > 100 {
		return fmt.Errorf("quality must be between 1 and 100")
	}
	bg, err := imageio.ParseHexColor(o.Background)
	if err != nil {
		return fmt.Errorf("invalid background color: %w", err)
	}
	if bg.A != 255 {
		return fmt.Errorf("background color must be opaque")
	}
	return nil
}
