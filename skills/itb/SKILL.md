---
name: itb
description: Use the `itb` CLI for image compression, crop, resize, rotation, conversion, watermarking, inspection, QR/barcode PNG generation and decoding (including Micro QR and rMQR), S3-compatible storage operations, or running the trusted `itb serve` HTTP API.
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
   - `rotate` for clockwise/counter-clockwise image rotation by any angle (positive = counter-clockwise).
   - `convert` for `jpg` / `jpeg` / `png` / `webp` conversion (inputs limited to these formats).
   - `watermark` for text or image watermarking.
   - `compare` for objective image-quality comparison using PSNR / SSIM / MS-SSIM.
   - `barcode generate` for QR/Code128/Code39/EAN13/EAN8 PNGs, and `barcode decode` for those formats plus Micro QR/rMQR, including multiple codes and positions. Micro QR/rMQR are decode-only.
   - `inspect` for metadata and file hash checks; add `--strict --full-decode` as an upload preflight (catches corrupted tails and reports GIF frame/animation info). Content is recognized from the file itself (PNG/JPEG/GIF/WebP/BMP/TIFF raster-decodable; SVG recognized but not raster-decoded); use `--hash <algo>` to compute only needed hashes.
   - `s3` for S3-compatible upload/download/list/stat/delete (`s3 list` is single-page by default; add `--all` for complete pagination and check `complete` / `next_continuation_token` in the `itb.s3.list.v2` JSON).
   - `serve` for the trusted HTTP API used by remote automation or a personal VPS deployment.
4. Use transform command operands as `itb <command> [options] <src> [dst]`; `convert` requires `<dst>`, while the other transforms may derive one. `compare` instead takes `itb compare [options] <src> <dst>` where both operands are read-only inputs (the `dst` is the comparison target, not an output file).
5. For multi-step local image pipelines, write intermediate outputs to a temporary or task-specific output directory and run commands in sequence.
6. Verify outputs with file existence, dimensions/format checks, or a visual preview when the result is user-facing.

CLI vs HTTP API: prefer CLI commands for local and scripted workflows. Use `itb serve` only when an HTTP integration is required; it exposes the same domain image operations but does not provide a browser UI.

## Command Use

For exact syntax, flags, defaults, and constraints, run `itb <command> --help` first. The installed binary's help is authoritative for that installed version — it always reflects the exact CLI you are driving, even when these skill files lag behind. Use `references/itb-command-reference.md` as supplemental guidance when deeper examples, environment variables, or cloud upload settings are needed.

Common patterns:

```bash
itb resize --width 1200 --mode fit input.jpg output.jpg
itb convert -q 85 output.jpg output.webp
itb watermark -t "Draft" --mode repeat --opacity 0.25 output.webp marked.webp
itb barcode generate qr 'https://example.com' code.png --format json
itb barcode decode code.png --symbology qr --symbology micro-qr --symbology rmqr --format json
```

## Safety Rules

- Barcode generation requires `barcode generate <symbology> <data> <dst.png>`; existing targets require explicitly intended `--force`. Sizes are pixels/modules, never printer millimeters or DPI. Linear text uses a fixed embedded font. EAN accepts payload or a checked full code; read `encoded_text` to get the complete checksum-bearing value.
- Barcode decoding accepts JPEG/PNG/WebP and returns `itb.barcode.decode.v1`; `codes: []` means a successful scan with no detections. Preserve all returned positions, including identical text at different positions. Prefer `--format json` for automation; failures use the existing single-document `itb.error.v1` contract. Do not install Python/uv or a separate ZXing binary for this workflow.
- `convert <src> <dst>` accepts only `jpg`/`jpeg`/`png`/`webp` inputs; its target format is determined only by the `dst` extension. It preserves alpha for PNG/WebP output (lossy and lossless alike) and only flattens transparent areas onto `--background` when converting to JPEG.
- All transforms (`convert`/`resize`/`crop`/`rotate`/`watermark`) share the same input contract: only `jpg`/`jpeg`/`png`/`webp` (GIF/BMP/TIFF rejected — animated GIFs are never silently reduced to their first frame), and JPEG EXIF orientation is baked into the pixels, so percent-based crops and resize plans always operate on the rotated (logical) dimensions.
- `rotate --angle <degrees>` takes a signed angle in `(-360, 360)`, never `0`: positive rotates counter-clockwise, negative clockwise. Exact `90/180/270` are interpolation-free; arbitrary angles adjust the output canvas as needed, keeping uncovered areas transparent for PNG/WebP and flattening them onto white for JPEG. File encoding still follows the destination format: JPEG and lossy WebP are re-encoded and are not file-level lossless.
- `compress` keeps the input and writes `<name>_compressed.<ext>` by default; pass `--in-place` only when the user explicitly wants to overwrite the original (`--in-place` cannot be combined with a `[dst]` operand).
- Never choose an output path that aliases any input resource. `itb` rejects equivalent paths, hard links, and symlinks that resolve to the same file. For image watermarks this also applies to `--image`.
- `compare` never modifies either operand; same-file comparison is valid (`itb compare a.jpg a.jpg` prints `PSNR: +Inf dB`). Both images must share identical logical dimensions — there is no implicit resize.
- Treat `s3 delete` as destructive; use `-f` only when the user clearly requested non-interactive deletion.
- Use S3 resource operands positionally: `upload <src> [key]`, `download <key> [dst]`, `stat/delete <key>`, and `list [prefix]`. Do not use the removed `--input`, `--output`, `--key`, or `--prefix` flags.
- Do not print secrets. Prefer environment variables for `ITB_S3_*` credentials, including the temporary-credential `ITB_S3_SESSION_TOKEN` (session tokens must not land in shell history).
- Use `--force-path-style` (or `ITB_S3_FORCE_PATH_STYLE`) for MinIO-style endpoints; loopback and `:9000` endpoints enable path style automatically.
- When uploading for publishing, attach provenance with `--metadata key=value` (e.g. `source-sha256`, `width`, `height`) and `--cache-control no-cache` for stable-URL images; `itb-sha256` is a reserved metadata key.
- For `watermark`, use either text (`-t`) or image (`--image`) watermarks. Image watermarks only support `position` mode; tiled image watermarks are not supported.
- `serve` binds to `127.0.0.1` by default; never suggest `0.0.0.0` on untrusted networks.
- `serve` exposes an HTTP API only. It has no WebUI and no `--open` flag. Authentication requires `ITB_API_TOKEN` unless `--no-auth` is explicitly used on a loopback address.
- When a result is user-facing, confirm the expected output path exists and preview the image when practical.
