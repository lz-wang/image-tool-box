# ITB Command Reference

Use `itb` from the current shell path, or `./itb` when the executable is in the current directory.

```bash
itb --help
itb --version
itb version
```

## Compress

Compress PNG/JPEG. By default the input is kept and the result is written next to it as `<name>_compressed.<ext>`; pass `--in-place` to overwrite the input.

```bash
itb compress photo.png
itb compress photo.png compressed.png
itb compress -q 90 photo.jpg compressed.jpg
itb compress --in-place photo.jpg
itb compress --format json photo.png
```

Flags:

- `<src>`: required input image path.
- `[dst]`: optional output image path; default adds `_compressed`.
- `--in-place`: overwrite the input file; cannot be used with `[dst]`.
- `-q, --quality`: quality `1-100`, default `80`.
- `--format`: `table` (default) or `json` — the `itb.compress.v1` contract with
  `input`/`output` `{path, format, size, sha256}`, `quality`, `processor`
  (`pngquant+oxipng` or `djpeg+cjpeg`), and `elapsed_ms`.

Output is staged to a temp file and atomically committed to the destination: a
failed run never leaves a partial file, and an existing destination stays
untouched.

Pipeline:

- PNG: `pngquant` then `oxipng`.
- JPEG: `djpeg` then `cjpeg` from libjpeg-turbo.

## Crop

Crop by anchor and percentage.

```bash
itb crop --anchor left --width 40% a.jpg left.jpg
itb crop --anchor center --width 40% --height 40% a.jpg center.jpg
```

Flags:

- `<src>`: required input image path.
- `[dst]`: optional output path; default adds `_cropped`.
- `--anchor`: `left`, `right`, `top`, `bottom`, `top-left`, `top-right`, `bottom-left`, `bottom-right`, `center`.
- `--width`: crop width percentage, e.g. `40%`.
- `--height`: crop height percentage, e.g. `40%`.

Rules:

- Percentages must be `(0, 100]`.
- `left` / `right` require `--width` only.
- `top` / `bottom` require `--height` only.
- Corner and `center` anchors require both `--width` and `--height`.

## Resize

Resize by width, height, bounding box, fill crop, stretch, or percent.

```bash
itb resize --width 1200 photo.jpg wide.jpg
itb resize --width 1200 --height 630 --mode fit photo.jpg social.jpg
itb resize --width 1200 --height 630 --mode fill --anchor top photo.jpg filled.jpg
itb resize --percent 50% photo.png half.png
```

Flags:

- `<src>`: required input image path.
- `[dst]`: optional output path; default adds `_resized`.
- `--width`, `--height`: target dimensions.
- `--percent`: exact scale percentage, e.g. `50%`; upscaling (`200%`) works.
- `--mode`: `fit`, `fill`, `stretch`; default `fit`.
- `--anchor`: fill-mode anchor; default `center`.
- `--filter`: `nearest`, `linear`, `mitchell`, `catmullrom`, `lanczos`; default `lanczos`. Use `mitchell` when smoother edges and reduced ringing are preferred; use the default `lanczos` when maximum detail retention is preferred.

## Rotate

Rotate by any angle. Positive angles rotate counter-clockwise, negative clockwise; exact `90/180/270` are interpolation-free, arbitrary angles use bilinear interpolation and adjust the canvas according to the rotated bounding box (uncovered areas stay transparent for PNG/WebP, flatten onto white for JPEG).

```bash
itb rotate --angle 90 photo.jpg                       # CCW 90° (writes photo_rotated.jpg)
itb rotate --angle -90 photo.jpg clockwise.jpg        # CW 90°
itb rotate --angle 45 transparent.png angle45.png     # arbitrary angle, transparent corners
itb rotate --angle 22.5 photo.webp angle.webp         # fractional angle
```

Flags:

- `<src>`: required input image path (`jpg`/`jpeg`/`png`/`webp` only).
- `[dst]`: optional output path; default adds `_rotated`.
- `--angle`: required signed angle in degrees; range `(-360, 360)`, never `0`; fractional values allowed.

Semantics:

- JPEG EXIF orientation is normalized before the user rotation, so the angle applies to the visual (logical) pixels.
- `<dst>` must not resolve to the same file as `<src>` (equivalent paths, hard links, symlinks rejected).
- Also exposed over HTTP as `POST /api/v1/rotate` (`input` + floating-point `angle`).

