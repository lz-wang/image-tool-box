package imageio

import "fmt"

// OutputError identifies failures creating, encoding, closing or committing an
// output file. HTTP adapters can distinguish server-side storage/encoding errors
// from invalid input without matching operating-system error messages.
type OutputError struct {
	Err error
}

func (e *OutputError) Error() string { return fmt.Sprintf("image output failed: %v", e.Err) }

// Unwrap preserves the underlying I/O error for errors.Is and errors.As.
func (e *OutputError) Unwrap() error { return e.Err }
