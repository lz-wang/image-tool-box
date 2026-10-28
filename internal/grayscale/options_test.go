package grayscale

import (
	"errors"
	"testing"
)

func TestExplicitValuesAndDefaults(t *testing.T) {
	for _, tc := range []struct {
		name           string
		validate       func(int) error
		valid, invalid []int
	}{
		{"levels", ValidateLevels, []int{2, 4, 16, 256}, []int{-1, 0, 3, 255, 257}},
		{"quality", ValidateQuality, []int{1, 90, 100}, []int{-1, 0, 101}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.valid {
				if err := tc.validate(v); err != nil {
					t.Fatalf("%d: %v", v, err)
				}
			}
			for _, v := range tc.invalid {
				if err := tc.validate(v); !errors.Is(err, ErrInvalidOptions) {
					t.Fatalf("%d: %v", v, err)
				}
			}
		})
	}
	opts := Options{}
	opts.Normalize()
	if opts.Levels != 256 || opts.Quality != 90 {
		t.Fatalf("defaults: %+v", opts)
	}
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
}
