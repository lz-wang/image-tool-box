package barcode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/disintegration/imaging"
	"imagetoolbox/internal/nativebin"
)

func nativeTestDecoder(t *testing.T) nativeDecoder {
	t.Helper()
	path := os.Getenv("ITB_TEST_ZXING_READER")
	if path == "" {
		nativebin.Init(os.DirFS("../.."))
		var err error
		path, err = nativebin.Ensure(nativebin.ZXingReader)
		if err != nil {
			if os.Getenv("ITB_REQUIRE_BARCODE_NATIVE") == "1" {
				t.Fatal(err)
			}
			t.Skipf("native reader not available: %v", err)
		}
	}
	return nativeDecoder{path: path}
}

func assertNativeCode(t *testing.T, d Decoder, img image.Image, s Symbology, text string) {
	t.Helper()
	r, err := decodeImage(context.Background(), img, allSymbologies(), d)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Codes) != 1 || r.Codes[0].Text != text || r.Codes[0].Symbology != s || len(r.Codes[0].Points) != 4 {
		t.Fatalf("decode: %+v", r)
	}
}

func TestNativeRoundTrip(t *testing.T) {
	d := nativeTestDecoder(t)
	for _, tc := range []struct {
		s          Symbology
		data, text string
	}{{QRCode, "hello 世界", "hello 世界"}, {Code128, "ABC123", "ABC123"}, {Code39, "ABC-123", "ABC-123"}, {EAN13, "690123456789", "6901234567892"}, {EAN8, "9638507", "96385074"}} {
		t.Run(string(tc.s), func(t *testing.T) {
			o := DefaultGenerateOptions()
			o.Symbology = tc.s
			o.Data = tc.data
			p, err := GeneratePlan(o)
			if err != nil {
				t.Fatal(err)
			}
			img, err := p.Render(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			assertNativeCode(t, d, img, tc.s, tc.text)
		})
	}
	for _, level := range []string{"L", "M", "Q", "H"} {
		t.Run("ecc-"+level, func(t *testing.T) {
			o := DefaultGenerateOptions()
			o.Data = "ECC-123"
			o.ErrorCorrection = level
			p, _ := GeneratePlan(o)
			img, _ := p.Render(context.Background())
			assertNativeCode(t, d, img, QRCode, o.Data)
		})
	}
}

func TestNativeScanScenes(t *testing.T) {
	d := nativeTestDecoder(t)
	o := DefaultGenerateOptions()
	o.Data = "rotate"
	p, _ := GeneratePlan(o)
	img, _ := p.Render(context.Background())
	for _, tc := range []struct {
		name string
		img  image.Image
	}{{"0", img}, {"90", imaging.Rotate90(img)}, {"180", imaging.Rotate180(img)}, {"270", imaging.Rotate270(img)}, {"inverted", imaging.Invert(img)}} {
		t.Run(tc.name, func(t *testing.T) { assertNativeCode(t, d, tc.img, QRCode, "rotate") })
	}
	o.ModuleSize = 2
	p, _ = GeneratePlan(o)
	small, _ := p.Render(context.Background())
	assertNativeCode(t, d, small, QRCode, "rotate")
	r, err := decodeImage(context.Background(), image.NewGray(image.Rect(0, 0, 50, 50)), allSymbologies(), d)
	if err != nil || len(r.Codes) != 0 {
		t.Fatal(r, err)
	}
	r, err = decodeImage(context.Background(), img, []Symbology{EAN13}, d)
	if err != nil || len(r.Codes) != 0 {
		t.Fatal("filter", r, err)
	}
	canvas := image.NewGray(image.Rect(0, 0, 900, 700))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	for i, tc := range []struct {
		s    Symbology
		data string
	}{{QRCode, "multi"}, {Code128, "ABC123"}, {EAN13, "690123456789"}} {
		o = DefaultGenerateOptions()
		o.Symbology = tc.s
		o.Data = tc.data
		p, _ = GeneratePlan(o)
		part, _ := p.Render(context.Background())
		pos := image.Pt(40, 40+i*210)
		if i == 0 {
			pos = image.Pt(500, 30)
		}
		draw.Draw(canvas, part.Bounds().Add(pos), part, image.Point{}, draw.Src)
	}
	r, err = decodeImage(context.Background(), canvas, allSymbologies(), d)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Codes) != 3 {
		t.Fatalf("expected all three codes: %+v", r)
	}
	seen := map[Symbology]string{}
	for _, c := range r.Codes {
		seen[c.Symbology] = c.Text
	}
	if seen[QRCode] != "multi" || seen[Code128] != "ABC123" || seen[EAN13] != "6901234567892" {
		t.Fatal(seen)
	}
	// Two QR symbols with the same payload must both survive.
	canvas = image.NewGray(image.Rect(0, 0, 700, 350))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	for _, x := range []int{20, 370} {
		draw.Draw(canvas, img.Bounds().Add(image.Pt(x, 20)), img, image.Point{}, draw.Src)
	}
	r, err = decodeImage(context.Background(), canvas, allSymbologies(), d)
	if err != nil || len(r.Codes) != 2 {
		t.Fatalf("two identical payloads: %+v %v", r, err)
	}
}

