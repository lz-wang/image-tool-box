package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"imagetoolbox/internal/nativebin"
)

func TestBarcodeCLIE2E(t *testing.T) {
	binary := buildE2EBinary(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "code.png")
	out, stderr, exit := runE2E(t, binary, dir, "barcode", "generate", "qr", "hello", dst, "--format=json")
	if exit != 0 || stderr != "" || decodeSingleJSON(t, out)["schema_version"] != "itb.barcode.generate.v1" {
		t.Fatal(exit, out, stderr)
	}
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"generate", "qr", "hello", dst}, CodeTargetConflict},
		{[]string{"generate", "ean13", "invalid", filepath.Join(dir, "bad.png")}, CodeInvalidArgument},
		{[]string{"generate", "aztec", "hello", filepath.Join(dir, "bad.png")}, CodeInvalidArgument},
		{[]string{"decode", filepath.Join(dir, "missing.png")}, CodeFileNotFound},
	} {
		args := append([]string{"barcode"}, tc.args...)
		args = append(args, "--format=json")
		out, stderr, exit = runE2E(t, binary, dir, args...)
		r := decodeSingleJSON(t, out)
		if exit == 0 || stderr != "" || r["schema_version"] != MachineErrorSchemaVersion || r["error"].(map[string]any)["code"] != tc.code {
			t.Fatal(r, exit, stderr)
		}
	}
	corrupt := filepath.Join(dir, "broken.png")
	if err := os.WriteFile(corrupt, []byte("bad image"), 0600); err != nil {
		t.Fatal(err)
	}
	out, stderr, exit = runE2E(t, binary, dir, "barcode", "decode", corrupt, "--format=json")
	if exit == 0 || stderr != "" || decodeSingleJSON(t, out)["error"].(map[string]any)["code"] != CodeFileRead {
		t.Fatal(out, stderr, exit)
	}
}

func TestBarcodeNativeCLIE2E(t *testing.T) {
	nativebin.Init(os.DirFS("../.."))
	if _, err := nativebin.Ensure(nativebin.ZXingReader); err != nil {
		if os.Getenv("ITB_REQUIRE_BARCODE_NATIVE") == "1" {
			t.Fatal(err)
		}
		t.Skipf("embedded reader unavailable: %v", err)
	}
	binary := buildE2EBinary(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "code.png")
	out, stderr, exit := runE2E(t, binary, dir, "barcode", "generate", "qr", "hello", dst, "--format=json")
	if exit != 0 || stderr != "" {
		t.Fatal(out, stderr, exit)
	}
	out, stderr, exit = runE2E(t, binary, dir, "barcode", "decode", dst, "--format=json")
	r := decodeSingleJSON(t, out)
	if exit != 0 || stderr != "" || r["schema_version"] != "itb.barcode.decode.v1" || len(r["codes"].([]any)) != 1 || r["codes"].([]any)[0].(map[string]any)["text"] != "hello" {
		t.Fatal(r, exit, stderr)
	}
	out, stderr, exit = runE2E(t, binary, dir, "barcode", "decode", dst, "--symbology", "ean13", "--format=json")
	r = decodeSingleJSON(t, out)
	if exit != 0 || stderr != "" || len(r["codes"].([]any)) != 0 {
		t.Fatal(r, exit, stderr)
	}
}
