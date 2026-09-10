# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

本轮围绕"可靠上传/下载/列举/校验 + 机器可读输出"共 11 个独立 commit，全部稳定 JSON schema 清单如下：

| Schema | 用途 |
|--------|------|
| `itb.error.v1` | CLI 失败输出的统一机器可读错误契约（`--format json` 时失败输出到 stdout，含稳定 `E_*` 错误码） |
| `itb.inspect.v3` | inspect JSON（新增 `content` 内容识别对象；保留 v2 的 full-decode 字段） |
| `itb.compress.v1` | compress `--format json`（input/output 的 path/format/size/sha256、quality、processor、elapsed_ms） |
| `itb.s3.list.v2` | s3 list JSON（从裸数组升级为结构化对象，携带 complete/分页元数据） |
| `itb.s3.upload.v2` | s3 upload JSON（新增 `status`：uploaded/skipped/reused；skipped/reason 兼容保留） |
| `itb.s3.download.v2` | s3 download JSON（新增 `status`：downloaded/reused 与 `content_type`） |

### Added

- **Commit 1** `s3 list` 完整分页契约：`--page-size`（1-1000，`--max-keys` 保留为 v0.9.x alias）、`--all`、`--limit`、`--continuation-token`；`--limit` 截断发生在 S3 请求边界（MaxKeys 收缩为剩余配额），token 恢复不跳过对象；token 缺失/重复/不前进时返回 `E_INCOMPLETE_LIST`，绝不输出半份结果。
- **Commit 2** 统一机器可读错误契约 `itb.error.v1`：请求 `--format json` 的命令失败时 stdout 输出一份错误 JSON（schema_version/operation/error{code,message,retryable,http_status,provider_code}），stderr 不再重复；锁定 17 个稳定 `E_*` 错误码；S3 provider 错误只透出 HTTP 状态与 provider code，凭据/签名/原始 provider 文本绝不透出。
- **Commit 3** 共享哈希包 `internal/filehash`：单遍流式多算法摘要（sha256/sha1/md5/crc32）；`SumFile` 读取后以 `os.SameFile` + size/modtime 检测可观察变化，变化时报 `E_SOURCE_CHANGED`；`inspect` 新增可重复 `--hash`（与 `--no-hash` 互斥），未指定时仍计算全部算法。
- **Commit 4** `inspect` 内容识别层重构（`itb.inspect.v3`）：三阶段检测（内容识别 → 结构校验 → 可选完整解码）；格式注册表单点维护；BMP/TIFF 经 `golang.org/x/image` 完整解码；SVG 经流式 XML 解析识别（`recognized=true`、`decode_supported=false`，不是损坏，无显式宽高合法），HTML 改名 `.svg` 必须失败。
- **Commit 5** `compress` 结构化输出与安全提交：`--format json`（`itb.compress.v1`，含 input/output sha256、processor 固定命名 `pngquant+oxipng` / `djpeg+cjpeg`）；输出先写目标目录临时文件、校验后原子 rename，失败不留 partial 文件。
- **Commit 6** `s3 upload` 稳定本地快照：源文件复制到私有临时快照并单遍计算 SHA-256，`itb-sha256` 与实际 PUT body 严格对应，SDK 重试 rewind 安全；快照期间源文件可观察变化时以 `E_SOURCE_CHANGED` 失败。
- **Commit 7** `s3 upload --skip-matching`：远端完整状态（sha256/size/Content-Type + 显式请求的 header/metadata，requested subset matching）一致才跳过（`status=reused`）；三个 skip 策略互斥。
- **Commit 8** `s3 download` 期望值与本地复用：`--expect-size`（指针三态，0 字节合法）、`--expect-content-type`（参数与大小写不敏感）；`--if-exists verify` 仅在有校验依据时复用本地副本（`status=reused`），无依据直接 `E_INVALID_ARGUMENT`，绝不"文件存在就复用"。
- **Commit 9** `s3` 网络控制暴露：`--max-attempts`（默认 3，AWS SDK 标准 retryer）、`--connect-timeout`/`--response-header-timeout`（默认 30s）、`--operation-timeout`（默认 0 禁用，作用于整个操作上下文）。
- **Commit 10** `s3 upload --if-exists verify` 不可覆盖条件上传：真条件写 `IfNoneMatch="*"`（绝不 HEAD+判断+PUT 模拟）；412 后 HEAD 按完整状态匹配决定 reused/`E_TARGET_CONFLICT`；409 `ConditionalRequestConflict` 加入 SDK retryer 可重试；provider 不支持时以 `E_UNSUPPORTED_CAPABILITY` 失败，绝不降级。
- **Commit 11** E2E 收口：MinIO 集成覆盖分页（3 对象 + page-size 2 强制两页）、skip-matching、条件上传、期望值校验与本地复用；编译后二进制 E2E 锁定 inspect 内容识别契约（七种格式 + 伪装/损坏样本）、错误契约 stdout/stderr 单文档语义与 compress 失败不留 partial。

