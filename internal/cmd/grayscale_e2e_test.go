package cmd

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"testing"

	"imagetoolbox/internal/grayscale"
	"imagetoolbox/internal/imageio"
)

func TestGrayscaleBinaryE2E(t *testing.T) {
	binary := buildE2EBinary(t)
	dir := t.TempDir()
	src := writeE2EImage(t, dir, "photo.jpg")
	stdout, stderr, code := runE2E(t, binary, dir, "grayscale", src)
	if code != 0 || stderr != "" {
		t.Fatalf("%d %s %s", code, stdout, stderr)
	}
	dst := filepath.Join(dir, "photo_grayscale.png")
	img, err := imageio.OpenStatic(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*image.Gray); !ok {
		t.Fatalf("not Gray8: %T", img)
	}
	stdout, stderr, code = runE2E(t, binary, dir, "grayscale", "--levels", "16", "--dither", "fs", src, "eink.png")
	if code != 0 || stderr != "" {
		t.Fatalf("%d %s %s", code, stdout, stderr)
	}
	want := filepath.Join(dir, "domain.png")
	if err := grayscale.ConvertFile(src, want, grayscale.Options{Levels: 16, Dither: grayscale.DitherFS}); err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "eink.png"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("CLI/domain differ")
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing", []string{"missing.png"}, CodeFileNotFound},
		{"levels", []string{"--levels", "3", src}, CodeInvalidArgument},
		{"levels zero", []string{"--levels", "0", src}, CodeInvalidArgument},
		{"dither", []string{"--dither", "fs", src}, CodeInvalidArgument},
		{"background", []string{"--background", "#00000000", src}, CodeInvalidArgument},
		{"quality", []string{"--quality", "0", src}, CodeInvalidArgument},
		{"quality above range", []string{"--quality", "101", src}, CodeInvalidArgument},
		{"lossless jpeg", []string{"--lossless", src, "out.jpg"}, CodeInvalidArgument},
		{"operands", nil, CodeInvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"grayscale", "--format", "json"}, tc.args...)
			out, errs, code := runE2E(t, binary, dir, args...)
			if code == 0 || errs != "" {
				t.Fatalf("%d %s %s", code, out, errs)
			}
			doc := decodeSingleJSON(t, out)
			if doc["schema_version"] != MachineErrorSchemaVersion || doc["operation"] != "grayscale" || doc["error"].(map[string]any)["code"] != tc.want {
				t.Fatal(doc)
			}
		})
	}
	for _, ext := range []string{"gif", "bmp", "tiff"} {
		t.Run("reject "+ext, func(t *testing.T) {
			input := writeE2EImage(t, dir, "unsupported."+ext)
			out, stderr, code := runE2E(t, binary, dir, "grayscale", "--format", "json", input, "bad.png")
			if code == 0 || stderr != "" {
				t.Fatalf("%d %s %s", code, out, stderr)
			}
			doc := decodeSingleJSON(t, out)
			if doc["schema_version"] != MachineErrorSchemaVersion || doc["operation"] != "grayscale" || doc["error"].(map[string]any)["code"] != CodeInvalidArgument {
				t.Fatal(doc)
			}
		})
	}
}