## Convert

Convert between `jpg`, `jpeg`, `png`, and `webp`. Inputs are limited to `jpg`/`jpeg`/`png`/`webp` (GIF/BMP/TIFF are rejected); the EXIF orientation of JPEG inputs is applied to the pixels during conversion.

This input-format and orientation contract is shared by every transform (`convert`/`resize`/`crop`/`rotate`/`watermark`): they all open inputs through the same static-image entry point, so GIFs never get silently reduced to their first frame and percent-based crop/resize plans always operate on the rotated (logical) dimensions.

```bash
itb convert -q 85 photo.png photo.webp
itb convert transparent.png flat.jpg --background "#FFFFFF"
itb convert photo.jpg photo.png
```

Flags:

- `<src>`: required input image path (`jpg`/`jpeg`/`png`/`webp` only).
- `<dst>`: required output path; its `jpg`/`jpeg`/`png`/`webp` extension determines the target format.
- `-q, --quality`: JPEG/WebP quality; default `80`. Compression effort in lossless WebP mode; ignored for PNG.
- `--lossless`: lossless WebP encoding; PNG is always lossless, so this flag is a no-op for PNG.
- `--background`: opaque background for transparent areas when converting to JPEG; default `#FFFFFF`. Ignored for PNG/WebP.

## Watermark

Add text or image watermarks.

Position text watermark:

```bash
itb watermark -t "Author" photo.jpg marked.jpg
itb watermark -t "Copyright" --position center --opacity 0.8 photo.png center.png
```

Repeated text watermark:

```bash
itb watermark -t "DRAFT" --mode repeat --angle 45 --opacity 0.3 photo.png draft.png
itb watermark -t "CONFIDENTIAL" --mode repeat --color "#FF0000" photo.png red.png
```

Image watermark:

```bash
itb watermark --image logo.png --scale 0.2 --position bottom-right photo.jpg logo.jpg
```

Flags:

- `<src>`: required input image path.
- `-t, --text`: watermark text. Required unless using `--image`.
- `[dst]`: optional output path; default adds `_watermarked`.
- `-m, --mode`: `position` or `repeat`; default `position`.
- `--color`: watermark hex color (`#RGB`/`#RRGGBB`/`#RRGGBBAA`); empty auto-selects black/white.
- `--opacity`: `0` to `1`; default `0.5`.
- `--font-size`: `0` means auto-size; max `4096`.
- `--font`: font file path; empty auto-selects an available default font.
- `--image`: image watermark path.
- `--scale`: image watermark scale based on base image short edge; default `0.2`.
- `--position`: `bottom-right`, `bottom-left`, `top-right`, `top-left`, `center`; default `bottom-right`.
- `--margin`: margin ratio based on short edge; default `0.04`; must be `>= 0`.
- `--angle`: repeat text angle; default `30`; range `-360` to `360`.
- `--space`: repeat spacing; `0` auto-calculates.

## Compare

Read-only objective image-quality comparison (`PSNR` / `SSIM` / `MS-SSIM`), pure Go, CLI-only (not exposed via the HTTP API).

```bash
itb compare original.jpg compressed.jpg                    # default: PSNR + MS-SSIM
itb compare original.jpg compressed.jpg --ssim             # SSIM only
itb compare original.png original.webp --psnr --ssim --ms-ssim
itb compare photo.jpg photo.jpg                            # sanity check: PSNR: +Inf dB
```

Flags and operands:

- `<src>`: required reference image path.
- `<dst>`: required comparison-target image path (read-only input, not an output file).
- `--psnr`: compute PSNR in dB (`+Inf` when the active channels are identical).
- `--ssim`: compute SSIM (standard parameters: 11×11 Gaussian window, sigma 1.5, K1=0.01, K2=0.03, L=255; no automatic downsampling).
- `--ms-ssim`: compute MS-SSIM (fixed five scales with Wang weights `{0.0448, 0.2856, 0.3001, 0.2363, 0.1333}`; 2×2 average downsampling between scales).

Metric selection semantics:

- No metric flag given → PSNR + MS-SSIM.
- Any metric flag given (including `=false`) → only the explicitly enabled metrics run; `--psnr=false` alone is an error (`at least one metric must be selected`).

Requirements and semantics:

