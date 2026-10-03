# 图片处理

只读取当前操作对应的小节。精确参数以选定程序的 `itb <command> --help` 为准。

## 输入与输出

`resize`、`crop`、`rotate`、`convert`、`watermark` 及 `compare` 接受 JPEG/PNG/WebP；GIF/BMP/TIFF 不用于这些操作，也不会将动画 GIF 静默截为首帧。JPEG 的 EXIF 方向会先应用到像素，因此裁剪、缩放和比较使用实际显示方向的逻辑尺寸。

默认保留原图。省略输出路径时，变换命令在文件名中添加 `_compressed`、`_cropped`、`_resized`、`_rotated` 或 `_watermarked`；`convert` 必须显式提供输出路径。已有独立输出可能被替换，执行前检查所选路径；输出不得指向任何输入的同一实际文件。

## 压缩

压缩 PNG/JPEG，默认质量为 `80`，范围 `1–100`：

```bash
itb compress photo.png
itb compress -q 80 photo.jpg compressed.jpg
itb compress --format json photo.png compressed.png
```

仅在明确要求覆盖原图时使用 `itb compress --in-place photo.jpg`；它不能与输出路径同时使用。

PNG 使用 `pngquant → oxipng`，JPEG 使用 `djpeg → cjpeg`。结果先写同目录临时文件，再原子提交；失败不留半成品并保留已有目标。

JSON 契约为 `itb.compress.v1`：`input` / `output` 包含 `path`、`format`、`size`、`sha256`，另有 `quality`、`processor` 和 `elapsed_ms`。处理器名称固定为 `pngquant+oxipng` 或 `djpeg+cjpeg`，摘要对应实际处理内容。

## 裁剪

按锚点和百分比裁剪：

```bash
# 保留左侧 40%
itb crop --anchor left --width 40% photo.jpg left.jpg
# 以中心为锚点，保留宽高各 40%
itb crop --anchor center --width 40% --height 40% photo.jpg center.jpg
```

- 百分比范围为 `(0, 100]`。
- `left` / `right` 只提供 `--width`；`top` / `bottom` 只提供 `--height`。
- `top-left`、`top-right`、`bottom-left`、`bottom-right`、`center` 同时提供宽高。

## 缩放

```bash
itb resize --width 1200 photo.jpg wide.jpg
itb resize --width 1200 --height 630 --mode fit photo.jpg fitted.jpg
itb resize --width 1200 --height 630 --mode fill --anchor top photo.jpg filled.jpg
itb resize --width 1200 --height 630 --mode stretch photo.jpg stretched.jpg
itb resize --percent 50% photo.png half.png
```

| 参数 | 默认值 | 用途 |
|------|--------|------|
| `--width` / `--height` | 未指定 | 目标像素尺寸；仅指定一边时保留比例 |
| `--percent` | 未指定 | 按比例缩放；可用 `200%` 放大，不与宽高同时使用 |
| `--mode` | `fit` | `fit` 等比适配边界；`fill` 等比放大后裁满；`stretch` 拉伸至指定尺寸 |
| `--anchor` | `center` | `fill` 模式的裁剪锚点 |
| `--filter` | `lanczos` | `nearest`、`linear`、`mitchell`、`catmullrom`、`lanczos` |

需要平滑边缘并减弱振铃时选 `mitchell`；通常保留默认 `lanczos`。`fill` 和 `stretch` 需要同时指定宽高。

## 旋转

```bash
itb rotate --angle 90 photo.jpg counterclockwise.jpg
itb rotate --angle -90 photo.jpg clockwise.jpg
itb rotate --angle 45 transparent.png angle45.png
itb rotate --angle 22.5 photo.webp angle.webp
```

`--angle` 必填，支持小数，范围为 `(-360, 360)` 且不能为 `0`。正角度逆时针，负角度顺时针。

精确 `90/180/270` 不做插值；任意角度使用双线性插值，按旋转包围盒规则调整画布，不保证严格完整容纳。未覆盖区域在 PNG/WebP 中透明，在 JPEG 中铺白。JPEG 和有损 WebP 仍会重新编码，不能称为文件级无损旋转。

## 格式转换

```bash
itb convert -q 85 photo.jpg photo.webp
itb convert --lossless transparent.png transparent.webp
itb convert --background "#FFFFFF" transparent.png flat.jpg
itb convert photo.jpg photo.png
```

目标格式仅由 `<dst>` 的 `jpg` / `jpeg` / `png` / `webp` 扩展名确定，没有 `--to` 参数。

- `-q, --quality` 默认为 `80`：JPEG/有损 WebP 使用质量；无损 WebP 使用编码努力程度；PNG 忽略它。
- `--lossless` 启用无损 WebP；PNG 始终无损，该参数对 PNG 不产生额外效果。
- `--background` 默认为 `#FFFFFF`，只用于转 JPEG 时给透明区域铺不透明底色；PNG/WebP 保留透明度。

## 水印

文字与图片水印二选一。中文文字需要覆盖相应字形的字体，示例中的字体路径替换为本机实际文件：