### Fixed

- **收尾 1** `fix(ci)`：MinIO 集成/CLI E2E 移至 Codecov 上传之前执行，`fail_ci_if_error` 降为 false——coverage 服务故障或受保护分支缺 token 不再阻断功能验证与构建矩阵。
- **收尾 2** `fix(cli)` 补完 `itb.error.v1` 分类 taxonomy：Action 内参数错误（operand 数量、flag 组合冲突）引入 typed `InvalidArgumentError`，不再误报 `E_INTERNAL`；provider 限流错误码（SlowDown/Throttling 等）稳定映射 `E_THROTTLED`（`ErrorDetail` 新增 Throttled 标记，5xx 保持 `E_NETWORK` 可重试）；`InvalidAccessKeyId`/`SignatureDoesNotMatch`/`ExpiredToken` 及 HTTP 401 映射为 `E_INVALID_CREDENTIALS`（与"凭证未配置"、403 `E_ACCESS_DENIED` 区分）；`WrapError` 全分支以双 `%w` 保留原始 provider 错误链，保证 `provider_code`/`http_status` 可提取。
- **收尾 3** `fix(compress)` 稳定源快照消除 TOCTOU：新增内部原语 `internal/stablefile`（复制到私有临时快照 + 单遍 SHA-256 + 可观察变化检测），compress 的输入摘要、格式检测与压缩管道全部基于同一份快照——报告中的 input sha256/size 从此与实际压缩内容严格对应，源文件在处理期间被替换时以 `E_SOURCE_CHANGED` 失败；S3 upload 的快照逻辑迁移至同一原语，消除两处重复实现。输入必须是普通文件（FIFO/目录直接参数错误，不再可能在 open 阶段阻塞）。
- **收尾 4** `fix(s3)` 修复 download 本地复用的组合语义漏洞：`--verify` 或 `--expect-content-type` 在 `--if-exists verify` 复用路径此前可能被完全跳过（有显式 `--verify-sha256` 时不执行 HEAD）。现在复用不降低校验强度——`--verify` 与 `--expect-content-type` 强制先 HEAD 比对远端 itb-sha256 / Content-Type，显式 digest 与远端 metadata 矛盾时直接 `E_TARGET_CONFLICT`；只有 `--verify-sha256`（+ 可选 `--expect-size`）保持零网络复用。CLI help 与双语 README 同步更新组合语义。

## [v0.9.3] - 2026-09-03

### Changed

- `resize` 的 `--filter` 支持列表收敛为领域层单一事实来源：新增 `resize.Filter` 类型、过滤器常量与 `FilterNames()`，CLI 的 help 文案与参数校验改从 `FilterNames()` 派生，消除 domain/validator/help 三处重复维护的枚举。纯内部架构改进，命令行为、help 渲染与错误信息均无变化。

## [v0.9.2] - 2026-09-03

### Added

- `resize --filter mitchell` 新增 Mitchell-Netravali cubic 重采样器：输出较平滑，相比 Catmull-Rom 更少出现 ringing；默认 filter 仍为 `lanczos`。CLI `--filter` 枚举、HTTP API `filter` 字段与 `skills/itb` 命令参考同步开放该取值。

## [v0.9.1] - 2026-09-02

### Changed

