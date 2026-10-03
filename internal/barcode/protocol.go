package barcode

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"io"
)

const maxDecodePixels = 64 * 1024 * 1024

func allSymbologies() []Symbology {
	return []Symbology{QRCode, MicroQRCode, RMQRCode, Code128, Code39, EAN13, EAN8}
}

func normalizeSymbologies(values []Symbology) ([]Symbology, error) {
	if len(values) == 0 {
		return allSymbologies(), nil
	}
	result := make([]Symbology, 0, len(values))
	seen := map[Symbology]bool{}
	for _, value := range values {
		s, err := ParseSymbology(string(value))
		if err != nil {
			return nil, err
		}
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result, nil
}

func writeFrame(w io.Writer, img *image.Gray, formats []Symbology) error {
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	if width <= 0 || height <= 0 || width > 32768 || height > 32768 || int64(width)*int64(height) > maxDecodePixels {
		return fmt.Errorf("%w: decode dimensions exceed protocol limits", ErrInvalidImage)
	}
	header := make([]byte, 21)
	copy(header, "ITBZ")
	header[4] = 1
	binary.BigEndian.PutUint32(header[5:], uint32(width))
	binary.BigEndian.PutUint32(header[9:], uint32(height))
	binary.BigEndian.PutUint32(header[13:], uint32(width))
	var mask uint32
	for _, s := range formats {
		for bit, value := range allSymbologies() {
			if s == value {
				mask |= 1 << bit
			}
		}
	}
	if mask == 0 {
		return ErrInvalidSymbology
	}
	binary.BigEndian.PutUint32(header[17:], mask)
	if _, err := w.Write(header); err != nil {
		return err
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		start := img.PixOffset(b.Min.X, y)
		if _, err := w.Write(img.Pix[start : start+width]); err != nil {
			return err
		}
	}
	return nil
}

type RawCode struct {
	Format string  `json:"format"`
	Text   string  `json:"text"`
	Points []Point `json:"points"`
}

func parseResponse(r io.Reader) ([]RawCode, error) {
	var response struct {
		Version int       `json:"version"`
		Codes   []RawCode `json:"codes"`
	}
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(&response); err != nil {
		return nil, fmt.Errorf("%w: invalid response: %v", ErrDecoderFailure, err)
	}
	if response.Version != 1 || response.Codes == nil {
		return nil, fmt.Errorf("%w: missing codes or unsupported protocol version", ErrDecoderFailure)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing response data", ErrDecoderFailure)
	}
	if len(response.Codes) > 255 {
		return nil, fmt.Errorf("%w: too many results", ErrDecoderFailure)
	}
	return response.Codes, nil
}

func normalizeCode(raw RawCode) (Code, error) {
	mapping := map[string]Symbology{"QRCodeModel2": QRCode, "MicroQRCode": MicroQRCode, "RMQRCode": RMQRCode, "Code128": Code128, "Code39": Code39, "EAN13": EAN13, "EAN8": EAN8}
	s, ok := mapping[raw.Format]
	if !ok || len(raw.Points) != 4 {
		return Code{}, fmt.Errorf("%w: invalid result format or position", ErrDecoderFailure)
	}
	return Code{Text: raw.Text, Symbology: s, Points: raw.Points}, nil
}
