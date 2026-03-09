# GO 图像工具箱

## 图像压缩

### 外部依赖

#### libjpeg-turbo 有损压缩JPG

- 仓库地址: <https://github.com/libjpeg-turbo/libjpeg-turbo.git>
- 当前版本: [Release 3.1.3 · libjpeg-turbo/libjpeg-turbo](https://github.com/libjpeg-turbo/libjpeg-turbo/releases/tag/3.1.3)

构建方式

```bash
git clone https://github.com/libjpeg-turbo/libjpeg-turbo.git
cd libjpeg-turbo

mkdir build-macos-arm64
cd build-macos-arm64

cmake .. \
  -DENABLE_SHARED=FALSE \
  -DENABLE_STATIC=TRUE \
  -DCMAKE_OSX_ARCHITECTURES=x86_64 \
  -DCMAKE_BUILD_TYPE=Release

make -j
```

#### pngquant 有损压缩PNG

- 仓库地址: <https://github.com/kornelski/pngquant>
- 项目网站: [pngquant — lossy PNG compressor](https://pngquant.org/)
- 当前版本: 3.0.3

#### oxipng 无损压缩PNG


- 仓库地址: <https://github.com/oxipng/oxipng.git>
- 当前版本: [Release v10.1.0 · oxipng/oxipng](https:-//github.com/oxipng/oxipng/releases/tag/v10.1.0)

### 使用方法

#### 构建

```bash
make build
```

#### 压缩图片

自动检测图片格式（PNG/JPEG）并压缩：

```bash
# 压缩 PNG 图片（覆盖原文件）
./itb compress -i photo.png

# 压缩 JPEG 图片（覆盖原文件）
./itb compress -i photo.jpg

# 指定输出文件
./itb compress -i photo.png -o compressed.png

# 指定压缩质量（1-100，默认 80）
./itb compress -i photo.jpg -q 90
```

#### 命令参数

| 参数 | 说明 |
|------|------|
| `-i, --input` | 输入图片文件路径 |
| `-o, --output` | 输出图片文件路径（不指定则覆盖原文件） |
| `-q, --quality` | 压缩质量 1-100（默认 80） |

#### 压缩管道

- **PNG**: `pngquant` → `oxipng`（有损 + 无损双重压缩）
- **JPEG**: `djpeg` → `cjpeg`（libjpeg-turbo 解码 + 编码）

## 图像水印

为图片添加文字水印，支持两种模式：位置水印（单点）和重复平铺水印。

### 位置水印（position）

在指定位置添加单个水印，自动根据背景亮度选择黑/白文字颜色，并添加描边提高可读性。

```bash
# 默认右下角
./itb watermark -i photo.jpg -t "© Author"

# 指定位置
./itb watermark -i photo.png -t "Copyright" -p center

# 调整透明度
./itb watermark -i photo.png -t "Author" --opacity 0.8

# 指定输出路径
./itb watermark -i photo.jpg -t "Author" -o output.jpg
```

#### 位置参数

| 值 | 说明 |
|----|------|
| `bottom-right` | 右下角（默认） |
| `bottom-left` | 左下角 |
| `top-right` | 右上角 |
| `top-left` | 左上角 |
| `center` | 居中 |

### 重复平铺水印（repeat）

文字以平铺方式覆盖整张图片，支持旋转角度和间距调整。

```bash
# 基本用法
./itb watermark -i photo.png -t "WATERMARK" --mode repeat

# 自定义旋转角度和透明度
./itb watermark -i photo.png -t "DRAFT" --mode repeat --angle 45 --opacity 0.3

# 自定义颜色和间距
./itb watermark -i photo.png -t "CONFIDENTIAL" --mode repeat --color "#FF0000" --space 100
```

### 命令参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-i, --input` | (必填) | 输入图片路径 |
| `-t, --text` | (必填) | 水印文字 |
| `-o, --output` | `xxx_watermarked.ext` | 输出路径（默认在原文件名后加 `_watermarked`） |
| `-m, --mode` | position | 水印模式：position / repeat |
| `-p, --position` | bottom-right | 水印位置（position 模式） |
| `--margin` | 0.04 | 边距比例（position 模式） |
| `--opacity` | 0.5 | 透明度 0~1，数值越大字体颜色越深 |
| `--color` | #4db6ac | 水印颜色（repeat 模式） |
| `--angle` | 30 | 旋转角度（repeat 模式） |
| `--space` | 75 | 平铺间距（repeat 模式） |
| `--font-size` | 48 | 字体大小（repeat 模式） |
| `--font` | (自动) | 字体文件路径（不指定则自动使用系统字体） |

### 字体说明

- **位置水印**：自动使用系统字体，无需指定
- **重复水印**：默认使用内置 Go 字体，可通过 `--font` 指定自定义字体

## 图像上传

### S3上传

TODO

### LskyPro 上传

TODO