func TestNativeMicroAndRMQR(t *testing.T) {
	d := nativeTestDecoder(t)
	writer := os.Getenv("ITB_TEST_ZXING_WRITER")
	if writer == "" {
		if os.Getenv("ITB_REQUIRE_BARCODE_NATIVE") == "1" {
			t.Fatal("ITB_TEST_ZXING_WRITER is required")
		}
		t.Skip("test-only writer not configured")
	}
	for _, s := range []Symbology{MicroQRCode, RMQRCode} {
		t.Run(string(s), func(t *testing.T) {
			cmd := exec.Command(writer, string(s), "12345")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			data, err := cmd.Output()
			if err != nil {
				t.Fatal(err, stderr.String())
			}
			r := bufio.NewReader(bytes.NewReader(data))
			magic, err := r.ReadString('\n')
			if err != nil || magic != "P5\n" {
				t.Fatal("invalid PGM")
			}
			dimensions, _ := r.ReadString('\n')
			var w, h int
			if _, err = fmt.Sscanf(dimensions, "%d %d", &w, &h); err != nil || w <= 0 || h <= 0 {
				t.Fatal(dimensions)
			}
			max, _ := r.ReadString('\n')
			if max != "255\n" {
				t.Fatal(max)
			}
			img := image.NewGray(image.Rect(0, 0, w, h))
			if _, err = io.ReadFull(r, img.Pix); err != nil {
				t.Fatal(err)
			}
			// Create the PNG in the test, with no committed binary fixture.
			path := filepath.Join(t.TempDir(), "code.png")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, img)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
			result, err := decodeFile(context.Background(), path, DecodeOptions{}, d)
			if err != nil || len(result.Codes) != 1 || result.Codes[0].Symbology != s || result.Codes[0].Text != "12345" {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestNativeEXIFAndProtocol(t *testing.T) {
	d := nativeTestDecoder(t)
	o := DefaultGenerateOptions()
	o.Data = "exif"
	p, _ := GeneratePlan(o)
	img, _ := p.Render(context.Background())
	canvas := image.NewGray(image.Rect(0, 0, 400, 320))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(canvas, img.Bounds(), img, image.Point{}, draw.Src)
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, canvas, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	// Little-endian TIFF, Orientation=6 (logical dimensions swap).
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	var length [2]byte
	binary.BigEndian.PutUint16(length[:], uint16(len(exif)+2))
	data := append([]byte{0xff, 0xd8, 0xff, 0xe1}, length[:]...)
	data = append(data, exif...)
	data = append(data, jpegData.Bytes()[2:]...)
	path := filepath.Join(t.TempDir(), "exif.jpg")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := decodeFile(context.Background(), path, DecodeOptions{}, d)
	if err != nil || result.Input.Width != 320 || result.Input.Height != 400 || len(result.Codes) != 1 || result.Codes[0].Text != "exif" {
		t.Fatalf("EXIF: %+v %v", result, err)
	}
	for _, frame := range [][]byte{nil, []byte("bad protocol"), append([]byte("ITBZ\x01"), make([]byte, 16)...)} {
		cmd := exec.Command(d.path)
		cmd.Stdin = bytes.NewReader(frame)
		if err := cmd.Run(); err == nil {
			t.Fatal("accepted invalid frame")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	if _, err := d.Decode(ctx, img, allSymbologies()); err == nil {
		t.Fatal("ignored deadline")
	}
}
