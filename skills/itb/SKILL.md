---
name: itb
description: Use the `itb` CLI in image-processing workflows. Trigger when a user asks to compress, crop, resize, convert, watermark, batch-process images, upload images to S3-compatible storage, upload images to LskyPro, or choose the right `itb` command/flags for an image workflow.
---

# ITB

Use this skill to turn image-processing requests into safe, concrete `itb` CLI commands.

## Core Workflow

1. Prefer an existing `itb` executable available in the current workspace or shell path.
2. Inspect the source image(s) before destructive operations. Preserve originals unless the user explicitly wants in-place mutation.
3. Choose the narrowest command:
   - `compress` for PNG/JPEG size reduction.
   - `crop` for percentage-based anchored cuts.
   - `resize` for dimensions, aspect-ratio fitting, filling, stretching, or percentage scaling.
   - `convert` for `jpg` / `jpeg` / `png` / `webp` conversion.
   - `watermark` for text or image watermarking.
   - `batch` for repeated `resize`, `convert`, or `watermark` over directories.
   - `s3` for S3-compatible upload/download/list/delete.
   - `lsky upload` for LskyPro image hosting.
4. Prefer explicit `-o` / `--output` for single-file transformations so follow-up steps can use predictable paths.
5. For multi-step local image pipelines, write intermediate outputs to a temporary or task-specific output directory and run commands in sequence.
6. Verify outputs with file existence, dimensions/format checks, or a visual preview when the result is user-facing.

## Command Use

Load `references/itb-command-reference.md` when exact flags, examples, defaults, environment variables, or cloud upload settings are needed.

Common patterns:

```bash
itb resize -i input.jpg -o output.jpg --width 1200 --mode fit
itb convert -i output.jpg -o output.webp --to webp -q 85
itb watermark -i output.webp -o marked.webp -t "Draft" --mode repeat --opacity 0.25
```

Batch examples:

```bash
itb batch resize --input-dir ./images --output-dir ./out --recursive --width 1200
itb batch convert --input-dir ./images --output-dir ./out --glob "*.png" --to webp --skip-existing
```

## Safety Rules

- Do not omit `-o` with `compress` unless overwriting the original is intended; `compress` overwrites input when no output is provided.
- Treat `s3 delete` as destructive; use `-f` only when the user clearly requested non-interactive deletion.
- Do not print secrets. Prefer environment variables for `S3_*`, `AWS_*`, and `LSKY_*` credentials.
- Use `--force-path-style` for MinIO-style endpoints when needed.
- For `watermark`, use either text (`-t`) or image (`--image`) watermarks. Image watermarks only support `position` mode; tiled image watermarks are not supported.
- When a result is user-facing, confirm the expected output path exists and preview the image when practical.
