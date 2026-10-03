# HTTP API

`itb serve` exposes a trusted, versioned image-processing API at `/api/v1`.
It is not a remote shell: S3 management, workflows, user management, queues, and WebUI endpoints are intentionally absent.

All image operations require `Authorization: Bearer $ITB_API_TOKEN`. `GET /api/v1/health` does not require authentication and returns `{"status":"ok"}`.

Every operation uses `multipart/form-data`. `input` is the required source image file except for barcode generation, which accepts scalar fields only. Except for the transport-only `convert` field `to`, operation option names match the corresponding CLI long flag. Image-transforming endpoints stream binary downloads with `Content-Disposition`, `X-ITB-Input-Size`, `X-ITB-Output-Size`, and `X-ITB-Operation` headers. `inspect` and barcode decoding return JSON.

Scalar fields are limited to 4 KiB (16 KiB for `text`); an oversized field is rejected with `413 payload_too_large`. Uploaded files are stored under server-generated temporary names — client filenames are only used for download names — so identical filenames for `input` and `image` never collide.

## Operations

| Endpoint | Multipart fields |
| --- | --- |
| `POST /api/v1/compress` | `input`, `quality` (default `80`) |
| `POST /api/v1/resize` | `input`, `width`, `height`, `percent`, `mode`, `anchor`, `filter` |
| `POST /api/v1/crop` | `input`, `anchor`, `width`, `height` |
| `POST /api/v1/rotate` | `input`, `angle` |
| `POST /api/v1/convert` | `input`, `to`, `quality`, `lossless`, `background` |
| `POST /api/v1/watermark` | `input`, `text`, `image`, `mode`, `color`, `space`, `angle`, `opacity`, `font`, `font-size`, `position`, `margin`, `scale` |
| `POST /api/v1/inspect` | `input`, `detail`, `no-detail`, `no-hash`, `strict`, `full-decode` |
| `POST /api/v1/barcode/generate` | `symbology`, `data`, `error-correction`, `module-size`, `border`, `module-width`, `module-height`, `no-draw-text` (no file upload) |
| `POST /api/v1/barcode/decode` | `input`, optional comma-separated `symbology` |

`inspect` never requires a decodable image: with `strict` unset (or `false`) it returns file metadata plus an `error` object for undecodable inputs; with `strict=true` decoding failures return `400`.

`full-decode=true` fully decodes the image (frame-by-frame for GIF, validating the file tail) and extends the JSON (`itb.inspect.v2`) with `full_decode_ok` (omitted when not attempted), `frame_count` (GIF only), `animation_known`, and `animated`. With `strict=true` a full-decode failure returns `400`; otherwise the failure is reported in `warnings` and as `full_decode_ok: false`.

`width`, `height`, `quality`, `space`, and `font-size` are integers. The watermark `angle` is an integer; the rotate `angle` is a floating-point number. `opacity`, `margin`, and `scale` are floating-point numbers. Boolean fields accept the standard Go boolean forms, including `true`, `false`, `1`, and `0`.

For `resize`, `filter` accepts `nearest`, `linear`, `mitchell`, `catmullrom`, or `lanczos`. If omitted, `lanczos` is used. `mitchell` is the Mitchell-Netravali cubic filter — smoother output with less ringing than Catmull-Rom.

`convert` only accepts JPEG/PNG/WebP inputs; other decodable formats (GIF/BMP/TIFF) return `415 unsupported_format`. HTTP `to` is transport-only: the adapter constructs a temporary output path with that extension, and the convert domain layer derives its target format solely from that output path. The same input-format limit applies to every transform endpoint (`resize`/`crop`/`rotate`/`watermark` reject GIF/BMP/TIFF the same way), and the EXIF orientation of JPEG inputs is applied to the pixels for all of them (orientation metadata embedded in WebP files is not processed). Size admission and plan derivation run on the post-rotation logical dimensions, matching the decoded image bounds. Alpha is preserved for PNG/WebP output in both lossy and lossless modes. `background` applies to JPEG output only: an invalid or non-opaque value (e.g. `#00000000`) is rejected with `400 invalid_argument` when converting to JPEG, and background values are ignored for other targets.

`rotate` rotates by a floating-point `angle` in degrees: positive = counter-clockwise, negative = clockwise, range `(-360, 360)`, never `0`. Exact `90/180/270` are interpolation-free; arbitrary angles adjust the output canvas as needed (uncovered areas stay transparent for PNG/WebP and flatten onto white for JPEG). The planned output dimensions are admitted before allocation, so an angle that would expand a valid input beyond the limits returns `413 image_too_large`. The rotate working set (an NRGBA source copy plus the output canvas for arbitrary angles) is admitted against `--max-working-bytes` before allocation as well.

Unknown fields, duplicate fields, and legacy `file`, `watermark`, or `options` fields return `400`; they are not silently ignored.

### Barcode generation and decoding

