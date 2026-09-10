# GO 图像工具箱

[![codecov](https://codecov.io/gh/lz-wang/image-tool-box/graph/badge.svg?token=UW9vZvWwxY)](https://codecov.io/gh/lz-wang/image-tool-box)

> **English**: [README.en.md](./README.en.md)

> 外部依赖说明见 [docs/build-bins.md](docs/build-bins.md)。
> 
> CI 当前会并行构建以下平台：
> 
> - macOS amd64 / arm64
> - Linux amd64 / arm64
> - Windows amd64 / arm64
> 
> Release 产物中，macOS / Linux 使用 `.tar.gz`，Windows 使用 `.zip`；Windows 可执行文件和内置压缩工具均带 `.exe` 扩展名。

## Linux 兼容性

官方 Linux amd64 / arm64 构建的 `compress` 功能要求 **glibc >= 2.28**；Go 实现的其他功能不在启动时强制检查此条件。Alpine Linux / musl 当前不受支持。

| 系统 | `compress` 支持情况 |
|------|---------------------|
| CentOS 7（glibc 2.17） | ❌ |
| Ubuntu 18.04（glibc 2.27） | ❌ |
| Rocky / AlmaLinux 8（glibc 2.28） | ✅ |
| Debian 10+ | ✅ |
| Ubuntu 20.04+ | ✅ |

CI 使用 `manylinux_2_28` 构建 Linux 内置压缩器，并校验其最高 `GLIBC_*` 符号版本及动态库依赖。发行包只包含单个 `itb` 文件；压缩器仍内嵌在其中，并在首次使用时按需解压到用户缓存目录。

## 安装

新版本发布后，可通过 Homebrew（macOS / Linux）安装：

```bash
brew tap lz-wang/tap
brew install lz-wang/tap/itb
```

安装完成后可运行 `itb --version`（或 `itb version`）验证。

> [!WARNING]
> **macOS 运行提示**
>
> 如果在 macOS 上运行二进制时提示“无法验证开发者”，并且每次都需要到“安全性与隐私”里手动放行，内部使用场景下可以在下载或解压后先移除 `quarantine` 标记：
>
> ```bash
> xattr -d com.apple.quarantine your_binary
> ```

> **文件安全**：显式提供 `<dst>` 时，输出不得与任何输入资源指向同一实际文件（包括等价路径、hard link 和 symlink）。`resize`、`crop`、`rotate`、`watermark` 等命令可省略 `[dst]` 以使用默认派生输出路径；`convert` 必须显式提供 `<dst>`。原地压缩请使用 `compress --in-place`。

## 压缩图片

自动检测图片格式（PNG/JPEG）并压缩，默认保留原文件：

```bash
# 压缩 PNG 图片（输出 photo_compressed.png）
./itb compress photo.png

# 压缩 JPEG 图片（输出 photo_compressed.jpg）
./itb compress photo.jpg

# 指定输出文件
./itb compress photo.png compressed.png

# 覆盖原文件（不能同时提供输出路径）
./itb compress --in-place photo.jpg

# 指定压缩质量（1-100，默认 80）
./itb compress -q 90 photo.jpg

# 机器可读 JSON 输出（契约 itb.compress.v1）
./itb compress -q 80 source.png output.png --format json
```

压缩输出采用**安全提交**：结果先写入目标目录下的临时文件，成功并校验
（非空、格式正确）后原子 rename 到目标路径。任何失败都会清理临时文件，
已存在的目标保持原状，目标路径不会出现 partial 内容。

`--format json` 输出契约 `itb.compress.v1`：

```json
{
  "schema_version": "itb.compress.v1",
  "input":  {"path": "source.png", "format": "png", "size": 120000, "sha256": "..."},
  "output": {"path": "output.png", "format": "png", "size": 80000, "sha256": "..."},
  "quality": 80,
  "processor": "pngquant+oxipng",
  "elapsed_ms": 123
}
```

`processor` 固定为 `pngquant+oxipng`（PNG）或 `djpeg+cjpeg`（JPEG）；
输入/输出的 `sha256` 由共享的 `internal/filehash` 单遍流式计算，输入侧
附带可观察变化检测（读取期间源文件被修改时以 `E_SOURCE_CHANGED` 失败）。

<details>
<summary>命令参数与压缩管道</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<src>` | (必填) | 输入图片文件路径 |
| `[dst]` | `*_compressed.*` | 输出路径，省略时在原文件名后加 `_compressed` |
| `--in-place` | `false` | 覆盖输入文件（不能同时提供 `[dst]`） |
| `-q, --quality` | `80` | 压缩质量 1-100 |
| `--format` | `table` | 输出格式：`table` / `json`（JSON 契约 `itb.compress.v1`） |

**压缩管道**

- **PNG**: `pngquant` → `oxipng`（有损 + 无损双重压缩）
- **JPEG**: `djpeg` → `cjpeg`（libjpeg-turbo 解码 + 编码）

</details>

## 图像裁剪

按锚点和百分比保留图片区域。

```bash
# 保留左侧 40% 宽度
./itb crop --anchor left --width 40% a.jpg

# 保留右侧 40% 宽度
./itb crop --anchor right --width 40% a.jpg

# 保留左上角 40% x 40% 区域
./itb crop --anchor top-left --width 40% --height 40% a.jpg

# 保留中心 40% x 40% 区域
./itb crop --anchor center --width 40% --height 40% a.jpg
```

<details>
<summary>命令参数与规则</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<src>` | (必填) | 输入图片路径 |
| `[dst]` | `*_cropped.*` | 输出路径，省略时在原文件名后加 `_cropped` |
| `--anchor` | (必填) | 裁剪锚点：`left` / `right` / `top` / `bottom` / `top-left` / `top-right` / `bottom-left` / `bottom-right` / `center` |
| `--width` | | 裁剪宽度百分比，例如 `40%` |
| `--height` | | 裁剪高度百分比，例如 `40%` |

**参数规则**

- 仅支持百分比格式，范围为 `(0, 100]`
- `left` / `right` 必须提供 `--width`，且不能提供 `--height`
- `top` / `bottom` 必须提供 `--height`，且不能提供 `--width`
- `top-left` / `top-right` / `bottom-left` / `bottom-right` / `center` 必须同时提供 `--width` 和 `--height`

</details>

## 图像缩放

支持按宽高、百分比和不同模式调整图片尺寸。

```bash
# 指定宽度，按比例缩放
./itb resize --width 1200 photo.jpg

# 指定宽高框，保持比例适配
./itb resize --width 1200 --height 630 --mode fit photo.jpg

# 指定宽高框并裁切填满
./itb resize --width 1200 --height 630 --mode fill --anchor top photo.jpg

# 按百分比缩放
./itb resize --percent 50% photo.png
```

<details>
<summary>命令参数</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<src>` | (必填) | 输入图片路径 |
| `[dst]` | `*_resized.*` | 输出路径 |
| `--width` | | 目标宽度（像素） |
| `--height` | | 目标高度（像素） |
| `--percent` | | 按百分比精确缩放，例如 `50%`；支持放大如 `200%` |
| `--mode` | `fit` | 缩放模式：`fit` / `fill` / `stretch` |
| `--anchor` | `center` | `fill` 模式的锚点 |
| `--filter` | `lanczos` | 采样器：`nearest` / `linear` / `mitchell` / `catmullrom` / `lanczos` |

**参数规则**

- 必须指定 `--percent`，或至少指定 `--width` / `--height` 之一
- `--percent` 不能与 `--width` / `--height` 同时使用
- `fit` 支持仅指定宽度或高度，并保持宽高比
- `fill` 必须同时指定宽度和高度
- `stretch` 同时指定宽高时不保持原始宽高比

**采样器选择**

- `nearest`：最快，不做抗锯齿，适合像素画、mask 等离散像素图像
- `linear`：双线性插值，速度快，输出较平滑
- `mitchell`：Mitchell-Netravali cubic filter，输出较平滑，相比 Catmull-Rom 更少出现 ringing
- `catmullrom`：锐利的 cubic filter，在质量与性能之间取得平衡
- `lanczos`：默认值，适合照片等需要高细节保持的高质量缩放

</details>

## 图片旋转

按任意角度旋转图片：正角度逆时针、负角度顺时针；精确 `90/180/270` 不做插值，任意角度使用双线性插值并按 imaging 的旋转包围盒规则调整输出画布，避免常规角度下裁掉主体内容。

```bash
# 逆时针 90 度（输出 photo_rotated.jpg）
./itb rotate --angle 90 photo.jpg

# 顺时针 90 度
./itb rotate --angle -90 photo.jpg clockwise.jpg

# 任意角度（画布按需调整，PNG 保留透明）
./itb rotate --angle 45 transparent.png result.png

# 小数角度
./itb rotate --angle 22.5 photo.webp rotated.webp
```

<details>
<summary>命令参数</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<src>` | (必填) | 输入图片路径（仅 `jpg` / `jpeg` / `png` / `webp`） |
| `[dst]` | `*_rotated.*` | 输出路径，省略时在原文件名后加 `_rotated` |
| `--angle` | (必填) | 旋转角度（度）：正数逆时针、负数顺时针；支持小数，范围 `(-360, 360)` 且不能为 `0` |

</details>

旋转语义：

- 输入遵循统一的 transform 契约：仅 JPEG/PNG/WebP，JPEG 的 EXIF Orientation 先归一化，再执行本次旋转
- 任意角度按 imaging 的旋转包围盒规则调整输出画布，避免常规角度下裁掉主体内容；未覆盖区域 PNG/WebP 保持透明，JPEG 铺白色背景
- `<dst>` 不得与 `<src>` 指向同一实际文件（等价路径、hard link、symlink 均拒绝）；HTTP API 同样暴露 `rotate`

## 图像格式转换

支持 `jpg/jpeg/png/webp` 互转，输出格式完全由必填 `<dst>` 的扩展名指定。输入仅接受 `jpg/jpeg/png/webp`；转换时 **JPEG** 的 EXIF Orientation 会应用到实际像素（WebP 携带的 orientation 元数据当前不处理），输出不保留 EXIF/GPS/XMP 等 metadata。

**输入格式与 Orientation 统一契约**（适用于 `convert` / `resize` / `crop` / `rotate` / `watermark`）：
所有变换命令的输入都严格限定 `JPEG/PNG/WebP`（GIF/BMP/TIFF 一律拒绝，杜绝 animated GIF 被静默处理首帧），并且 **JPEG EXIF Orientation 一律烘焙进像素**——`resize`/`crop`/`rotate`/`watermark` 的计划推导与资源准入基于应用旋转后的逻辑尺寸，与最终输出一致。

```bash
# 转为 WebP
./itb convert photo.png photo.webp

# 透明 PNG 转 JPG，指定铺底颜色
./itb convert photo.png photo.jpg --background "#FFFFFF"

# 指定输出路径
./itb convert photo.jpg output.png
```

转换语义按目标格式固定：

| 目标格式 | quality | lossless | Alpha | background |
|---------|---------|----------|-------|------------|
| JPEG | 生效 | 不支持（报错） | 按背景色铺底 | 生效 |
| PNG | 忽略 | 始终无损（参数无额外影响） | 保留 | 忽略 |
| WebP | 生效（无损模式下为压缩强度） | 切换无损编码 | 保留（有损/无损均不丢失） | 忽略 |

<details>
<summary>命令参数</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<src>` | (必填) | 输入图片路径（仅 `jpg` / `jpeg` / `png` / `webp`） |
| `<dst>` | (必填) | 输出路径；目标格式由 `.jpg` / `.jpeg` / `.png` / `.webp` 扩展名决定 |
| `-q, --quality` | `80` | JPEG/WebP 输出质量；WebP 无损模式下表示压缩强度，PNG 忽略该参数 |
| `--lossless` | `false` | 使用 WebP 无损编码；PNG 始终为无损格式，该参数对 PNG 无额外影响 |
| `--background` | `#FFFFFF` | 输出 JPEG 时透明区域使用的背景色（必须为不透明颜色） |

</details>

## 图像水印

为图片添加文字或图片水印；文字水印支持两种模式：位置水印（单点）和重复平铺水印，图片水印仅支持位置水印。图片水印模式下，`<dst>` 也不得与 `--image` 指向同一文件。

### 位置水印（position）

在指定位置添加单个水印，自动根据背景亮度选择黑/白文字颜色，并添加描边提高可读性。

```bash
# 默认右下角
./itb watermark -t "© Author" photo.jpg

# 指定位置
./itb watermark -t "Copyright" --position center photo.png

# 调整透明度
./itb watermark -t "Author" --opacity 0.8 photo.png

# 指定输出路径
./itb watermark -t "Author" photo.jpg output.jpg

# 添加图片水印
./itb watermark --image logo.png --scale 0.2 --position bottom-right photo.jpg
```

### 重复平铺水印（repeat）

文字以平铺方式覆盖整张图片，支持旋转角度和间距调整。

```bash
# 基本用法
./itb watermark -t "WATERMARK" --mode repeat photo.png

# 自定义旋转角度和透明度
./itb watermark -t "DRAFT" --mode repeat --angle 45 --opacity 0.3 photo.png

# 自定义颜色
./itb watermark -t "CONFIDENTIAL" --mode repeat --color "#FF0000" photo.png
```

<details>
<summary>命令参数</summary>

**通用参数**

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<src>` | (必填) | 输入图片路径 |
| `-t, --text` | 与 `--image` 二选一 | 水印文字 |
| `[dst]` | `*_watermarked.*` | 输出路径，省略时在原文件名后加 `_watermarked` |
| `-m, --mode` | `position` | 水印模式：`position`（位置）/ `repeat`（平铺） |
| `--color` | (自动) | 水印颜色（`#RGB` / `#RRGGBB` / `#RRGGBBAA`）；空则自动选择黑/白 |
| `--opacity` | `0.5` | 透明度，范围 0~1 |
| `--font-size` | `0` | 字体大小，`0` 表示根据图片自动计算，上限 4096 |
| `--font` | (自动) | 字体文件路径，空则自动使用可用的默认字体 |
| `--image` | 与 `--text` 二选一 | 图片水印路径 |
| `--scale` | `0.2` | 图片水印缩放比例，基于底图短边 |

**position 模式参数**

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--position` | `bottom-right` | 水印位置：`bottom-right` / `bottom-left` / `top-right` / `top-left` / `center` |
| `--margin` | `0.04` | 边距比例，基于图片短边计算，不能为负 |

**repeat 模式参数**

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--angle` | `30` | 旋转角度（度），范围 -360~360 |
| `--space` | `0` | 平铺间距，`0` 表示根据字体大小自动计算 |

</details>

## 图片质量比较

只读比较两张图片的客观质量指标（PSNR / SSIM / MS-SSIM），纯 Go 实现，不依赖外部工具。

```bash
# 默认：PSNR + MS-SSIM
./itb compare original.jpg compressed.jpg

# 仅 SSIM
./itb compare original.jpg compressed.jpg --ssim

# 全部指标
./itb compare original.jpg compressed.jpg \
  --psnr --ssim --ms-ssim
```

<details>
<summary>命令参数</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<src>` | (必填) | 基准图片 |
| `<dst>` | (必填) | 待比较图片 |
| `--psnr` | - | 计算 PSNR（峰值信噪比，单位 dB；完全一致为 `+Inf`） |
| `--ssim` | - | 计算 SSIM（结构相似性，11×11 高斯窗口标准参数） |
| `--ms-ssim` | - | 计算 MS-SSIM（固定五尺度，短边需 ≥ 161 像素） |

</details>

> 未指定任何指标 flag 时默认计算 PSNR 和 MS-SSIM；一旦指定任意指标 flag，只计算显式选择的指标。

比较语义：

- 支持 JPEG / PNG / WebP；JPEG 的 EXIF Orientation 已归一化，比较的是实际视觉像素
- 两张图片的逻辑尺寸必须完全一致（`1920×1080` 与 `1280×720` 会直接报错），不会隐式 resize / crop / pad
- `<src>` 与 `<dst>` 都是只读输入，同一文件自我比较是合法的 sanity check（输出 `PSNR: +Inf dB`）
- SSIM 要求两边均 ≥ 11×11；MS-SSIM 固定五尺度，要求短边 ≥ 161 像素，小图请改用 `--psnr` 或 `--ssim`
- 含 Alpha 的图片使用 itb 定义的 alpha-aware 变体（premultiplied RGB + A 共同参与比较），数值不应要求与只比较 RGB 的第三方工具逐位一致
- 输出顺序固定为 PSNR、SSIM、MS-SSIM，例如：

```text
PSNR: 42.318274 dB
SSIM: 0.976391
MS-SSIM: 0.987423
```

该能力仅通过 CLI 暴露，不进入 HTTP API。

## 图片检查

读取图片文件信息、内容识别结论、图像基本信息、详细元数据和文件 hash。

**内容识别（`itb.inspect.v3` 起）**：格式从**文件内容**识别（magic 嗅探 + SVG 流式
XML 解析），从不依赖扩展名。支持识别 PNG / JPEG / GIF / WebP / BMP / TIFF / SVG：
其中 BMP、TIFF 可完整光栅解码（基于 `golang.org/x/image`）；SVG 会被识别为图片
（`recognized=true`）但**不做光栅解码**（`decode_supported=false`）——这是能力边界
而非文件损坏，没有显式 width/height 的 SVG 同样合法。

```bash
# 默认表格输出，默认计算所有 hash，默认输出详细数据
./itb inspect photo.jpg

# JSON 输出
./itb inspect --format json photo.jpg

# 只输出 sha256
./itb inspect --format plain photo.jpg

# 关闭详细数据
./itb inspect --no-detail photo.jpg

# 不计算 hash
./itb inspect --no-hash photo.jpg

# 只计算指定算法（可重复指定）
./itb inspect --hash sha256 --hash crc32 --no-detail --format json photo.jpg

# 完整解码校验（GIF 逐帧），可作为上传前 preflight
./itb inspect --strict --full-decode --format json image.png
```

<details>
<summary>命令参数</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<src>` | (必填) | 输入图片路径 |
| `--format` | `table` | 输出格式：`table` / `json` / `plain`（`plain` 仅输出 SHA-256） |
| `--no-detail` | `false` | 不输出详细元数据 |
| `--detail` | `true` | 兼容保留（已从 help 隐藏），等价于不传 `--no-detail` |
| `--hash` | (全部) | 只计算指定算法（可重复：`sha256` / `sha1` / `md5` / `crc32`）；未指定时计算全部 |
| `--no-hash` | `false` | 不计算文件 hash（与 `--hash` 互斥） |
| `--strict` | `false` | 图像解析失败时直接返回错误 |
| `--full-decode` | `false` | 完整解码图片（GIF 逐帧），校验文件后半部分并输出帧数/动画状态 |

</details>

哈希计算由共享的 `internal/filehash` 实现，采用**单次流式读取 + 读取后变化检测**：
hash 完成后复查文件（`os.SameFile` + size/modtime），检测到读取期间发生可观察变化
（就地修改、被替换或删除）时以 `E_SOURCE_CHANGED` 失败，不输出可信度存疑的摘要；
无法检测保留 size 与 modtime 的恶意并发修改。`plain` 输出要求 sha256 在计算集合中
（`--hash crc32` 之类的组合会报参数错误）。

JSON 契约版本为 `itb.inspect.v3`。v3 新增 `content` 内容识别对象：

```json
{
  "content": {
    "format": "jpeg",
    "canonical_extension": ".jpg",
    "mime_type": "image/jpeg",
    "recognized": true,
    "decode_supported": true,
    "full_decode_supported": true,
    "extension_matches": true
  }
}
```

| 字段 | 说明 |
|------|------|
| `format` / `canonical_extension` / `mime_type` | 识别出的规范格式名、规范扩展名与权威 MIME（来自格式注册表）；未识别时省略 |
| `recognized` | 文件内容是否被识别为受支持格式（含 SVG） |
| `decode_supported` | 是否存在光栅解码器；SVG 为 `false`（能力边界，非损坏） |
| `full_decode_supported` | 是否支持 `--full-decode`；SVG 为 `false`（对其使用该 flag 记录 warning） |
| `extension_matches` | 文件扩展名是否与识别出的格式一致（`.jpeg`/`.tif` 等 alias 也算匹配） |

检测流程分三个阶段：**内容识别 → 结构校验（`image.DecodeConfig`，SVG 跳过）→
可选完整解码**。未识别内容保留 v2 行为（`error.decode_config_failed`，`--strict`
报错）；已识别但结构损坏的文件同样保留该结论。默认（header 解码）只读取图片头，
无法发现"文件头正常但后半部分损坏"的文件；`--full-decode` 对文件做完整解码并补充
以下字段：

| 字段 | 说明 |
|------|------|
| `full_decode_ok` | 三态：省略 = 未尝试；`true` = 完整解码通过；`false` = 文件后半部分损坏 |
| `frame_count` | GIF 完整解码得到的帧数（其他格式省略） |
| `animation_known` | `animated` 是否可信：JPEG/PNG/BMP/TIFF 恒为 `true`；GIF 需要 `--full-decode`；WebP 来自 VP8X 头嗅探 |
| `animated` | 动画状态，仅在 `animation_known=true` 时有意义 |

## HTTP API（itb serve）

除本地 CLI 外，`itb serve` 提供 `/api/v1` HTTP API，直接调用领域包而不执行 CLI 子进程。API 面向可信的个人 VPS 部署，不提供 WebUI、S3 管理、工作流或用户系统。

```bash
# 启动 HTTP API（默认 http://127.0.0.1:8080）
./itb serve

# 指定监听地址
./itb serve --addr 127.0.0.1:9000

```

### 功能范围

- **图片操作**：`compress`、`resize`、`crop`、`rotate`、`convert`、`watermark`、`inspect`
- **不提供**：S3 管理、WebUI、工作流、用户系统、数据库或任务队列

### 安全边界

默认只监听 `127.0.0.1`。`ITB_API_TOKEN` 是生产环境必需的 Bearer Token；生产部署应由 Nginx/Caddy 在 HTTPS 下反向代理至 localhost。仅本地开发可以使用 `--no-auth`，且它只能绑定 loopback 地址。

<details>
<summary>命令参数与 HTTP API</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--addr` | `127.0.0.1:8080` | 监听地址 |
| `--max-upload` | `64MiB` | 最大 multipart 请求大小 |
| `--max-pixels` | `50000000` | 最大图片像素数（含上传图片、水印图与计划输出尺寸） |
| `--max-dimension` | `16384` | 最大图片单边尺寸（含上传图片、水印图与计划输出尺寸） |
| `--max-concurrent` | `2` | 最大并发图片操作数 |
| `--max-working-bytes` | `512MiB` | 单个操作中间画布内存上限（watermark、任意角度 rotate 等） |
| `--timeout` | `2m` | 单个图片操作超时 |
| `--no-auth` | `false` | 仅 loopback 本地开发时禁用认证 |

API 统一前缀 `/api/v1`，例如健康检查：

```bash
curl http://127.0.0.1:8080/api/v1/health
```

图片处理端点使用与 CLI long flag 同名的 `multipart/form-data` 字段，处理结果以流式二进制响应返回。完整参数与部署说明见 [API 文档](docs/api.md) 和 [VPS 部署文档](docs/deployment.md)。

```bash
curl -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@photo.png' \
  -F 'to=webp' \
  -F 'quality=80' \
  https://itb.example.com/api/v1/convert -o photo.webp
```

</details>

<details>
<summary>从源码构建</summary>

```bash
make build    # 编译 itb
make serve    # 构建并启动 HTTP API
make check    # go vet
make test     # go test
```

</details>

## S3 兼容存储操作

S3 的对象/文件 operand 使用位置参数：`upload <src> [key]`、`download <key> [dst]`、`stat/delete <key>`、`list [prefix]`。连接配置与执行策略继续使用 flags 或 `ITB_S3_*` 环境变量。

支持 AWS S3、MinIO、阿里云 OSS、腾讯云 COS 等所有 S3 协议兼容的存储服务。

输出约定：**stdout 只承载正式结果**（`--format table|json` 切换；upload/download/stat/list 的
JSON 均携带 `schema_version` 契约，list 的契约版本为 `itb.s3.list.v2`），进度提示与诊断信息走
**stderr**，脚本可以放心用管道消费 stdout 的 JSON。

### 机器可读错误契约（itb.error.v1）

任何请求了 `--format json` 的命令在失败时，stdout 统一输出一份 `itb.error.v1` JSON 文档，
stderr 不再重复打印同一错误（非 JSON 模式行为不变：错误文本走 stderr）：

```json
{
  "schema_version": "itb.error.v1",
  "operation": "s3.download",
  "error": {
    "code": "E_CHECKSUM_MISMATCH",
    "message": "downloaded content does not match the expected SHA-256",
    "retryable": false,
    "http_status": null,
    "provider_code": null
  }
}
```

稳定错误码清单：

| 错误码 | 含义 |
|--------|------|
| `E_INVALID_ARGUMENT` | 参数非法（参数解析错误、operand 数量与 flag 组合冲突） |
| `E_INVALID_CONFIG` | 连接配置不完整（endpoint/bucket 等） |
| `E_FILE_NOT_FOUND` | 本地文件不存在 |
| `E_FILE_READ` | 本地文件读取失败（权限等） |
| `E_SOURCE_CHANGED` | 读取期间源文件发生可观察变化 |
| `E_OBJECT_NOT_FOUND` | 对象不存在 |
| `E_BUCKET_NOT_FOUND` | 存储桶不存在 |
| `E_ACCESS_DENIED` | 访问被拒绝（权限不足） |
| `E_INVALID_CREDENTIALS` | 凭证缺失、不完整或被 provider 判定无效（InvalidAccessKeyId/SignatureDoesNotMatch/ExpiredToken） |
| `E_TIMEOUT` | 操作或网络超时 |
| `E_NETWORK` | 网络通信失败 |
| `E_THROTTLED` | 服务端限流/过载（SlowDown、Throttling 等错误码） |
| `E_CHECKSUM_MISMATCH` | 下载内容与期望 SHA-256 不一致 |
| `E_TARGET_CONFLICT` | 目标对象已存在且与期望状态不一致 |
| `E_UNSUPPORTED_CAPABILITY` | provider 不支持所需能力 |
| `E_INCOMPLETE_LIST` | 列举无法可靠继续 |
| `E_INTERNAL` | itb 内部错误 |

安全边界：S3 provider 错误只透出 `http_status` 与 `provider_code` 两个字段，
`message` 使用固定摘要；Authorization、SecretAccessKey、SessionToken、signed URL
以及 provider 原始错误文本绝不进入该输出。

MinIO 兼容性由 CI 持续验证：CI 在 step 内以 `docker run` 启动真实 MinIO
（GitHub Actions 的 service container 不支持容器命令，无法传入
`server /data`），并设置 `ITB_REQUIRE_MINIO=1`（strict 模式：MinIO 不可用时
测试失败而非跳过），执行覆盖
upload / stat / download / skip-existing / skip-unchanged / skip-matching /
metadata / cache-control / overwrite / verify / delete / list 分页 /
条件上传（If-None-Match）/ 期望值校验 / 本地副本复用与 path-style 的集成测试
（`internal/s3/minio_test.go`）；本地 `go test` 在 MinIO 不可达时自动跳过，
可通过 `ITB_TEST_MINIO_ENDPOINT` 等环境变量指向自建实例运行。
不依赖 MinIO 的编译后二进制 E2E（`internal/cmd/e2e_test.go`）锁定 inspect
内容识别契约（PNG/JPEG/GIF/WebP/BMP/TIFF/SVG/伪装与损坏样本）、
`itb.error.v1` stdout/stderr 单文档语义与 compress 失败不留 partial。

### 环境变量

```bash
ITB_S3_ENDPOINT           # S3 端点 URL
ITB_S3_ACCESS_KEY_ID      # Access Key ID
ITB_S3_SECRET_ACCESS_KEY  # Secret Access Key
ITB_S3_SESSION_TOKEN      # 临时凭证 Session Token（可选）
ITB_S3_REGION             # 区域（默认 us-east-1）
ITB_S3_BUCKET             # 存储桶名称（可省略 -b）
ITB_S3_FORCE_PATH_STYLE   # 强制路径样式 URL（true/false）
```

配置优先级：CLI flag > `ITB_S3_*` 环境变量 > 默认值；环境变量可满足 `--endpoint` / `--access-key` / `--secret-key` / `--bucket` 的必填校验。

临时凭证（AccessKey + SecretKey + SessionToken）建议全部通过环境变量注入，
避免 Session Token 进入 shell history。

<details>
<summary>公共参数</summary>

所有 S3 子命令共享以下参数：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-e, --endpoint` | (必填) | S3 端点 URL（或 `ITB_S3_ENDPOINT`） |
| `-a, --access-key` | (必填) | Access Key ID（或 `ITB_S3_ACCESS_KEY_ID`） |
| `-s, --secret-key` | (必填) | Secret Access Key（或 `ITB_S3_SECRET_ACCESS_KEY`） |
| `--session-token` | (空) | 临时凭证 Session Token（或 `ITB_S3_SESSION_TOKEN`，建议用环境变量） |
| `-r, --region` | `us-east-1` | 区域 |
| `-b, --bucket` | (必填) | 存储桶名称（或 `ITB_S3_BUCKET`） |
| `--force-path-style` | `false` | 强制路径样式 URL（MinIO 需要；或 `ITB_S3_FORCE_PATH_STYLE`；loopback / `:9000` 端点自动启用） |
| `--max-attempts` | `3` | 单个 S3 API 操作的最大尝试次数（含首次，AWS SDK 标准 retryer 自动重试） |
| `--connect-timeout` | `30s` | TCP 连接建立超时 |
| `--response-header-timeout` | `30s` | 等待响应头超时（不限制 body 传输时长） |
| `--operation-timeout` | `0`（禁用） | 整个操作的总时长上限（list 全部分页 / upload + verify / download）；显式配置时可能中断大文件传输 |

网络超时说明：不设置 http 请求总超时——大文件 GET/PUT 的传输时长取决于
文件大小与网络状况；`--response-header-timeout` 只限制等待响应头，
`--operation-timeout` 通过 context 作用于整个操作（含上传稳定快照与
`--verify` 回读），`0` 表示禁用。

</details>

### 上传文件

```bash
# 上传文件到存储桶
./itb s3 upload -b my-bucket -e http://localhost:9000 photo.jpg

# 指定对象键名（默认使用文件名）
./itb s3 upload -b my-bucket photo.jpg images/photo.jpg

# 指定 Content-Type
./itb s3 upload -b my-bucket --content-type application/json data.json

# 写入用户 metadata（key=value，可重复；键转小写，itb-sha256 为保留键）
./itb s3 upload -b my-bucket image.webp image/xx.webp \
  --metadata source-sha256=abc123 --metadata width=1920 --metadata height=1080

# 设置标准 HTTP 响应头（稳定 URL 发布）
./itb s3 upload -b my-bucket --cache-control no-cache image.webp

# 同名对象已存在即跳过（1 次 HEAD 代替整文件上传）
./itb s3 upload -b my-bucket --skip-existing photo.jpg

# 内容一致才跳过（比对 itb-sha256 metadata，不依赖 ETag）
./itb s3 upload -b my-bucket --skip-unchanged photo.jpg

# 完整状态一致才跳过（复用远端对象，status=reused）
./itb s3 upload -b my-bucket --skip-matching photo.jpg \
  --metadata source-sha256=abc123 --cache-control no-cache

# 不可覆盖条件上传（If-None-Match，用于 sha256/xxx 这类不可变对象键）
./itb s3 upload -b my-bucket --if-exists verify original.png sha256/xxx.png

# PUT 后追加 1 次 HEAD，校验远端属性与本次上传一致
./itb s3 upload -b my-bucket --verify photo.jpg
```

上传时会把本地文件的 SHA-256 写入对象用户 metadata（`x-amz-meta-itb-sha256`），
`--skip-unchanged` 依赖该值判断远端对象与本地是否一致；默认行为仍是无条件覆盖。
`--skip-existing`、`--skip-unchanged` 与 `--skip-matching` 是三个互斥的上传策略，
同时使用多个会报参数错误。

**`--skip-matching`（完整状态匹配）**：远端对象的完整状态与本次上传的预期一致才
跳过（JSON `status=reused`）。始终比对 **SHA-256、Content-Length、Content-Type**；
调用方显式指定的 `--cache-control` / `--content-disposition` / `--content-encoding`
/ `--metadata` 也必须匹配——采用 **requested subset matching**：请求的每个 metadata
键都必须原样在场且相等，远端多出的 metadata 不影响匹配；未指定的 header 表示
"don't care" 而非"要求为空"。请求契约：命中为单次 `HEAD`；未命中为 `HEAD → PUT`
（再加 `--verify` 时为 `HEAD → PUT → HEAD`，最多两个 HEAD）。

**`--if-exists verify`（不可覆盖条件上传）**：通过条件写
（`PutObject IfNoneMatch="*"`）实现"不存在才写入"：对象不存在 → 正常上传；
已存在 → provider 返回 412，itb 随后 HEAD 并按完整预期状态匹配
（`matchesExpectedState`，与 `--skip-matching` 同一事实来源）：一致则
`status=reused`，不一致返回 `E_TARGET_CONFLICT`。**核心是 provider 原子判定的
条件写，绝不以 "HEAD + 判断 + PUT" 模拟**（那有 TOCTOU 竞态）。并发条件写冲突
（AWS 定义的 409 `ConditionalRequestConflict`）已加入 SDK retryer 的可重试错误码，
重试读取的是同一份稳定快照；provider 明确不支持条件头（501 `NotImplemented`）
时以 `E_UNSUPPORTED_CAPABILITY` 失败，**绝不自动降级**。默认 `--if-exists replace`
保持无条件覆盖。

**稳定快照语义**：上传前源文件会被复制到一份私有临时快照（0600，用后即删），
SHA-256 在同一次读取中计算，随后源文件做可观察变化检测（`os.SameFile` +
size/modtime）。这保证 `itb-sha256` 与实际 PUT body 严格对应、SDK 重试每次
读取的都是同一份稳定数据、原文件在快照之后的任何变化不影响本次上传；快照期间
源文件被修改/替换/删除时整个上传以 `E_SOURCE_CHANGED` 失败，不上传内容不一致
的数据。Content-Type 基于快照内容检测，扩展名兜底仍使用原始文件名。

`--metadata` 必须是 `key=value` 格式（可重复指定），键统一转小写、不可为空、
禁含控制字符，重复键与保留键 `itb-sha256` 会报参数错误（在任何网络请求之前）。

`--content-type` 缺省时按**文件内容**检测 MIME（magic sniff 覆盖 JPEG/PNG/GIF/
WebP/PDF/ZIP/HTML/JSON/SVG），扩展名仅在内容无法识别时兜底——HTML 错误页改名成
`error.jpg` 会以 `text/html` 上传，而不是 `image/jpeg`。

<details>
<summary>upload 参数</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<src>` | (必填) | 本地文件路径 |
| `[key]` | 文件名 | 对象键名 |
| `--content-type` | 内容检测 | 内容类型（显式指定原样生效） |
| `--metadata` | (空) | 对象用户 metadata `KEY=VALUE`（可重复） |
| `--cache-control` | (空) | Cache-Control 响应头（如 `no-cache`、`max-age=31536000`） |
| `--content-disposition` | (空) | Content-Disposition 响应头 |
| `--content-encoding` | (空) | Content-Encoding 响应头 |
| `--skip-existing` | `false` | 对象键已存在即跳过上传 |
| `--skip-unchanged` | `false` | 内容一致才跳过上传（比对 itb-sha256 metadata） |
| `--skip-matching` | `false` | 完整状态一致才跳过（sha256/size/Content-Type + 显式请求的 header/metadata，`status=reused`） |
| `--if-exists` | `replace` | 写入策略：`replace`（无条件覆盖）/ `verify`（不可覆盖条件上传 If-None-Match） |
| `--verify` | `false` | PUT 后追加 1 次 HEAD，校验远端 size/Content-Type/HTTP 头/metadata 与本次上传一致 |
| `--format` | `table` | 输出格式：`table` / `json`（JSON 契约 `itb.s3.upload.v2`） |

</details>

`--format json` 契约为 `itb.s3.upload.v2`，新增 `status` 字段：
`uploaded`（已上传）/ `skipped`（命中 skip-existing/skip-unchanged）/ `reused`
（完整状态一致，复用远端对象）；`skipped`/`reason` 字段兼容保留。

`--verify` 的请求契约：默认上传 `PUT`；`--verify` 为 `PUT → HEAD`；
`--skip-existing` 命中为单次 `HEAD`，未命中加 `--verify` 为
`HEAD → PUT → HEAD`；`--skip-unchanged` 命中为单次 `HEAD`。
HEAD 校验只能证明 header/metadata 与预期一致，**不等于** body SHA-256
校验；body 完整性校验由 `download --verify` / `--verify-sha256` 承担。

跳过命中的 JSON 结果字段语义：`--skip-existing` 命中时 `size` 为本地
输入文件的字节数（`sha256` 未计算、留空）；`--skip-unchanged` 命中时
`size` 与 `sha256` 均为本地文件的确切值。

### 下载文件

```bash
# 下载文件
./itb s3 download -b my-bucket photo.jpg ./photo.jpg

# 未指定 [dst] 时保存到当前目录，文件名取对象键最后一段（photo.jpg）
./itb s3 download -b my-bucket images/photo.jpg

# 边下载边校验（读取对象 itb-sha256 metadata，单遍计算，不二次读取本地文件）
./itb s3 download -b my-bucket --verify photo.jpg

# 按已知哈希校验（provider-neutral 完整性验证，可与 --verify 同用）
./itb s3 download -b my-bucket --verify-sha256 "$SOURCE_SHA256" sha256/xxx /tmp/original.png

# 期望值校验：对象大小与 Content-Type
./itb s3 download -b my-bucket --expect-size 123456 \
  --expect-content-type image/png photo.jpg

# 本地副本一致则复用（跳过 GET，status=reused）
./itb s3 download -b my-bucket --verify-sha256 "$SOURCE_SHA256" \
  --if-exists verify sha256/xxx /tmp/original.png
```

下载先写入同目录临时文件，成功后 rename 到目标路径；任何失败（网络中断、
写盘错误、校验不通过）都会删除临时文件，目标路径不会留下 partial 文件。
`--verify` / `--verify-sha256` 哈希不一致时返回校验错误（`ErrChecksumMismatch`），
这才是对 body 字节的真正完整性校验（upload 的 `--verify` 只校验 header/metadata）。
`--verify-sha256` 必须是 64 个十六进制字符（32 字节）的合法 SHA-256 digest，
否则在任何网络请求之前返回参数错误（`ErrInvalidSHA256`）。

**期望值校验**：`--expect-size`（指针语义，`0` 是合法的对象大小而非"未指定"）
与 `--expect-content-type`（比较忽略 `; charset=...` 参数与大小写）会在创建
目标文件**之前**对 GET 响应头检查一次，下载结束后再对实际写入字节数检查一次；
任一不符返回 `E_TARGET_CONFLICT`（`ErrExpectationMismatch`），不留任何文件。

**本地复用（`--if-exists verify`）**：默认 `replace` 总是执行 GET 并覆盖（与
v0.9.x 一致）。`verify` 只在调用方提供校验依据（`--verify-sha256`，或 `--verify`
时先 HEAD 取远端 itb-sha256）时才允许跳过 GET：本地副本 size/SHA-256 与期望
一致则复用（JSON `status=reused`）；副本存在但不一致返回 `E_TARGET_CONFLICT`；
本地不存在则正常下载。**没有任何校验依据时直接报 `E_INVALID_ARGUMENT`——绝不
"文件存在就复用"。**

`--format json` 契约为 `itb.s3.download.v2`：新增 `status`（`downloaded` /
`reused`）与 `content_type` 字段。

<details>
<summary>download 参数</summary>

| 参数 | 说明 |
|------|------|
| `<key>` | 对象键名（必填） |
| `[dst]` | 本地输出路径（默认保存到当前目录，文件名取对象键最后一段） |
| `--verify` | 读取对象 itb-sha256 metadata，边下载边计算 SHA-256 并比对 |
| `--verify-sha256` | 期望的 SHA-256（64 个十六进制字符），独立于对象 metadata 的完整性校验 |
| `--expect-size` | 期望的对象字节数（响应头与实际字节各检查一次） |
| `--expect-content-type` | 期望的 Content-Type（忽略参数与大小写） |
| `--if-exists` | 目标已存在策略：`replace`（默认，总是 GET 覆盖）/ `verify`（本地副本可证明一致则复用） |
| `--format` | 输出格式：`table` / `json`（JSON 契约 `itb.s3.download.v2`） |

</details>

### 删除对象

```bash
# 删除对象（需要确认）
./itb s3 delete -b my-bucket photo.jpg

# 强制删除（不需要确认）
./itb s3 delete -b my-bucket -f photo.jpg
```

<details>
<summary>delete 参数</summary>

| 参数 | 说明 |
|------|------|
| `<key>` | 对象键名（必填） |
| `-f, --force` | 强制删除，不确认 |

</details>

### 列出对象

```bash
# 列出所有对象
./itb s3 list -b my-bucket

# 按前缀过滤
./itb s3 list -b my-bucket images/

# JSON 格式输出
./itb s3 list -b my-bucket --format json

# 完整分页：持续翻页直到遍历结束
./itb s3 list -b my-bucket image/ --all --format json

# 控制单页大小与总输出上限
./itb s3 list -b my-bucket image/ --page-size 500 --limit 5000 --format json

# 从上一次返回的 continuation token 恢复遍历
./itb s3 list -b my-bucket image/ --continuation-token TOKEN --format json
```

list 默认只请求一页（`MaxKeys` 上限 1000，与 v0.9.x 单页行为一致）；
`--all` 才持续翻页直到遍历结束。

JSON 契约版本为 `itb.s3.list.v2`（v2 起 JSON 从裸对象数组升级为结构化对象）：

```json
{
  "schema_version": "itb.s3.list.v2",
  "bucket": "my-bucket",
  "prefix": "image/",
  "complete": true,
  "count": 2,
  "pages": 1,
  "next_continuation_token": "...",
  "objects": [
    {"key": "image/a.png", "size": 123, "last_modified": "...", "etag": "\"...\"", "storage_class": "STANDARD"}
  ]
}
```

`complete=true` 只表示**从本次起始 token 开始已正常遍历结束**：

- 单页模式下服务端还有后续页，或被 `--limit` 截断时 `complete=false`，
  并返回 `next_continuation_token` 供 `--continuation-token` 恢复；
- `--limit` 截断发生在 S3 请求边界（最后一次请求的 `MaxKeys` 收缩为剩余
  配额），恢复遍历不会跳过任何对象；
- 服务端报告还有更多对象但 token 缺失、重复或不前进时，整个命令以
  `E_INCOMPLETE_LIST` 失败，不输出半份成功结果；
- 中间任何一页请求失败时整个命令失败，同样不输出半份 JSON。

<details>
<summary>list 参数</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `[prefix]` | | 对象键前缀 |
| `--page-size` | `1000` | 单次 ListObjectsV2 请求的 `MaxKeys`（1-1000）；`--max-keys` 保留为 v0.9.x 兼容 alias |
| `--all` | `false` | 持续翻页直到遍历结束（默认只请求一页） |
| `--limit` | `0` | 输出对象总数上限（`0` = 不限制）；截断时 `complete=false` 并携带恢复 token |
| `--continuation-token` | (空) | 从上一次 list 返回的 token 恢复遍历 |
| `--format` | `table` | 输出格式：`table` / `json` / `plain`（JSON 契约 `itb.s3.list.v2`） |

</details>

### 查看对象元数据

```bash
# 查看单个对象的完整元数据（只发一次 HEAD 请求，不下载内容）
./itb s3 stat -b my-bucket images/photo.jpg

# JSON 格式输出
./itb s3 stat -b my-bucket --format json images/photo.jpg
```

stat 始终按精确对象键查询，对象不存在时不回退到 list 推断。返回的元数据包括
Size、ETag、Content-Type、Storage Class、Cache-Control、Version ID 与用户 Metadata。

`--format json` 携带机器可读契约版本 `schema_version: itb.s3.stat.v1`：

```json
{
  "schema_version": "itb.s3.stat.v1",
  "key": "...",
  "size": 123,
  "content_type": "...",
  "metadata": {}
}
```

脚本应依赖 `schema_version` 判断契约版本，而不是解析终端文本。

<details>
<summary>stat 参数</summary>

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<key>` | (必填) | 对象键名 |
| `--format` | `table` | 输出格式：`table` / `json` |

</details>

<details>
<summary>云服务商配置示例</summary>

| 云服务商 | Endpoint 示例 | ForcePathStyle |
|---------|---------------|----------------|
| AWS S3 | `https://s3.amazonaws.com` | `false` |
| MinIO | `http://localhost:9000` | `true` |
| 阿里云 OSS | `https://oss-cn-hangzhou.aliyuncs.com` | `false` |
| 腾讯云 COS | `https://cos.ap-guangzhou.myqcloud.com` | `false` |

</details>

## 许可证

本项目使用 MIT 许可证。内置的第三方工具请参阅 [LICENSE-THIRD-PARTY.md](./LICENSE-THIRD-PARTY.md)。