- CLI help 全面英文化并升级为人类用户与 LLM agent 都可直接依赖的自包含命令契约：root help 按 Category 分组（Image transforms / Analysis / Storage / Service / Utility，由 urfave 原生渲染）；每个命令统一 `DEFAULTS / CONSTRAINTS / EXAMPLES` 结构，补齐此前只存在于 Skill/README 的隐含默认行为——`[dst]` 推导规则（`_compressed` / `_resized` / `_cropped` / `_rotated` / `_watermarked`）、S3 operand 默认（upload key = `basename(<src>)`、download dst = 当前目录 + 对象键最后一段、list 省略 prefix = 全量）、compare 的默认 PSNR + MS-SSIM 与显式选择规则、`--force-path-style` 的有效默认（loopback / `:9000` 端点自动启用）。计算型默认值通过 `DefaultText` 展示（watermark `--color` / `--font` / `--font-size` / `--space` = auto，s3 upload `--content-type` = auto-detect），普通默认值继续由 flag `Value` 单一维护并自动渲染。
- `inspect` 的 compatibility-only `--detail` 从 help 隐藏（行为保留，仍可显式传参），help 只展示 `--no-detail`；Description 明确详细元数据与 SHA-256 默认开启。
- 新增 help contract 测试：`TestAllHelpIsEnglish` 递归锁定全部命令 help 无汉字；table-driven 默认值 contract（28 条）；旧 flag（`--input` / `--output` / `--to` / `--key` / `--prefix` / `--tile`）不得回归。`skills/itb/SKILL.md` 改为以已安装二进制的 `itb <command> --help` 为权威命令来源，reference 作为补充。

## [v0.9.0] - 2026-09-02

### Added

- 新增 `itb rotate --angle <degrees> <src> [dst]` 图像旋转命令：正角度逆时针、负角度顺时针，支持小数，范围 `(-360, 360)` 且不能为 0；精确 90/180/270 不做插值，任意角度双线性插值并按 imaging 旋转包围盒规则调整输出画布，未覆盖区域 PNG/WebP 保持透明、JPEG 铺白色背景；省略 `[dst]` 时输出 `<name>_rotated.<ext>`。输入统一走 transform 契约（仅 JPEG/PNG/WebP，JPEG EXIF Orientation 先归一化再旋转）。
- HTTP API 新增 `POST /api/v1/rotate`（multipart：`input` + 浮点 `angle`）：先按 Probe 逻辑尺寸经领域 `Resolve` 推导旋转后画布并完成输出资源准入，再执行旋转——任意角度扩大画布导致的超限在分配之前返回 413 `image_too_large`。
- 新增 `itb compare <src> <dst>` 只读图片质量比较命令，纯 Go 实现 PSNR、SSIM 与五尺度 MS-SSIM；默认计算 PSNR + MS-SSIM，显式指标 flag 时仅计算所选指标。两图逻辑尺寸必须一致（不隐式缩放），JPEG EXIF Orientation 已归一化，含 Alpha 图片采用 premultiplied RGB + A 的 alpha-aware 变体；SSIM 最小 11×11，MS-SSIM 短边最小 161。仅 CLI 暴露，不进入 HTTP API。

### Fixed

- 修正 MS-SSIM 奇数宽/高边缘块的 2×2 下采样：原先对边缘像素做 clamp 重复采样并以可变除数修正，导致奇数尺寸图（如 321×257，以及 161 最小边界的每一层金字塔）的边缘块平均值被放大 2-4 倍；现在只平均实际存在的像素。偶数尺寸的数值不受影响。MS-SSIM 参考实现不再复用生产下采样 helper，并新增奇数尺寸（3×2/2×3/3×3/5×5/161×161/321×257）回归锁定。

### Changed

- `itb serve --max-working-bytes` 将 rotate 纳入工作集准入：任意角度旋转会同时驻留输入 NRGBA 副本与输出画布（保守估算 4 字节/像素，正交旋转只计输出画布），超限在分配前返回 413 `image_too_large`，与 watermark 的服务资源模型一致。
- `compare` 改为逐通道"提取-计算-复用"的流式处理：峰值工作集从物化全部 6/8 个 float32 平面降为一对可复用通道平面（4K 图约 199MiB → 66MiB，指标平面占用量降至 1/3），更稳妥地处理 24/48MP 摄影原图；YCbCr/Gray/Gray16/CMYK 天然不透明图不再为透明度检测做整图遍历。指标数值与命令行为不变。

## [v0.8.0] - 2026-09-01

### Fixed

- Image transform commands reject destinations that resolve to the source file, including equivalent paths, hard links, and symbolic links, preventing destructive partial overwrites. `compress --in-place` continues to use its temporary-file + rename path.

### Changed

