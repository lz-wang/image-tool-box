package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"imagetoolbox/internal/barcode"
)

func TestBarcodeCLIContract(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "code.png")
	var out, stderr bytes.Buffer
	err := ExecuteArgs(context.Background(), "test", []string{"itb", "barcode", "generate", "ean13", "690123456789", dst, "--format", "json"}, &out, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	r := decodeSingleJSON(t, out.String())
	if r["schema_version"] != "itb.barcode.generate.v1" || r["encoded_text"] != "6901234567892" || stderr.Len() != 0 {
		t.Fatal(r, stderr.String())
	}
	for _, tc := range []struct {
		name     string
		args     []string
		code, op string
	}{
		{"conflict", []string{"generate", "qr", "hello", dst}, CodeTargetConflict, "barcode.generate"},
		{"ean", []string{"generate", "ean8", "bad", filepath.Join(dir, "bad.png")}, CodeInvalidArgument, "barcode.generate"},
		{"symbology", []string{"generate", "micro-qr", "hello", filepath.Join(dir, "micro.png")}, CodeInvalidArgument, "barcode.generate"},
		{"operands", []string{"generate", "qr"}, CodeInvalidArgument, "barcode.generate"},
		{"missing", []string{"decode", filepath.Join(dir, "missing.png")}, CodeFileNotFound, "barcode.decode"},
		{"bad-flag", []string{"generate", "qr", "hello", dst, "--module-size", "bad"}, CodeInvalidArgument, "barcode.generate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out.Reset()
			stderr.Reset()
			args := append([]string{"itb", "barcode"}, tc.args...)
			args = append(args, "--format=json")
			err := ExecuteArgs(context.Background(), "test", args, &out, &stderr)
			if !errors.Is(err, ErrReported) {
				t.Fatal(err)
			}
			r := decodeSingleJSON(t, out.String())
			if r["schema_version"] != MachineErrorSchemaVersion || r["operation"] != tc.op || r["error"].(map[string]any)["code"] != tc.code || stderr.Len() != 0 {
				t.Fatal(r, stderr.String())
			}
		})
	}
	bad := filepath.Join(dir, "corrupt.png")
	if err := os.WriteFile(bad, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	_ = ExecuteArgs(context.Background(), "test", []string{"itb", "barcode", "decode", bad, "--format=json"}, &out, &stderr)
	if decodeSingleJSON(t, out.String())["error"].(map[string]any)["code"] != CodeFileRead {
		t.Fatal(out.String())
	}
}

func TestBarcodeHelp(t *testing.T) {
	for _, sub := range []string{"generate", "decode"} {
		var out, errout bytes.Buffer
		if err := ExecuteArgs(context.Background(), "test", []string{"itb", "barcode", sub, "--help"}, &out, &errout); err != nil {
			t.Fatal(err)
		}
		for _, section := range []string{"DEFAULTS:", "CONSTRAINTS:", "EXAMPLES:"} {
			if !strings.Contains(out.String(), section) {
				t.Fatal(sub, section)
			}
		}
	}
	for _, err := range []error{barcode.ErrInvalidOptions, barcode.ErrInvalidData, barcode.ErrInvalidSymbology} {
		if classifyError(err).Code != CodeInvalidArgument {
			t.Fatal(err)
		}
	}
	if classifyError(barcode.ErrDecoderUnavailable).Code != CodeUnsupportedCapability {
		t.Fatal("capability mapping")
	}
}
