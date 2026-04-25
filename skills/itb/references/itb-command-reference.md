# ITB Command Reference

Use `itb` from the current shell path, or `./itb` when the executable is in the current directory.

```bash
itb --help
itb version
```

## Compress

Compress PNG/JPEG. If `-o` is omitted, the input file is overwritten.

```bash
itb compress -i photo.png -o compressed.png
itb compress -i photo.jpg -o compressed.jpg -q 90
```

Flags:

- `-i, --input`: input image path.
- `-o, --output`: output image path; omit only for intentional overwrite.
- `-q, --quality`: quality `1-100`, default `80`.

Pipeline:

- PNG: `pngquant` then `oxipng`.
- JPEG: `djpeg` then `cjpeg` from libjpeg-turbo.

## Crop

Crop by anchor and percentage.

```bash
itb crop -i a.jpg -o left.jpg --anchor left --width 40%
itb crop -i a.jpg -o center.jpg --anchor center --width 40% --height 40%
```

Flags:

- `-i, --input`: input image path.
- `-o, --output`: output path; default adds `_cropped`.
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
itb resize -i photo.jpg -o wide.jpg --width 1200
itb resize -i photo.jpg -o social.jpg --width 1200 --height 630 --mode fit
itb resize -i photo.jpg -o filled.jpg --width 1200 --height 630 --mode fill --anchor top
itb resize -i photo.png -o half.png --percent 50%
```

Flags:

- `-i, --input`: input image path.
- `-o, --output`: output path; default adds `_resized`.
- `--width`, `--height`: target dimensions.
- `--percent`: scale percentage, e.g. `50%`.
- `--mode`: `fit`, `fill`, `stretch`; default `fit`.
- `--anchor`: fill-mode anchor; default `center`.
- `--filter`: `nearest`, `linear`, `catmullrom`, `lanczos`; default `lanczos`.

## Convert

Convert between `jpg`, `jpeg`, `png`, and `webp`.

```bash
itb convert -i photo.png -o photo.webp --to webp -q 85
itb convert -i transparent.png -o flat.jpg --to jpg --background "#FFFFFF"
itb convert -i photo.jpg -o photo.png --to png
```

Flags:

- `-i, --input`: input image path.
- `-o, --output`: output path; default adds `_converted.<ext>`.
- `--to`: required target format: `jpg`, `jpeg`, `png`, `webp`.
- `-q, --quality`: lossy quality; default `80`.
- `--lossless`: lossless encoding for webp/png.
- `--background`: background for transparent-to-opaque conversion; default `#FFFFFF`.

## Watermark

Add text or image watermarks.

Position text watermark:

```bash
itb watermark -i photo.jpg -o marked.jpg -t "Author"
itb watermark -i photo.png -o center.png -t "Copyright" --position center --opacity 0.8
```

Repeated text watermark:

```bash
itb watermark -i photo.png -o draft.png -t "DRAFT" --mode repeat --angle 45 --opacity 0.3
itb watermark -i photo.png -o red.png -t "CONFIDENTIAL" --mode repeat --color "#FF0000"
```

Image watermark:

```bash
itb watermark -i photo.jpg -o logo.jpg --image logo.png --scale 0.2 --position bottom-right
```

Flags:

- `-i, --input`: input image path.
- `-t, --text`: watermark text. Required unless using `--image`.
- `-o, --output`: output path; default adds `_watermarked`.
- `-m, --mode`: `position` or `repeat`; default `position`.
- `--color`: watermark color; empty auto-selects black/white.
- `--opacity`: `0` to `1`; default `0.5`.
- `--font-size`: `0` means auto-size.
- `--font`: font file path; empty uses system font.
- `--image`: image watermark path.
- `--scale`: image watermark scale based on base image short edge; default `0.2`.
- `--tile`: image tiling flag, currently unsupported.
- `--position`: `bottom-right`, `bottom-left`, `top-right`, `top-left`, `center`; default `bottom-right`.
- `--margin`: margin ratio based on short edge; default `0.04`.
- `--angle`: repeat text angle; default `30`.
- `--space`: repeat spacing; `0` auto-calculates.

## Batch

Batch supports `resize`, `convert`, and `watermark`. Outputs preserve relative directory structure and generated names.

```bash
itb batch resize --input-dir ./images --output-dir ./out --recursive --width 1200
itb batch convert --input-dir ./images --output-dir ./out --glob "*.png" --to webp
itb batch watermark --input-dir ./images --output-dir ./out -t "Author"
```

Common flags:

- `--input-dir`: required input directory.
- `--output-dir`: required output directory.
- `--glob`: file match pattern; default `*`.
- `--recursive`: recurse into subdirectories; default `false`.
- `--workers`: concurrency; default `4`.
- `--skip-existing`: skip existing outputs; default `false`.
- `--fail-fast`: stop as soon as practical after an error; default `false`.

Task-specific batch flags mirror the matching single-file command, except output paths are generated automatically.

## S3-Compatible Storage

Supports AWS S3, MinIO, Alibaba OSS, Tencent COS, and other S3-compatible services.

Environment variables:

```bash
S3_ENDPOINT
S3_ACCESS_KEY_ID
S3_SECRET_ACCESS_KEY
S3_REGION
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY
AWS_REGION
```

Common flags:

- `-e, --endpoint`: S3 endpoint URL.
- `-a, --access-key`: access key; prefer environment variables.
- `-s, --secret-key`: secret key; prefer environment variables.
- `-r, --region`: default `us-east-1`.
- `-b, --bucket`: required bucket.
- `--force-path-style`: often required for MinIO.

Examples:

```bash
itb s3 upload -i photo.jpg -b my-bucket -e http://localhost:9000 --force-path-style
itb s3 upload -i photo.jpg -b my-bucket -k images/photo.jpg
itb s3 download -b my-bucket -k images/photo.jpg -o ./photo.jpg
itb s3 list -b my-bucket -p images/ --format json
itb s3 delete -b my-bucket -k images/photo.jpg
itb s3 delete -b my-bucket -k images/photo.jpg -f
```

Use `s3 delete -f` only for explicitly requested non-interactive deletion.

## LskyPro Upload

Environment variables:

```bash
LSKY_URL
LSKY_TOKEN
```

Examples:

```bash
itb lsky upload -i photo.jpg
itb lsky upload -i photo.jpg --url https://img.example.com --token "$LSKY_TOKEN"
itb lsky upload -i photo.jpg --strategy 2
itb lsky upload -i photo.jpg --output json
itb lsky upload -i photo.jpg --output url
```

Flags:

- `-i, --input`: required image path.
- `--url`: LskyPro root or `/api/v1` URL; prefer `LSKY_URL`.
- `--token`: API token; prefer `LSKY_TOKEN`.
- `-s, --strategy`: storage strategy ID; default `0`.
- `-o, --output`: `markdown`, `url`, or `json`; default `markdown`.