- **BREAKING:** local image commands now use positional paths: `itb <command> [options] <src> [dst]`; `convert` requires `<dst>` and determines its format from the destination extension. Removed local image-command `-i`/`--input`, `-o`/`--output`, and `convert --to`.
- **BREAKING:** S3 object/file selectors now use positional operands: `s3 upload <src> [key]`, `s3 download <key> [dst]`, `s3 stat <key>`, `s3 delete <key>`, and `s3 list [prefix]`. Removed S3 locator flags `--input/-i`, `--output/-o`, `--key/-k`, and `--prefix/-p`.

## [v0.7.0] - 2026-09-01

### Added

- `itb s3 upload` 新增对象元数据与 HTTP 头：`--metadata key=value`（可重复）、`--cache-control`、`--content-disposition`、`--content-encoding`，随 PUT 写入对象；非法键值与系统保留键 `itb-sha256` 在任何网络请求之前拒绝。
- `itb s3 upload --verify`：PUT 成功后追加一次 HEAD，比对 Content-Length、Content-Type、Cache-Control、`itb-sha256` 与全部用户 metadata，不一致时明确指出失配字段。
- `itb s3 download --verify` / `--verify-sha256 <hex>`：边下载边计算 SHA-256，两种校验可同用；内容先写同目录临时文件，成功后 rename，任何失败路径都不留 partial 文件。
- `itb s3` 支持会话凭证：`--session-token` / `ITB_S3_SESSION_TOKEN`（以 `X-Amz-Security-Token` 参与签名）；`--force-path-style` 新增 `ITB_S3_FORCE_PATH_STYLE` 环境变量。
- `itb s3 stat`/`upload`/`download` 的 JSON 输出携带 `schema_version`；`upload`/`download` 新增 `--format table|json`（默认 table 不变）。stdout 只承载正式结果，进度与诊断走 stderr。
- `itb inspect --full-decode`（HTTP `full-decode`）：GIF 逐帧、其余格式整图解码，捕获"文件头正常但后半部分损坏"；schema 升级 `itb.inspect.v2`，新增 `full_decode_ok`、`frame_count`、`animation_known`、`animated`。
- `itb serve --max-working-bytes`（默认 512MiB）：对水印等操作的中间工作集执行内存准入，超限在分配前返回 413。
- MinIO 集成测试：CI 以 service 方式启动真实 MinIO 并强制执行（`ITB_REQUIRE_MINIO=1`），本地默认优雅跳过，可用 `ITB_TEST_MINIO_*` 指向自建实例。

### Changed

- S3 Content-Type 按文件内容检测：显式 `--content-type` > magic sniff（覆盖 JPEG/PNG/GIF/WebP/PDF/ZIP/HTML/JSON/SVG）> 扩展名兜底 > `application/octet-stream`；HTML 错误页改名 `.jpg` 后不再伪装 `image/jpeg` 上传。
- convert/resize/crop/watermark 统一解码入口 `imageio.OpenStatic`：输入严格限定 JPEG/PNG/WebP，GIF/BMP/TIFF 一律拒绝，animated GIF 不再被静默处理首帧（水印 logo 输入同样受限）。
- EXIF Orientation 统一语义：`imageio.Info` 区分物理尺寸与逻辑尺寸，Probe 返回应用旋转后的逻辑尺寸并与解码后 bounds 恒等；所有静态 transform 在解码阶段烘焙方向，竖拍 JPEG 不再横躺。
- resize `--percent` 修正为按百分比精确缩放（此前 `--percent 200` 在默认 fit 模式下输出仍为原尺寸）；输出规划 `Resolve` 与实际执行 `Apply` 恒等，fit 包围盒超限但真实输出在限内的请求不再被保守拒绝。
- WebP 编码固定保留 Alpha；lossless WebP 开启 `Exact`，透明像素下 RGB 完整保留；是否铺底由目标格式唯一决定，调用方无法再让 lossy WebP 静默丢失透明度。
- S3 领域结果与 CLI 输出分离：upload/download 返回结构化结果，进度提示经 stderr 输出，stdout 由 CLI 统一渲染。
- HTTP API 错误状态映射改为 typed errors 分派：超时 504、尺寸超限 413、不支持的格式 415；handler panic 收敛为 JSON 500；`internal/httpapi` 按职责拆分为 handler/middleware/multipart/operations/response。

### Fixed

