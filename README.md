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
- 当前版本: 

#### oxipng 无损压缩PNG


- 仓库地址: <https://github.com/oxipng/oxipng.git>
- 当前版本: [Release v10.1.0 · oxipng/oxipng](https:-//github.com/oxipng/oxipng/releases/tag/v10.1.0)

### 使用方法

TODO

## 图像水印

### 位置水印

TODO

### 重复水印

## 图像上传

### S3上传

TODO

### LskyPro 上传

TODO
