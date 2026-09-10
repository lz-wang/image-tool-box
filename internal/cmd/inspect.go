package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"imagetoolbox/internal/filehash"
	"imagetoolbox/internal/inspect"
)

func newInspectCommand() *cli.Command {
	return &cli.Command{
		Name:      "inspect",
		Usage:     "Inspect image info, metadata, and file hashes",
		Category:  categoryAnalysis,
		ArgsUsage: "<src>",
		Description: `Inspect a local image file: file information, content
recognition, basic image properties, detailed metadata, and
file hashes.

Content is recognized from the file itself (magic bytes plus
streamed XML for SVG), never from its extension. PNG, JPEG,
GIF, WebP, BMP, and TIFF are raster-decodable; SVG is
recognized as an image (recognized=true) but is never
raster-decoded (decode_supported=false) — that is not an
error, and files without explicit SVG dimensions are valid.

This command is read-only: it does not modify the source
image. Detailed metadata and all hashes are included by
default; use --no-detail / --hash / --no-hash to trim the
output.

--full-decode decodes the file completely (GIF frame by
frame), catching files whose header is fine but whose tail
is corrupted, and reports the frame count and animation
state. Combine it with --strict as an upload preflight.

EXAMPLES:
  itb inspect photo.jpg
  itb inspect --format json photo.jpg
  itb inspect --format plain photo.jpg
  itb inspect --no-detail photo.jpg
  itb inspect --no-hash photo.jpg
  itb inspect --hash sha256 --no-detail --format json image.png
  itb inspect --strict --full-decode --format json image.png`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:      "format",
				Value:     "table",
				Usage:     "Output `FORMAT`: table/json/plain (plain prints the SHA-256 only)",
				Validator: enumValidator("format", "table", "json", "plain"),
			},
			&cli.BoolFlag{
				Name:   "detail",
				Value:  true,
				Hidden: true,
				Usage:  "Include detailed metadata (compatibility flag; use --no-detail to skip)",
			},
			&cli.BoolFlag{
				Name:  "no-detail",
				Usage: "Skip detailed metadata",
			},
			&cli.BoolFlag{
				Name:  "strict",
				Usage: "Return an error when image parsing fails",
			},
			&cli.BoolFlag{
				Name:  "full-decode",
				Usage: "Fully decode the image (GIF frame by frame) to verify the file tail and report the frame count / animation state",
			},
		},
		// --hash 与 --no-hash 是互斥的哈希策略
		MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{
			{
				Flags: [][]cli.Flag{
					{
						&cli.StringSliceFlag{
							Name:  "hash",
							Usage: "Compute only the specified file `ALGO` (repeatable: sha256/sha1/md5/crc32); without --hash all algorithms are computed",
						},
					},
					{
						&cli.BoolFlag{
							Name:  "no-hash",
							Usage: "Skip file hash computation",
						},
					},
				},
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return operationError("inspect", runInspect(ctx, cmd))
		},
	}
}

func runInspect(ctx context.Context, cmd *cli.Command) error {
	inputFile, err := sourceArg(cmd)
	if err != nil {
		return err
	}

	algorithms, err := filehash.Parse(cmd.StringSlice("hash"))
	if err != nil {
		return err
	}

	result, err := inspect.File(inputFile, inspect.Options{
		Detail:     cmd.Bool("detail") && !cmd.Bool("no-detail"),
		NoHash:     cmd.Bool("no-hash"),
		Hashes:     algorithms,
		Strict:     cmd.Bool("strict"),
		FullDecode: cmd.Bool("full-decode"),
	})
	if err != nil {
		return err
	}

	switch cmd.String("format") {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)

	case "plain":
		if result.Hashes == nil || result.Hashes.SHA256 == "" {
			return invalidArgument("plain 输出需要 sha256；请移除 --no-hash 或加上 --hash sha256")
		}
		fmt.Fprintln(os.Stdout, result.Hashes.SHA256)
		return nil

	case "table":
		return inspect.PrintTable(os.Stdout, result)

	default:
		return invalidArgument("不支持的输出格式: %s（支持: table, json, plain）", cmd.String("format"))
	}
}
