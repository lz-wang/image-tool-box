# 外部依赖说明

本文档说明本项目使用的外部依赖、`bins/<os>-<arch>/` 目录约定，以及相关二进制的构建方式。

## 目录约定

建议按以下目录组织：

- `bins/macos-amd64/`
- `bins/macos-arm64/`
- `bins/linux-amd64/`
- `bins/linux-arm64/`
- `bins/windows-amd64/`
- `bins/windows-arm64/`

其中 Windows 平台产物统一使用 `.exe` 扩展名，例如：

- `pngquant.exe`
- `oxipng.exe`
- `cjpeg-static.exe`
- `djpeg-static.exe`
- `zxing-reader.exe`

## libjpeg-turbo

- 仓库地址: <https://github.com/libjpeg-turbo/libjpeg-turbo.git>
- 当前版本: [Release 3.1.3 · libjpeg-turbo/libjpeg-turbo](https://github.com/libjpeg-turbo/libjpeg-turbo/releases/tag/3.1.3)

建议统一使用 `-DENABLE_SHARED=FALSE -DENABLE_STATIC=TRUE` 构建静态版本的 `cjpeg` / `djpeg`。

本项目仅使用以下静态工具：

- `cjpeg-static`
- `djpeg-static`

`jpegtran-static` 没有运行时使用方，因此不会构建或嵌入发布二进制。

### macOS amd64

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

### macOS arm64

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

### Linux amd64 / arm64

Linux 内置压缩器的发布 ABI 基线是 **glibc 2.28**。不要在 Ubuntu runner 上直接构建它们；CI 会通过 `scripts/build-linux-bins-container.sh` 在对应的 PyPA `manylinux_2_28` 容器中完成构建：

```bash
./scripts/build-linux-bins-container.sh amd64 bins/linux-amd64
./scripts/verify-linux-abi.sh amd64 bins/linux-amd64
```

arm64 使用同样的流程：

```bash
./scripts/build-linux-bins-container.sh arm64 bins/linux-arm64
./scripts/verify-linux-abi.sh arm64 bins/linux-arm64
```

构建脚本固定 Rust 1.89.0 和对应 manylinux 镜像 digest；`pngquant` 启用 `static,z-static` feature，并且 `cjpeg-static` / `djpeg-static` 静态链接 libjpeg-turbo。ABI 校验会验证 ELF 架构、拒绝 `GLIBC_2.29+`，也会拒绝 `libz`、`libpng`、`liblcms`、`libstdc++` 等非基础 Linux 共享库依赖。

### 手动调试 Linux amd64

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

### Linux arm64

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

构建完成后，将对应平台产物复制到本仓库的 `bins/<os>-<arch>/` 目录，并在 `internal/nativebin/registry.go` 中校验对应平台的二进制映射。

### Windows amd64 / arm64

建议在对应架构的 Windows Runner 或主机上原生构建。CI 中当前使用 GitHub Actions Windows Runner 原生构建 `pngquant`、`oxipng` 和 `libjpeg-turbo`，并将产物放入：

- `bins/windows-amd64/`
- `bins/windows-arm64/`

`libjpeg-turbo` 在 Windows 上建议使用 CMake + Visual Studio 生成器，常见输出包括：

- `Release/cjpeg-static.exe`
- `Release/djpeg-static.exe`

发布阶段，Windows 构建产物使用 `.zip` 打包；macOS / Linux 保持 `.tar.gz`。

## pngquant

- 仓库地址: <https://github.com/kornelski/pngquant>
- 项目网站: [pngquant — lossy PNG compressor](https://pngquant.org/)
- 当前版本: 3.0.3

CI 中当前通过源码构建 `pngquant`，也可以复用 workflow 中的做法在 macOS、Linux、Windows 上手工构建。

## oxipng

- 仓库地址: <https://github.com/oxipng/oxipng.git>
- 当前版本: [Release v10.1.0 · oxipng/oxipng](https://github.com/oxipng/oxipng/releases/tag/v10.1.0)

CI 中当前通过源码构建 `oxipng`，也可以复用 workflow 中的做法在 macOS、Linux、Windows 上手工构建。

## ZXing-C++ reader

- 固定版本：[v3.1.1](https://github.com/zxing-cpp/zxing-cpp/releases/tag/v3.1.1)
- 源码归档 SHA-256：`7286b1e6ade66fe82b7c8208b4595deeb55d6486b410834fdc65702f46650542`
- 构建入口：`native/zxing-reader/CMakeLists.txt`；C++20、CMake >= 3.20。
- 仅 reader：`ZXING_READERS=ON`、`ZXING_WRITERS=OFF`、`BUILD_SHARED_LIBS=OFF`。只启用 QR（含 Micro QR/rMQR）和 1D；Aztec/DataMatrix/MaxiCode/PDF417 禁用。helper 没有图片编解码、文件路径、网络、用户配置或 writer 接口。

macOS 原生或对应架构构建示例：

```bash
cmake -S native/zxing-reader -B /tmp/itb-zxing-reader \
  -DCMAKE_BUILD_TYPE=Release -DCMAKE_OSX_ARCHITECTURES=arm64
cmake --build /tmp/itb-zxing-reader --parallel
cmake --install /tmp/itb-zxing-reader --component ITBReader --prefix "$PWD/bins/macos-arm64"
```

amd64 将架构改为 `x86_64`，目录改为 `bins/macos-amd64`。Windows 使用 Visual Studio `-A x64` / `-A ARM64`、`--config Release` 并安装到对应 `bins/windows-*`；CMake 固定静态 MSVC runtime。平台产物为 `zxing-reader`，Windows 为 `zxing-reader.exe`。`ITBReader` 安装组件只复制 helper，避免把 ZXing 静态库或头文件放入 embed 目录。

Linux 与压缩器一起通过 `scripts/build-linux-bins-container.sh` 在固定 digest 的 `manylinux_2_28` 中构建，依赖构建期 `glibc-static`。helper 完全静态链接 C++/GCC/glibc runtime；`scripts/verify-linux-abi.sh` 的原有基础动态库 allowlist 不变，禁止额外 `libstdc++`、`libgcc_s` 等运行时依赖。CI 同时校验架构与 glibc 2.28 上限。

Go 主程序始终 `CGO_ENABLED=0`。`main.go` 将 `bins/**` 注入 `nativebin.Init`，按工具单独提取到用户缓存 `itb/bins/<platform>/<sha256>/`，复用前校验 SHA-256；首次解码才提取 reader。运行时只分发 `itb`，无需另装 ZXing 或 Python/uv。

Go 将 JPEG/PNG/WebP 解码并归一化为 Gray8，stdin 私有协议为 `ITBZ` + version `1` + 大端 uint32 的 width/height/stride/format-mask + 像素；stdout 是版本化内部 JSON。该协议不属于 CLI/HTTP 公共 schema。

### 原生测试与 fixture

两个 workflow 在六个平台的 helper 构建后强制执行 roundtrip、多码、旋转、反色、EXIF、Micro QR/rMQR、compiled CLI 和 HTTP 测试。Micro QR/rMQR 测试单独构建 `native/zxing-test-writer`，固定同版本 ZXing 的 `NEW` writer、experimental API 与上游固定 zint submodule；生成 PGM 像素后由 Go 测试转为临时 PNG，不签入二进制 fixture。

测试 writer 只在 CI/本地验收阶段使用，绝不安装到 `bins/`、嵌入或发布。本地严格验收：

```bash
cmake -S native/zxing-test-writer -B /tmp/itb-zxing-test -DCMAKE_BUILD_TYPE=Release
cmake --build /tmp/itb-zxing-test --parallel
ITB_REQUIRE_BARCODE_NATIVE=1 \
ITB_TEST_ZXING_READER="$PWD/bins/macos-arm64/zxing-reader" \
ITB_TEST_ZXING_WRITER=/tmp/itb-zxing-test/zxing-test-writer \
CGO_ENABLED=0 go test ./internal/barcode ./internal/cmd ./internal/httpapi -run '^(TestNative|TestBarcode)' -v
```

reader 路径与平台相符；Windows 的 reader/writer 路径均带 `.exe`，writer 位于 `Release/`。普通纯 Go 单测在 reader 缺失时只跳过原生测试；`ITB_REQUIRE_BARCODE_NATIVE=1` 使缺失 reader 或测试 writer 直接失败。
