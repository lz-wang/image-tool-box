# GO 图像工具箱

## 图像压缩

### 外部依赖

#### libjpeg-turbo 有损压缩JPG

- 仓库地址: <https://github.com/libjpeg-turbo/libjpeg-turbo.git>
- 当前版本: [Release 3.1.3 · libjpeg-turbo/libjpeg-turbo](https://github.com/libjpeg-turbo/libjpeg-turbo/releases/tag/3.1.3)

构建方式

下面补充 macOS / Linux 下 `amd64` 与 `arm64` 的静态构建示例。

其他平台可按相同方式分别构建静态版本的 `cjpeg` / `djpeg`。建议统一使用 `-DENABLE_SHARED=FALSE -DENABLE_STATIC=TRUE`，并按目标平台创建单独的构建目录。

构建完成后，可执行文件通常位于构建目录下，常见产物包括：

- `cjpeg-static`
- `djpeg-static`
- `jpegtran-static`

建议在本项目中按 `bins/<os>-<arch>/` 组织，例如：

- `bins/macos-amd64/`
- `bins/macos-arm64/`
- `bins/linux-amd64/`
- `bins/linux-arm64/`

##### macOS amd64

```bash
git clone https://github.com/libjpeg-turbo/libjpeg-turbo.git
cd libjpeg-turbo

mkdir build-macos-amd64
cd build-macos-amd64

cmake .. \
  -DENABLE_SHARED=FALSE \
  -DENABLE_STATIC=TRUE \
  -DCMAKE_OSX_ARCHITECTURES=x86_64 \
  -DCMAKE_BUILD_TYPE=Release

make -j
```

##### macOS arm64

```bash
git clone https://github.com/libjpeg-turbo/libjpeg-turbo.git
cd libjpeg-turbo

mkdir build-macos-arm64
cd build-macos-arm64

cmake .. \
  -DENABLE_SHARED=FALSE \
  -DENABLE_STATIC=TRUE \
  -DCMAKE_OSX_ARCHITECTURES=arm64 \
  -DCMAKE_BUILD_TYPE=Release

make -j
```

##### Linux amd64

```bash
git clone https://github.com/libjpeg-turbo/libjpeg-turbo.git
cd libjpeg-turbo

mkdir build-linux-amd64
cd build-linux-amd64

cmake .. \
  -DENABLE_SHARED=FALSE \
  -DENABLE_STATIC=TRUE \
  -DCMAKE_SYSTEM_NAME=Linux \
  -DCMAKE_SYSTEM_PROCESSOR=x86_64 \
  -DCMAKE_BUILD_TYPE=Release

make -j
```

##### Linux arm64

如果在 arm64 Linux 主机原生构建：

```bash
git clone https://github.com/libjpeg-turbo/libjpeg-turbo.git
cd libjpeg-turbo

mkdir build-linux-arm64
cd build-linux-arm64

cmake .. \
  -DENABLE_SHARED=FALSE \
  -DENABLE_STATIC=TRUE \
  -DCMAKE_SYSTEM_NAME=Linux \
  -DCMAKE_SYSTEM_PROCESSOR=aarch64 \
  -DCMAKE_BUILD_TYPE=Release

make -j
```

如果在其他平台交叉编译，需要额外指定 toolchain，例如：

```bash
cmake .. \
  -DENABLE_SHARED=FALSE \
  -DENABLE_STATIC=TRUE \
  -DCMAKE_SYSTEM_NAME=Linux \
  -DCMAKE_SYSTEM_PROCESSOR=aarch64 \
  -DCMAKE_TOOLCHAIN_FILE=/path/to/toolchain.cmake \
  -DCMAKE_BUILD_TYPE=Release
```

如果要接入当前项目，还需要将对应平台产物复制到本仓库的 `bins/<os>-<arch>/` 目录，并在 [internal/compress/embed.go](/Users/lzwang/projects/ImageToolBox/internal/compress/embed.go) 中补充对应平台的二进制映射。

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
./itb watermark -i photo.png -t "Copyright" --position center

# 调整透明度
./itb watermark -i photo.png -t "Author" --opacity 0.8

# 指定输出路径
./itb watermark -i photo.jpg -t "Author" -o output.jpg
```

### 重复平铺水印（repeat）

文字以平铺方式覆盖整张图片，支持旋转角度和间距调整。

```bash
# 基本用法
./itb watermark -i photo.png -t "WATERMARK" --mode repeat

# 自定义旋转角度和透明度
./itb watermark -i photo.png -t "DRAFT" --mode repeat --angle 45 --opacity 0.3

# 自定义颜色
./itb watermark -i photo.png -t "CONFIDENTIAL" --mode repeat --color "#FF0000"
```

### 命令参数

#### 通用参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-i, --input` | (必填) | 输入图片路径 |
| `-t, --text` | (必填) | 水印文字 |
| `-o, --output` | `*_watermarked.*` | 输出路径，默认在原文件名后加 `_watermarked` |
| `-m, --mode` | `position` | 水印模式：`position`（位置）/ `repeat`（平铺） |
| `--color` | (自动) | 水印颜色，如 `#FF0000`；空则自动选择黑/白 |
| `--opacity` | `0.5` | 透明度，范围 0~1 |
| `--font-size` | `0` | 字体大小，`0` 表示根据图片自动计算 |
| `--font` | (自动) | 字体文件路径，空则自动使用系统字体 |

#### position 模式参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--position` | `bottom-right` | 水印位置：`bottom-right` / `bottom-left` / `top-right` / `top-left` / `center` |
| `--margin` | `0.04` | 边距比例，基于图片短边计算 |

#### repeat 模式参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--angle` | `30` | 旋转角度（度） |
| `--space` | `0` | 平铺间距，`0` 表示根据字体大小自动计算 |

## S3 兼容存储操作

支持 AWS S3、MinIO、阿里云 OSS、腾讯云 COS 等所有 S3 协议兼容的存储服务。

### 环境变量

```bash
S3_ENDPOINT             # S3 端点 URL（可选）
S3_ACCESS_KEY_ID        # Access Key
S3_SECRET_ACCESS_KEY    # Secret Key
S3_REGION               # 区域（默认 us-east-1）
```

### 公共参数

所有 S3 子命令共享以下参数：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-e, --endpoint` | (必填) | S3 端点 URL |
| `-a, --access-key` | (环境变量) | Access Key ID |
| `-s, --secret-key` | (环境变量) | Secret Access Key |
| `-r, --region` | `us-east-1` | 区域 |
| `-b, --bucket` | (必填) | 存储桶名称 |
| `--force-path-style` | `false` | 强制路径样式 URL（MinIO 需要） |

### 上传文件

```bash
# 上传文件到存储桶
./itb s3 upload -i photo.jpg -b my-bucket -e http://localhost:9000

# 指定对象键名（默认使用文件名）
./itb s3 upload -i photo.jpg -b my-bucket -k images/photo.jpg

# 指定 Content-Type
./itb s3 upload -i data.json -b my-bucket --content-type application/json
```

#### upload 参数

| 参数 | 说明 |
|------|------|
| `-i, --input` | 本地文件路径（必填） |
| `-k, --key` | 对象键名（默认使用文件名） |
| `--content-type` | 内容类型（自动检测） |

### 下载文件

```bash
# 下载文件
./itb s3 download -b my-bucket -k photo.jpg -o ./photo.jpg

# 使用对象键名作为本地文件名
./itb s3 download -b my-bucket -k images/photo.jpg
```

#### download 参数

| 参数 | 说明 |
|------|------|
| `-k, --key` | 对象键名（必填） |
| `-o, --output` | 本地输出路径（默认使用对象键名） |

### 删除对象

```bash
# 删除对象（需要确认）
./itb s3 delete -b my-bucket -k photo.jpg

# 强制删除（不需要确认）
./itb s3 delete -b my-bucket -k photo.jpg -f
```

#### delete 参数

| 参数 | 说明 |
|------|------|
| `-k, --key` | 对象键名（必填） |
| `-f, --force` | 强制删除，不确认 |

### 列出对象

```bash
# 列出所有对象
./itb s3 list -b my-bucket

# 按前缀过滤
./itb s3 list -b my-bucket -p images/

# JSON 格式输出
./itb s3 list -b my-bucket --format json
```

#### list 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-p, --prefix` | | 对象键前缀 |
| `--max-keys` | `1000` | 最大返回数量 |
| `--format` | `table` | 输出格式：`table` / `json` / `plain` |

### 云服务商配置示例

| 云服务商 | Endpoint 示例 | ForcePathStyle |
|---------|---------------|----------------|
| AWS S3 | `https://s3.amazonaws.com` | `false` |
| MinIO | `http://localhost:9000` | `true` |
| 阿里云 OSS | `https://oss-cn-hangzhou.aliyuncs.com` | `false` |
| 腾讯云 COS | `https://cos.ap-guangzhou.myqcloud.com` | `false` |

### LskyPro 上传

支持上传图片到 LskyPro 图床，兼容直接传站点根地址或完整的 `/api/v1` 地址。

### 环境变量

```bash
LSKY_URL    # LskyPro 地址，例如 https://img.example.com 或 https://img.example.com/api/v1
LSKY_TOKEN  # API Token
```

### 上传图片

```bash
# 使用环境变量上传
./itb lsky upload -i photo.jpg

# 显式指定服务地址和 Token
./itb lsky upload -i photo.jpg --url https://img.example.com --token your-token

# 指定存储策略 ID
./itb lsky upload -i photo.jpg --strategy 2

# 以 JSON 输出完整响应
./itb lsky upload -i photo.jpg --output json

# 输出 URL
./itb lsky upload -i photo.jpg --output url
```

#### upload 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-i, --input` | (必填) | 本地图片路径 |
| `--url` | (环境变量) | LskyPro 服务地址 |
| `--token` | (环境变量) | LskyPro API Token |
| `-s, --strategy` | `0` | 存储策略 ID，`0` 表示不指定 |
| `-o, --output` | `markdown` | 输出格式：`markdown` / `url` / `json` |

## 许可证

本项目使用 MIT 许可证。内置的第三方工具请参阅 [LICENSE-THIRD-PARTY.md](./LICENSE-THIRD-PARTY.md)。