- multipart 上传文件存储路径隔离：客户端 filename 只作为元数据，服务端存储路径一律由 `os.CreateTemp` 生成，`input=image=logo.png` 同名碰撞与输入输出互相覆盖不再发生。
- 数值校验拒绝 NaN/Inf：`opacity=NaN`、`scale=Inf`、`percent=NaN%` 此前可绕过范围检查；multipart 同名字段不论标量还是文件一律 400；API token 强制 `Bearer ` 前缀，裸 token 即使值正确也拒绝。
- 水印资源边界升级为操作工作集准入：文字 repeat 画布、平铺画布与旋转包围盒纳入保守内存估算，`scale=1000000`、`font-size=4096` 之类参数在分配前返回 413。
- JPEG background 强制不透明：`#00000000` 解析出的透明值不再静默变成默认白色，半透明色统一显式拒绝。
- `--verify-sha256` 严格校验 64 个十六进制字符，短串在任何网络请求之前失败。
- `--skip-existing` 命中时填充本地文件 size；`--skip-unchanged` 命中时填充 size 与 sha256，脚本消费方无需二次 stat。
- JPEG orientation 解析统一为单一 parser：XMP APP1 前置于 EXIF APP1 的文件不再出现 Probe 与解码方向漂移；非 EXIF APP1 的伪造 TIFF 头被拒绝。

## [v0.6.0] - 2026-08-31

### Changed

- HTTP API 由 Gin 迁移至 Go 标准库 `net/http`，并改用与 CLI long flag 一致的 multipart 字段。
- 图片操作的默认值和最终校验收敛至领域包；水印改为统一文件级领域入口。
- Linux 内嵌压缩工具的构建与 CI 门禁固定为 glibc 2.28 兼容契约；修正 Windows 内嵌二进制缓存命中。

### Added

- `ITB_API_TOKEN` Bearer 认证、上传/图片/并发/超时限制与优雅关闭。
- 结构化 API 错误、`slog` 访问日志与流式图片响应。

### Removed

- **BREAKING:** 移除内置 WebUI、React/MUI/Vite 前端及 `itb serve --open`；`itb serve` 现为纯 HTTP API 服务。
- **BREAKING:** 移除 Gin 和旧版 `file + options JSON` HTTP 参数契约。

## [v0.5.0] - 2026-08-30

### Added

- 新增 `itb serve` WebUI：React 19 + TypeScript + MUI 前端经 `go:embed` 内嵌进二进制，默认绑定 `127.0.0.1:8080`，支持 `--addr` 与 `--open`；`/api/v1` 提供压缩、缩放、裁剪、转换、文字/图片水印等单图处理接口，前端支持 Before/After 对比、以鼠标为中心的滚轮缩放、拖拽平移、系统明暗主题持久化与处理结果并排展示。
- 新增 `itb s3 stat`：单次 HEAD 查询对象元数据（大小、ETag、Content-Type、用户 metadata 等），不传输对象内容，支持 `table`/`json` 输出。
- `itb s3 upload` 新增 `--skip-existing` 与 `--skip-unchanged`（互斥）：分别按"对象键已存在"与"远端 `x-amz-meta-itb-sha256` 与本地 SHA-256 一致"跳过上传，以 1 次 HEAD 代替整文件传输；默认行为仍为无条件覆盖。
- 根命令支持 `itb --version` / `-v` 与未知命令名的纠错建议（Suggest），`s3` 子命令同步开启。
- 发布工作流在发布前于 CI 执行 `make check` 与 `make test`，并将发布归档同步上传到 WebDAV。

### Changed

- **BREAKING:** compress 未指定 `--output` 时不再覆盖原文件，改为输出 `*_compressed.*`；需要覆盖时显式传入新增的 `--in-place`（与 `--output` 互斥）。
- **BREAKING:** CLI 框架由 `spf13/cobra` 迁移至 `urfave/cli/v3`，命令树在 `cmd.New()` 中显式构造，移除包级 flag 全局变量与 `init()` 注册；用户可见命令与 `itb version` 输出保持不变。
- 参数校验前移至 CLI 层：枚举（resize/crop/convert/watermark/inspect/s3）、范围（quality、opacity、尺寸等）与百分比参数在文件 IO 之前报错；flag Usage 统一 `FILE`/`FORMAT`/`MODE` 等占位符。
- compress 编排逻辑下沉到 `internal/compress` 领域包，供 CLI 与 Web API 复用。
- S3 上传在 skip 模式下 HEAD 前置并只打开一次文件，减少本地 IO。