- Supported formats: JPEG / PNG / WebP (GIF/BMP/TIFF rejected); JPEG EXIF orientation is baked in, so the metrics compare the actual visual pixels.
- Both images must have identical logical dimensions (`1920x1080` vs `1280x720` fails with an error; never implicitly resized, cropped, or padded).
- SSIM requires both dimensions ≥ 11; MS-SSIM requires the short edge ≥ 161 (five fixed scales; the scale count is never reduced for smaller images — use `--psnr` or `--ssim` instead).
- Alpha handling: when either image has `alpha != 255`, the compared channels become premultiplied R/G/B plus A (fully transparent regions hide their RGB, and alpha loss is still detected). This is an itb-defined alpha-aware variant; do not expect bit-identical values with RGB-only third-party tools.
- Output is fixed-order plain text (`%.6f`), independent of flag order; identical images print `PSNR: +Inf dB`, `SSIM: 1.000000`, `MS-SSIM: 1.000000`. There is intentionally no `--format json` yet.

## Inspect

Read-only file/image inspection with hashes; JSON contract `itb.inspect.v3`.

Content recognition: the format comes from the file itself (magic bytes plus
streamed XML for SVG), never from the extension. PNG, JPEG, GIF, WebP, BMP, and
TIFF are raster-decodable; SVG is recognized (`recognized=true`) but never
raster-decoded (`decode_supported=false`) — not an error, and SVGs without
explicit width/height are valid. The `content` object reports `format`,
`canonical_extension`, `mime_type`, `recognized`, `decode_supported`,
`full_decode_supported`, and `extension_matches`.

```bash
itb inspect photo.jpg
itb inspect --format json photo.jpg
itb inspect --format plain photo.jpg   # sha256 only
itb inspect --no-hash photo.jpg
itb inspect --hash sha256 --hash crc32 --no-detail --format json photo.jpg
itb inspect --strict --full-decode --format json image.png   # upload preflight
```

Flags:

- `<src>`: required input image path.
- `--format`: `table` (default) / `json` / `plain`.
- `--no-detail` / `--no-hash`: skip detail or hash computation.
- `--hash ALGO`: compute only the given algorithm (repeatable:
  `sha256`/`sha1`/`md5`/`crc32`); unselected algorithms are omitted from JSON.
  Without `--hash`, all algorithms are computed. Mutually exclusive with
  `--no-hash`. Hashing is a single streaming pass with post-read change
  detection (`E_SOURCE_CHANGED` if the file observably changed mid-read).
- `--strict`: return an error instead of an `error` object when parsing fails.
  Recognized-as-SVG files additionally get a full XML structure validation
  (parse to EOF, single root, no dangling content); a truncated or malformed
  SVG reports `error.structure_invalid` (or fails under `--strict`).
- `--full-decode`: fully decode the image (frame-by-frame for GIF). Detects
  files whose header is fine but tail is corrupted; adds `full_decode_ok`
  (tri-state), `frame_count` (GIF), `animation_known`, and `animated`.
  Combine with `--strict` as a preflight before uploading.

## S3-Compatible Storage

Supports AWS S3, MinIO, Alibaba OSS, Tencent COS, and other S3-compatible services.

Environment variables:

```bash
ITB_S3_ENDPOINT
ITB_S3_ACCESS_KEY_ID
ITB_S3_SECRET_ACCESS_KEY
ITB_S3_SESSION_TOKEN
ITB_S3_REGION
ITB_S3_BUCKET
ITB_S3_FORCE_PATH_STYLE
```

Priority: CLI flag > `ITB_S3_*` environment variables > defaults. Environment variables satisfy required flags such as `--bucket`.

Common flags:

- `-e, --endpoint`: required S3 endpoint URL (or `ITB_S3_ENDPOINT`).
- `-a, --access-key`: required access key; prefer the `ITB_S3_ACCESS_KEY_ID` environment variable.
- `-s, --secret-key`: required secret key; prefer the `ITB_S3_SECRET_ACCESS_KEY` environment variable.
- `--session-token`: session token for temporary credentials; prefer `ITB_S3_SESSION_TOKEN` so the token stays out of shell history.
- `-r, --region`: default `us-east-1`.
- `-b, --bucket`: required bucket (or `ITB_S3_BUCKET`).
- `--force-path-style`: often required for MinIO (or `ITB_S3_FORCE_PATH_STYLE`; auto-enabled for loopback / `:9000` endpoints).
- `--max-attempts`: max S3 API attempts per operation (default 3, AWS SDK standard retryer).
- `--connect-timeout` / `--response-header-timeout`: transport timeouts, default `30s` each; body transfer is never time-limited by default.
- `--operation-timeout`: total limit for the whole operation; default `0` (disabled) because an explicit limit can interrupt large transfers.