Generation requires `symbology` and `data`; supported values are `qr`, `code128`, `code39`, `ean13` and `ean8`. Micro QR (`micro-qr`) and rMQR (`rmqr`) are decode-only. QR defaults are `error-correction=M` (L/M/Q/H), `module-size=10` pixels and `border=4` modules. Linear defaults are `module-width=2` pixels, `module-height=80` pixels and text enabled (`no-draw-text=false`), with ten-module horizontal quiet zones and 10 px vertical margins. All dimensions are integer pixels. Content restrictions and EAN checksum rules match the [CLI barcode contract](../README.en.md#barcode-generation-and-decoding).

Generation returns `image/png`, attachment `barcode.png`, `X-ITB-Operation: barcode.generate`, `X-ITB-Barcode-Symbology`, `X-ITB-Input-Size: 0` and `X-ITB-Output-Size`. There are no HTTP `output`, `force` or `format` fields. It calls the domain `GeneratePlan`, admits output dimensions, pixel count and `WorkingBytes` before rendering, and removes the per-request temporary file after responding. The plan includes the complete fixed-font text with 10 px margins on each side; long compact linear codes may therefore be wider when text is enabled. Admission uses that full width.

Decoding accepts JPEG/PNG/WebP, with JPEG EXIF Orientation normalized and alpha composited onto white. Omitted `symbology` searches all seven formats; e.g. `qr,micro-qr,rmqr` restricts the search. The `itb.barcode.decode.v1` JSON contains `input{path,width,height}` and `codes[]{text,symbology,points}`. `input.path` is the sanitized client filename, never a server path. Points are four pixel coordinates in top-left, top-right, bottom-right, bottom-left order in the normalized image. Multiple results and identical text at different positions are preserved. **No detected codes returns HTTP 200 with `codes: []`.** Invalid formats/parameters fail with 400, unsupported image formats with 415, and unavailable or failed native readers with 500 `internal_error`.

Both endpoints use the existing token authentication, concurrency, timeout and multipart limits. Decode input dimensions and working memory (decoded image, Gray8, protocol and native scratch estimate) are admitted before full decode or pixel allocation; domain limits additionally cap inputs at 64 Mi pixels and 32768 px per dimension. Limit failures return 413 `image_too_large`. The native process follows request cancellation/deadlines.

```bash
curl -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'symbology=qr' -F 'data=https://example.com' \
  -F 'error-correction=H' -F 'module-size=8' \
  "$API/barcode/generate" -o code.png

curl -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@code.png' -F 'symbology=qr,micro-qr,rmqr' \
  "$API/barcode/decode"
```

### Examples

```bash
export ITB_API_TOKEN='replace-with-a-long-random-token'
API=https://itb.example.com/api/v1

curl -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@photo.jpg' \
  -F 'width=1920' \
  -F 'height=1080' \
  -F 'mode=fill' \
  -F 'anchor=center' \
  -F 'filter=lanczos' \
  "$API/resize" -o result.jpg

curl -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@photo.png' \
  -F 'to=webp' \
  -F 'quality=80' \
  "$API/convert" -o photo.webp

curl -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@photo.jpg' \
  -F 'text=Confidential' \
  -F 'mode=repeat' \
  -F 'opacity=0.35' \
  "$API/watermark" -o watermarked.jpg

curl -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@photo.png' \
  -F 'angle=45' \
  "$API/rotate" -o rotated.png

curl -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@photo.jpg' \
  -F 'detail=true' \
  "$API/inspect"
```

## Errors and limits

Errors use one stable JSON shape:

```json
{
  "error": {
    "code": "invalid_argument",
    "message": "quality must be between 1 and 100"
  }
}
```

Codes are `invalid_argument`, `missing_input`, `unsupported_format`, `payload_too_large`, `image_too_large`, `unauthorized`, `busy`, `timeout`, `not_found`, `method_not_allowed`, and `internal_error`. Router-level errors (unknown routes, wrong methods) use the same JSON shape.

The default limits are a 64 MiB multipart request, 50,000,000 pixels, a 16,384 px maximum dimension, a 512 MiB intermediate-canvas working set, two concurrent image operations, and a two-minute operation timeout. `413` indicates request or image limits, `429` indicates all operation slots are busy, and `504` indicates a timeout.

Limits apply to uploaded images and planned output dimensions where applicable: `--max-pixels` / `--max-dimension` gate the input image, the resolved resize target (including `percent` upscales and single-side `fit` outputs), the planned rotate output (arbitrary angles can grow the canvas), and the final output. Uploaded watermark images (`image` on `watermark`) are also subject to the image limits, including the scaled watermark target derived from `scale`.

`--max-working-bytes` bounds the intermediate canvases a single operation may allocate (watermark text mark canvas, repeat tiling/rotation canvases, scaled watermark logos, and the rotate working set — for arbitrary angles an NRGBA source copy plus the rotated output canvas, while orthogonal rotations account for the output canvas only) as a conservative RGBA upper estimate before any allocation happens.

Configure these when starting the service:

```bash
itb serve \
  --addr 127.0.0.1:8080 \
  --max-upload 64MiB \
  --max-pixels 50000000 \
  --max-dimension 16384 \
  --max-concurrent 2 \
  --timeout 2m
```