```bash
# 单点文字水印
itb watermark -t "版权声明" --font /path/to/chinese-font.ttf photo.jpg marked.jpg
# 重复文字水印
itb watermark -t "草稿" --font /path/to/chinese-font.ttf \
  --mode repeat --angle 45 --opacity 0.3 photo.png draft.png
# 图片水印
itb watermark --image logo.png --scale 0.2 --position bottom-right photo.jpg marked.jpg
```

| 参数 | 默认值 | 用途 |
|------|--------|------|
| `-t, --text` / `--image` | 未指定 | 文字内容或水印图片路径，必须且只能选一种 |
| `-m, --mode` | `position` | `position` 单点放置；`repeat` 仅支持文字平铺 |
| `--color` | 自动 | 自动选黑白；可用 `#RGB` / `#RRGGBB` / `#RRGGBBAA` |
| `--opacity` | `0.5` | 透明度，范围 `[0, 1]` |
| `--font` | 自动 | 文字字体文件 |
| `--font-size` | `0` | `0` 自动，最大 `4096` 像素 |
| `--scale` | `0.2` | 图片水印尺寸相对于原图短边的比例 |
| `--position` | `bottom-right` | `bottom-right` / `bottom-left` / `top-right` / `top-left` / `center` |
| `--margin` | `0.04` | 单点模式边距相对于短边的比例，须非负 |
| `--angle` | `30` | 重复文字角度，范围 `[-360, 360]` |
| `--space` | `0` | 重复文字间距，`0` 自动 |

图片水印仅支持 `position` 模式，输出也不能与 `--image` 指向同一实际文件。

## 检查图片与哈希

```bash
itb inspect photo.jpg
itb inspect --format json photo.jpg
itb inspect --format plain photo.jpg
itb inspect --no-hash photo.jpg
itb inspect --hash sha256 --hash crc32 --no-detail --format json photo.jpg
itb inspect --strict --full-decode --format json photo.png
```

`inspect` 是只读检查。格式来自文件内容，能识别 PNG/JPEG/GIF/WebP/BMP/TIFF/SVG；前六种可解码，SVG 只识别和校验结构，不作栅格解码，没有显式宽高的合法 SVG 也可识别。

- `--format` 支持 `table`（默认）、`json`、`plain`；`plain` 只输出 SHA-256。
- `--no-detail` 跳过详细信息，`--no-hash` 跳过哈希。
- `--hash` 可重复选择 `sha256` / `sha1` / `md5` / `crc32`；默认计算全部，不与 `--no-hash` 同用。单遍读取后检查可观察变化，变化时报 `E_SOURCE_CHANGED`。
- `--strict` 让解析错误变为命令失败；SVG 会完整校验 XML 结构。
- `--full-decode` 检查完整图片内容，GIF 逐帧解码，可发现头部正常而尾部损坏的文件。与 `--strict` 组合用于上传前检查。

JSON 契约为 `itb.inspect.v3`。`content` 包含 `format`、`canonical_extension`、`mime_type`、`recognized`、`decode_supported`、`full_decode_supported`、`extension_matches`；另有 `decode_config_ok`、`full_decode_ok`、`frame_count`、`animation_known`、`animated`。完整解码状态有成功、失败和未执行三种情形；不要把合法 SVG 的 `decode_supported=false` 当成损坏。

## 质量比较

```bash
itb compare original.jpg compressed.jpg
itb compare original.jpg compressed.jpg --ssim
itb compare original.png compressed.webp --psnr --ssim --ms-ssim
itb compare photo.jpg photo.jpg --psnr
```

两个路径均为只读输入，同文件自我比较合法。两图的逻辑尺寸必须一致，不隐式缩放、裁剪或补边。

- 无指标参数时计算 PSNR + MS-SSIM；出现任一指标参数（包括 `=false`）后，只计算显式启用的指标。全部关闭会报错。
- PSNR 以 dB 表示，相同有效通道输出 `+Inf`。
- SSIM 固定 11×11 高斯有效窗口，sigma=1.5、K1=0.01、K2=0.03、L=255，不自动降采样；宽高均至少 `11`。
- MS-SSIM 固定五尺度，权重为 `{0.0448, 0.2856, 0.3001, 0.2363, 0.1333}`，尺度间用 2×2 均值、尺寸向上半除；短边至少 `161`，不因小图减少尺度。小图可显式选 `--psnr` 或符合尺寸要求的 `--ssim`。
- 任一图存在非全不透明像素时，比较预乘 R/G/B 和 A；这是 itb 自定义透明度感知变体，不应期望与只比较 RGB 的工具完全一致。

结果按固定顺序输出文本，小数保留六位；当前没有 `--format json`，也不提供 HTTP 比较接口。

## 常见组合流程

将横图制成适配边界的 WebP，再检查输出；输入路径和尺寸按任务替换：

```bash
mkdir -p output
itb resize --width 1200 --height 630 --mode fit photo.jpg output/resized.jpg
itb convert -q 85 output/resized.jpg output/final.webp
itb inspect --strict --full-decode --format json output/final.webp
```

多步重新编码可能引入质量损失；无需改尺寸或格式时直接压缩原图。发布到对象存储时，再读取 [S3 兼容存储](s3.md)。