Examples:

```bash
itb s3 upload -b my-bucket -e http://localhost:9000 --force-path-style photo.jpg
itb s3 upload -b my-bucket photo.jpg images/photo.jpg
itb s3 upload -b my-bucket --cache-control no-cache image.webp image/xx.webp \
  --metadata source-sha256=abc123 --metadata width=1920
itb s3 upload -b my-bucket --skip-existing photo.jpg
itb s3 upload -b my-bucket --skip-unchanged photo.jpg
itb s3 upload -b my-bucket --skip-matching photo.jpg --metadata source-sha256=abc123
itb s3 upload -b my-bucket --verify photo.jpg
itb s3 download -b my-bucket images/photo.jpg ./photo.jpg
itb s3 download -b my-bucket images/photo.jpg   # saves ./photo.jpg (last key segment)
itb s3 download -b my-bucket --verify-sha256 "$SOURCE_SHA256" sha256/xxx /tmp/original.png
itb s3 list -b my-bucket --format json images/
itb s3 list -b my-bucket images/ --all --format json
itb s3 list -b my-bucket images/ --page-size 500 --limit 5000 --format json
itb s3 list -b my-bucket images/ --continuation-token TOKEN --format json
itb s3 stat -b my-bucket images/photo.jpg
itb s3 stat -b my-bucket --format json images/photo.jpg
itb s3 delete -b my-bucket images/photo.jpg
itb s3 delete -b my-bucket -f images/photo.jpg
```

Use `s3 delete -f` only for explicitly requested non-interactive deletion.

S3 object/file operands are positional: `upload <src> [key]`, `download <key> [dst]`, `stat/delete <key>`, and `list [prefix]`. Connection settings and processing behavior remain flags or `ITB_S3_*` environment variables.

`s3 stat` shows full metadata of one object with a single HEAD request (no
content transfer) and never falls back to list inference; prefer it over
`s3 list`/`s3 download` when only checking whether an object exists or
inspecting its metadata.

`s3 upload` stores the file's SHA-256 in `x-amz-meta-itb-sha256` metadata by
default. `--skip-existing` skips when the key exists; `--skip-unchanged`
skips only when that metadata hash matches the local file (never rely on
ETag for this); `--skip-matching` skips (JSON `status=reused`) only when the
remote object's complete state matches: sha256, size, Content-Type, plus
every explicitly requested header/metadata (requested subset matching —
extra remote metadata is irrelevant, unspecified headers mean "don't care").
Default upload always overwrites. The three skip flags are mutually
exclusive — combining them is a flag error. The `itb.s3.upload.v2` JSON adds
`status`: `uploaded` / `skipped` / `reused` (legacy `skipped`/`reason` kept).

Uploads read the source through a private stable snapshot: the stored
`itb-sha256` always matches the actual PUT body, retries re-read identical
bytes, and a source that observably changes mid-snapshot fails with
`E_SOURCE_CHANGED` instead of uploading inconsistent data.

`--metadata key=value` (repeatable) attaches user metadata: keys are
lowercased, must be non-empty, may not contain control characters, and
`itb-sha256` is reserved (rejected before any network request).
`--cache-control`, `--content-disposition`, and `--content-encoding` set the
object's standard HTTP response headers — use `--cache-control no-cache` when
publishing images under stable URLs.

Without `--content-type`, the MIME type is detected from file content (magic
sniffing for JPEG/PNG/GIF/WebP/PDF/ZIP/HTML/JSON/SVG); the extension is only
a fallback. An HTML error page renamed to `.jpg` uploads as `text/html`, so
HTML/XML error bodies never masquerade as images.

`--verify` follows the PUT with one HEAD and checks that remote
size/Content-Type/HTTP headers/metadata match the upload (request contract:
`PUT → HEAD`; with skip flags a hit stays a single `HEAD`, a miss becomes
`HEAD → PUT → HEAD`). It proves header/metadata consistency only — body
integrity is verified on download, not here.

