package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v3"
	"imagetoolbox/internal/barcode"
)

func newBarcodeCommand() *cli.Command {
	defaults := barcode.DefaultGenerateOptions()
	return &cli.Command{Name: "barcode", Usage: "Generate and decode QR codes and common barcodes", Category: categoryUtility,
		Action: func(_ context.Context, c *cli.Command) error {
			return operationError("barcode", invalidArgument("需要选择 generate 或 decode 子命令"))
		},
		Commands: []*cli.Command{
			{Name: "generate", Usage: "Generate a PNG barcode", ArgsUsage: "<symbology> <data> <dst>",
				Description: `Generate QR, Code128, Code39, EAN13 or EAN8 as PNG.

DEFAULTS:
  QR: error correction M, module size 10, border 4.
  Linear: module width 2 px, bar height 80 px, text enabled;
  at least ten-module horizontal quiet zones and 10 px vertical margins.

CONSTRAINTS:
  Output must be PNG. Missing parent directories are created.
  Existing destinations require --force; Windows replacement is not guaranteed atomic.
  Linear output widens to fit complete text with 10 px horizontal margins.
  micro-qr and rmqr are decode-only. Dimensions use integer pixels.
  Code39 is standard uppercase ASCII without checksum.
  Code128 accepts 1..80 ASCII characters. Generation is bounded
  to 512 MiB working memory. EAN accepts payload or checked full code.

EXAMPLES:
  itb barcode generate qr hello code.png --format json
  itb barcode generate qr hello code.png --error-correction H --module-size 8
  itb barcode generate code128 ABC123 code.png --module-width 2 --module-height 80
  itb barcode generate ean13 690123456789 code.png --no-draw-text`,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "error-correction", Value: defaults.ErrorCorrection, Usage: "QR error correction: L/M/Q/H"},
					&cli.IntFlag{Name: "module-size", Value: defaults.ModuleSize, Usage: "Pixels per QR module"},
					&cli.IntFlag{Name: "border", Value: defaults.Border, Usage: "QR quiet zone in modules (zero allowed)"},
					&cli.IntFlag{Name: "module-width", Value: defaults.ModuleWidth, Usage: "Pixels per narrow linear module"},
					&cli.IntFlag{Name: "module-height", Value: defaults.ModuleHeight, Usage: "Linear bar height in pixels"},
					&cli.BoolFlag{Name: "no-draw-text", Usage: "Omit human-readable linear barcode text"},
					&cli.BoolFlag{Name: "force", Usage: "Replace an existing destination after staging a complete PNG (atomic on Unix)"},
					&cli.StringFlag{Name: "format", Value: "table", Usage: "Output format: table/json", Validator: enumValidator("format", "table", "json")},
				}, Action: func(ctx context.Context, c *cli.Command) error {
					return operationError("barcode.generate", runBarcodeGenerate(ctx, c))
				}},
			{Name: "decode", Usage: "Read all supported codes from an image", ArgsUsage: "<src>",
				Description: `Decode QR, Micro QR, rMQR, Code128, Code39, EAN13 and EAN8.

DEFAULTS:
  All seven symbologies are searched. Multiple codes and positions
  are returned. JPEG EXIF Orientation is normalized.

CONSTRAINTS:
  Input must be JPEG/PNG/WebP; it is never modified.
  Zero detected codes is a successful result (codes=[]).
  Input is bounded to 64 Mi pixels, 32768 pixels per dimension.

EXAMPLES:
  itb barcode decode code.png --format json
  itb barcode decode code.png --symbology qr --symbology micro-qr --symbology rmqr`,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "symbology", Usage: "Limit scanning to a symbology (repeatable)"},
					&cli.StringFlag{Name: "format", Value: "table", Usage: "Output format: table/json", Validator: enumValidator("format", "table", "json")},
				}, Action: func(ctx context.Context, c *cli.Command) error {
					return operationError("barcode.decode", runBarcodeDecode(ctx, c))
				}},
		},
	}
}

func runBarcodeGenerate(ctx context.Context, c *cli.Command) error {
	if c.NArg() != 3 {
		return invalidArgument("需要提供 <symbology> <data> <dst>")
	}
	opts := barcode.GenerateOptions{Symbology: barcode.Symbology(c.Args().Get(0)), Data: c.Args().Get(1), ErrorCorrection: c.String("error-correction"), ModuleSize: c.Int("module-size"), Border: c.Int("border"), DrawText: !c.Bool("no-draw-text"), ModuleWidth: c.Int("module-width"), ModuleHeight: c.Int("module-height")}
	r, err := barcode.GenerateFile(ctx, c.Args().Get(2), opts, c.Bool("force"))
	if err != nil {
		return err
	}
	if c.String("format") == "json" {
		return json.NewEncoder(c.Root().Writer).Encode(r)
	}
	_, err = fmt.Fprintf(c.Root().Writer, "Generated %s: %s (%dx%d, %d bytes)\n", r.Symbology, r.Output.Path, r.Output.Width, r.Output.Height, r.Output.SizeBytes)
	return err
}

func runBarcodeDecode(ctx context.Context, c *cli.Command) error {
	src, err := sourceArg(c)
	if err != nil {
		return err
	}
	var formats []barcode.Symbology
	for _, s := range c.StringSlice("symbology") {
		formats = append(formats, barcode.Symbology(s))
	}
	r, err := barcode.DecodeFile(ctx, src, barcode.DecodeOptions{Symbologies: formats})
	if err != nil {
		return err
	}
	if c.String("format") == "json" {
		return json.NewEncoder(c.Root().Writer).Encode(r)
	}
	for _, code := range r.Codes {
		if _, err = fmt.Fprintf(c.Root().Writer, "%s\t%s\n", code.Symbology, code.Text); err != nil {
			return err
		}
	}
	if len(r.Codes) == 0 {
		_, err = fmt.Fprintln(c.Root().Writer, "No codes detected")
	}
	return err
}
