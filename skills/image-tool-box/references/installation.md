# 安装与环境

仅在缺少 `itb`、需要升级、安装 Skill 或排查运行环境时读取。

## 先检查已有程序

```bash
command -v itb
itb --version
itb --help
```

工作区已有构建产物时，使用 `./itb --version` 和 `./itb --help`。Windows 可使用 `Get-Command itb` 与 `itb.exe --version`。

同一机器可能有多个版本。后续操作统一使用选定的可执行文件，并运行相应的 `itb <command> --help`；条码等新增功能需要该版本实际提供对应命令。

## Homebrew 安装与升级

macOS 或 Linux 已有 Homebrew 时：

```bash
brew tap lz-wang/tap
brew install lz-wang/tap/itb
itb --version
```

已安装时升级：

```bash
brew update
brew upgrade lz-wang/tap/itb
itb --version
```

## 下载官方发行包

没有 Homebrew 或使用 Windows 时，从 [官方发布页](https://github.com/lz-wang/image-tool-box/releases) 选择所需版本，下载对应平台归档及同名 `.sha256` 文件。版本号写入归档名时不带前导 `v`。

| 操作系统 | 处理器 | 归档名称 |
|----------|--------|----------|
| macOS | Intel / Apple Silicon | `itb_<版本>_macos_amd64.tar.gz` / `itb_<版本>_macos_arm64.tar.gz` |
| Linux | x86-64 / ARM64 | `itb_<版本>_linux_amd64.tar.gz` / `itb_<版本>_linux_arm64.tar.gz` |
| Windows | x86-64 / ARM64 | `itb_<版本>_windows_amd64.zip` / `itb_<版本>_windows_arm64.zip` |

在下载目录中先校验归档。Linux 使用 `sha256sum -c <归档名>.sha256`，macOS 使用 `shasum -a 256 -c <归档名>.sha256`；Windows 使用 `Get-FileHash <归档名> -Algorithm SHA256`，对照校验文件中的摘要。校验失败时停止安装。

macOS/Linux 使用 `tar -xzf <归档名>`；归档中的目录为 `itb-<平台>-<架构>/`，程序在其中。将 `itb` 放入 PATH 中的目录，例如 `~/.local/bin/`，并确保可执行权限。Windows 使用 `Expand-Archive <归档名> -DestinationPath <目标目录>`，将含 `itb.exe` 的目录加入 PATH。最后运行 `itb --version` 和 `itb --help` 验证。

正式发行包只有 `itb` 可执行程序；压缩器和条码解码器已内嵌，首次使用时按需提取。运行时无需另装 pngquant、oxipng、libjpeg-turbo、Python、uv 或 ZXing。

Linux 官方原生压缩与条码解码工具要求 **glibc ≥ 2.28**；Alpine/musl 当前不受支持。

## 从源码构建

需要开发或尚未发布的功能时，在本仓库中使用 `go.mod` 要求的 Go 版本：

```bash
make build
./itb --version
./itb --help
```

`make build` 只编译 Go 主程序，不下载或构建原生工具。新检出仓库可能没有平台二进制，此时纯 Go 功能可用，但压缩和条码解码需要按仓库的 `docs/build-bins.md` 补齐对应平台工具后重新构建。一般图片用户优先使用正式发行包。

## 安装本 Skill

Skill 文件与 `itb` 程序分别安装。将本仓库的完整 `skills/image-tool-box/` 目录复制到目标代理配置的 Skill 搜索目录，保留 `SKILL.md`、`agents/` 和 `references/` 的相对位置。

下面以 Codex 的用户 Skill 目录为例，在仓库根目录执行；若目标已有同名目录，先检查并备份，再替换，避免合并残留文件：

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
cp -R skills/image-tool-box "${CODEX_HOME:-$HOME/.codex}/skills/"
```

按目标代理的方式重新加载或开启新会话后，使用 `$image-tool-box` 调用。若旧版 `itb` Skill 也是从本仓库复制安装的，检查并备份旧目录后移除该副本，避免重复发现；其他代理使用其配置的 Skill 搜索路径。

## 常见环境问题

- 找不到命令：检查 PATH，或使用可执行文件的明确路径；复制 Skill 不会自动安装程序。
- 命令或选项不存在：检查当前程序的版本和对应 `--help`，再判断是否需要升级。
- 缺少内嵌原生工具：检查是否为仅编译主程序的开发版本；使用正式发行包或补齐构建工具，不额外部署独立 ZXing 来替代内嵌机制。
