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
		if err != nil && !errors.Is(err, ErrInvalidOptions) {
			err = fmt.Errorf("%w: %w", ErrInvalidOptions, err)
		}
	}()
	if err := ValidateLevels(o.Levels); err != nil {
		return err
	}
	if o.Dither != DitherNone && o.Dither != DitherFS && o.Dither != DitherOrdered {
		return fmt.Errorf("dither must be none, fs or ordered")
	}
	if o.Levels == 256 && o.Dither != DitherNone {
		return fmt.Errorf("dither requires levels below 256")
	}
	if err := ValidateQuality(o.Quality); err != nil {
		return err
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

// ValidateLevels checks a supplied value without applying defaults. Adapters use
// it before normalization so explicitly supplied zero cannot mean "omitted".
func ValidateLevels(levels int) error {
	if levels != 2 && levels != 4 && levels != 16 && levels != 256 {
		return fmt.Errorf("%w: levels must be 2, 4, 16 or 256", ErrInvalidOptions)
	}
	return nil
}

// ValidateQuality checks an explicitly supplied encoding quality.
func ValidateQuality(quality int) error {
	if quality < 1 || quality > 100 {
		return fmt.Errorf("%w: quality must be between 1 and 100", ErrInvalidOptions)
	}
	return nil
}
