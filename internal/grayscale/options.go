// Package grayscale converts static images to opaque Gray8 pixels.
package grayscale

import (
	"fmt"
	"strings"

	"imagetoolbox/internal/imageio"
)

const (
	DefaultLevels     = 256
	DefaultQuality    = 90
	DefaultBackground = "#FFFFFF"
)

// Options controls luma conversion and output encoding. Zero values use defaults.
type Options struct {
	Levels     int
	Background string
	Quality    int
	Lossless   bool
}

// Normalize applies defaults shared by CLI, HTTP and direct callers.
func (o *Options) Normalize() {
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
func (o Options) Validate() error {
	if o.Levels != 256 {
		return fmt.Errorf("levels must be 256")
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
