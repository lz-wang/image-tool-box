---
name: image-tool-box
description: 使用 itb 图片工具箱完成图片压缩、裁剪、缩放、旋转、格式转换、水印、信息检查、质量比较、二维码与条码生成或解码，以及 S3 兼容存储操作和 HTTP API 集成。适用于本地图片处理与脚本自动化；缺少 itb 时提供安装指引。
---

# 图片工具箱

将图片处理需求转为可执行的 `itb` 命令。Skill 名称是 `image-tool-box`，可执行程序仍叫 `itb`。

## 工作方式

1. 优先使用工作区或 PATH 中已有的 `itb`；查看 `itb --version`，再查看目标命令的 `--help`。工作区程序用 `./itb` 调用。缺少程序或需要新版功能时，读取安装指引。
2. 根据任务选择下表中的命令，只读取相关参考文档。已安装版本的帮助是参数、默认值和可用功能的权威来源，参考文档用于补充示例和操作约束。
3. 默认保留原图，将多步处理的中间文件放到临时目录或任务输出目录，并依次执行。
4. 完成后检查输出文件、格式和尺寸；面向用户的图片在可行时预览，并交付实际输出路径。

## 按需阅读

| 当前任务 | 命令选择 | 参考文档 |
|----------|----------|----------|
| 安装程序、升级或安装本 Skill | 检查版本和可用命令 | [安装与环境](references/installation.md) |
| 减小 PNG/JPEG 文件体积 | `compress` | [图片处理](references/image-operations.md#压缩) |
| 按锚点裁剪、改尺寸或旋转 | `crop`、`resize`、`rotate` | [图片处理](references/image-operations.md) |
| 转换格式或添加水印 | `convert`、`watermark` | [图片处理](references/image-operations.md) |
| 检查格式、尺寸、哈希或图片质量 | `inspect`、`compare` | [图片检查与比较](references/image-operations.md#检查图片与哈希) |
| 生成二维码或条码、读取图片中的码 | `barcode generate`、`barcode decode` | [二维码与条码](references/barcode.md) |
| 上传、下载、列举、查询或删除对象 | `s3` | [S3 兼容存储](references/s3.md) |
| 为远程自动化提供图片接口 | `serve` | [HTTP API](references/http-api.md) |

无需一次加载全部参考文档；先读当前任务对应的小节，遇到其他步骤再读取相应文档。

## 共用约束

- 文件变换使用 `itb <command> [options] <src> [dst]`；`convert` 的 `<dst>` 必填。`compare <src> <dst>` 的两个路径都是只读输入。
- `resize`、`crop`、`rotate`、`convert`、`watermark` 接受 JPEG/PNG/WebP，JPEG 的 EXIF 方向会先归一化。`inspect` 能识别更多格式，不代表这些格式能用于变换。
- 输出不能与任一输入指向同一实际文件，包括等价路径、硬链接和符号链接；图片水印文件也是输入。仅在用户明确要求时使用 `compress --in-place`。
- 本地和脚本任务优先使用命令行；需要 HTTP 集成时才使用 `serve`。
- 自动化优先使用命令支持的 `--format json`，按 `schema_version` 解析结果；`compare` 当前只有文本输出。JSON 模式失败时，标准输出恰好包含一份 `itb.error.v1`，标准错误不重复打印，且退出码非零。
- S3 凭据和 API 令牌通过环境注入，不打印或写进命令历史。远端删除、覆盖策略按用户已经授权的范围执行。
