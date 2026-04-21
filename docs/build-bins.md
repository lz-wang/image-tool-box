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

## libjpeg-turbo

- 仓库地址: <https://github.com/libjpeg-turbo/libjpeg-turbo.git>
- 当前版本: [Release 3.1.3 · libjpeg-turbo/libjpeg-turbo](https://github.com/libjpeg-turbo/libjpeg-turbo/releases/tag/3.1.3)

建议统一使用 `-DENABLE_SHARED=FALSE -DENABLE_STATIC=TRUE` 构建静态版本的 `cjpeg` / `djpeg`。

常见产物包括：

- `cjpeg-static`
- `djpeg-static`
- `jpegtran-static`

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

### Linux amd64

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

构建完成后，将对应平台产物复制到本仓库的 `bins/<os>-<arch>/` 目录，并在 [internal/compress/embed.go](/Users/lzwang/projects/ImageToolBox/internal/compress/embed.go) 中补充或校验对应平台的二进制映射。

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