### Removed

- **BREAKING:** 移除 `itb metadata` 别名，请使用 `itb inspect`；`inspect` 新增 `--no-detail` 关闭详细元数据（优先于 `--detail`）。
- 完全移除批处理能力：`itb batch` CLI、`internal/batch` 包、`/api/v1/batch/*` HTTP API 及前端残留的 Batch 组件与 API client。
- 完全移除 LskyPro 集成：`itb lsky` CLI、`internal/lsky` 包、`ITB_LSKY_*` 环境变量，以及 WebUI 的 `/api/v1/lsky/images` 上传接口、前端 `LskyPanel` 与整个 `web/src/storage/` 目录。
- WebUI 移除 S3 面板与 `/api/v1/s3/*` 接口；WebUI 收敛为图像处理功能（压缩/缩放/裁剪/转换/水印）。`itb s3` CLI 与 `ITB_S3_*` 环境变量不受影响。

### Fixed

- S3 配置来源统一到 CLI 层：`ITB_S3_*` 环境变量改由 urfave/cli `Sources` 绑定，`ITB_S3_BUCKET` 等环境变量现在能真正满足 `--bucket` 等 required flag 校验（此前因 Action 前置校验而失效），优先级为 CLI flag > 环境变量 > 默认值；`internal/s3` 不再读取环境变量，收敛为纯领域包。
- S3 HTTP 客户端移除 30s 总超时（会截断大文件上传/下载），改为 Transport 层 `ResponseHeaderTimeout=30s`，连接/TLS/连接池继承标准库默认值。
- 修正 CLI help 漂移：watermark 移除未实现的 `--tile` flag，resize/crop 补充尺寸与锚点参数组合规则，s3 必填项与默认值描述与实际行为一致。
- WebUI 修复中文下载文件名乱码。

## [v0.4.1] - 2026-08-13

### Added

- 发布工作流会在 GitHub Release 完成后，将四个平台的校验和同步为 `lz-wang/homebrew-tap` 中的 `itb` Formula；Formula 使用发布版本，且审计、安装和测试通过后才会推送。

## [v0.4.0] - 2026-07-20

### Added

- 新增 `inspect` 子命令：查看文件信息、图像元数据（宽高/格式/色彩模型）与文件 hash，支持 `table`/`json`/`plain` 三种输出格式，默认只读不解码像素。

### Changed

- **BREAKING:** 环境变量统一为 `ITB_` 前缀：`S3_*` → `ITB_S3_*`、`LSKY_*` → `ITB_LSKY_*`，旧名不再被读取。
- 文档与帮助文本不再宣传 `AWS_*` 环境变量（代码此前从未读取，属 dead config）。
- 修复 `--region` 默认值导致 `ITB_S3_REGION` 失效的问题，环境变量现在可正常覆盖区域。
- 统一根命令与所有子命令帮助文本中的命令名为 `itb`，禁用 cobra 默认 `completion` 子命令，并精简与短格式重复的描述。

## [v0.3.0] - 2026-04-22

### Added

- Added `resize` command for image scaling with percentage and explicit width/height controls.
- Added `convert` command for converting images between `jpg/jpeg/png/webp`.
- Added text and image watermark support, including tiled and positioned watermark modes.
- Added `batch` command for batch image processing across directories.
- Added S3-compatible object storage and LskyPro upload commands.

### Changed

- Switched WebP encoding from `github.com/chai2010/webp` to the pure Go `github.com/deepteams/webp` implementation.
- Restored `CGO_ENABLED=0` compatibility for build and release workflows across all target platforms.
- Expanded and refined README coverage for image processing and upload commands.

## [v0.2.0] - 2026-04-22

### Added

- Added `crop` command with anchor-based percentage cropping for `left`, `right`, `top`, `bottom`, corners, and `center`.
- Added Windows release artifacts for both `amd64` and `arm64`.
- Added Pushover notifications for CI and release workflow completion.

### Changed

- Extended build and release workflows to produce bundled binaries for macOS, Linux, and Windows in parallel.
- Updated GitHub Actions dependencies to current major versions and enabled Node.js 24 preflight mode.
- Improved Windows runner dependency setup to prefer preinstalled `cmake` and `nasm`, avoiding ARM64 Chocolatey install failures.

## [v0.1.0] - 2026-04-07

- Initial tagged release.