`s3 download --verify` compares the SHA-256 computed while streaming against
the object's `itb-sha256` metadata; `--verify-sha256 HASH` compares against a
known hash (both single-pass). `--expect-size N` and `--expect-content-type
MIME` are checked against response headers before the target is created and
against actual bytes afterwards (`E_TARGET_CONFLICT` on mismatch; 0-byte
objects are valid, so an unset size is distinct from `--expect-size 0`).
`--if-exists verify` reuses a local copy only when its size/SHA-256 provably
matches a provided basis (`status=reused`) — with no basis it fails
immediately; "the file exists" is never sufficient. Reuse never weakens
verification: `--verify` and `--expect-content-type` are enforced on the reuse
path too (one HEAD against the remote object); only `--verify-sha256` (plus
optional `--expect-size`) reuses with zero network access. Any failure —
including checksum mismatch — leaves no partial file at the output path
(temp file + rename). The `itb.s3.download.v2` JSON adds `status` and
`content_type`.

`s3 upload --if-exists verify` performs a true immutable conditional upload
(`IfNoneMatch="*"`): it writes only when the key is absent; if the key exists,
the remote state is matched against this upload (`status=reused`) or the
command fails with `E_TARGET_CONFLICT`. A provider without conditional-write
support fails with `E_UNSUPPORTED_CAPABILITY` — it never degrades to an
unsafe HEAD + PUT.

`upload`, `download`, and `stat` all accept `--format table|json`. JSON output
carries a `schema_version` contract (`itb.s3.upload.v2`, `itb.s3.download.v2`,
`itb.s3.stat.v1`); scripts should branch on it rather than parsing terminal
text. stdout carries results only — progress and diagnostics go to stderr.

Machine-readable failures: when a command invoked with `--format json` fails,
stdout carries exactly one `itb.error.v1` document
(`schema_version` / `operation` / `error{code,message,retryable,http_status,provider_code}`)
and nothing is duplicated on stderr. Branch on the stable `E_*` code
(`E_INVALID_ARGUMENT`, `E_OBJECT_NOT_FOUND`, `E_ACCESS_DENIED`,
`E_CHECKSUM_MISMATCH`, `E_INCOMPLETE_LIST`, `E_TIMEOUT`, `E_NETWORK`,
`E_THROTTLED`, ...); `retryable` says whether retrying the same operation can
succeed. S3 provider errors expose only HTTP status and provider code —
credentials and signed URLs never appear in this output.

`s3 list` requests a single page by default (`--page-size`, 1-1000, alias
`--max-keys`). Pass `--all` to paginate until complete; `--limit N` stops
after N objects. JSON output is the structured `itb.s3.list.v2` contract: an
empty listing still reports `"complete": true` with `"objects": []`, while a
truncated listing sets `"complete": false` and returns
`next_continuation_token` that can be fed back via `--continuation-token`.
`complete=true` only means the traversal from this run's starting token
finished normally. If the server reports more objects without a usable
continuation token, the command fails with `E_INCOMPLETE_LIST` instead of
returning a partial listing.

## Serve (HTTP API)

Starts the trusted HTTP API. It directly calls the same Go domain packages used by the CLI and does not spawn CLI subprocesses. There is no built-in WebUI.

Examples:

```bash
export ITB_API_TOKEN='replace-with-a-strong-token'
itb serve
itb serve --addr 127.0.0.1:9000
curl http://127.0.0.1:8080/api/v1/health

# For loopback local development only
itb serve --no-auth
```

Flags:

- `--addr`: listen address; default `127.0.0.1:8080`. Keep it on loopback unless the network is trusted.
- `--max-upload`: maximum multipart request size.
- `--max-pixels`: maximum decoded image pixel count.
- `--max-dimension`: maximum image width or height.
- `--max-concurrent`: maximum concurrent image operations.
- `--max-working-bytes`: per-operation working-memory limit.
- `--timeout`: per-operation timeout.
- `--no-auth`: disable Bearer-token authentication for loopback-only local development.

Authentication:

- Set `ITB_API_TOKEN` for normal operation.
- Image-processing requests use `Authorization: Bearer $ITB_API_TOKEN`.
- `/api/v1/health` remains unauthenticated.

Feature scope: `compress`, `resize`, `crop`, `rotate`, `convert`, `watermark`, and `inspect`. The HTTP API intentionally does not expose S3 management, a WebUI, workflows, user management, databases, or task queues.

Security notes:

- The API is stateless: uploads land in per-request temp directories that are removed after the response.
- The API only performs local image processing and never handles external-service credentials.
