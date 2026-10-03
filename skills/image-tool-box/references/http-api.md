# HTTP API

仅在需要通过 HTTP 集成图片处理或部署服务时读取。本地处理优先使用命令行。

## 启动与认证

先通过安全环境注入 `ITB_API_TOKEN`，再启动：

```bash
itb serve
# 指定本机回环端口
itb serve --addr 127.0.0.1:9000
```

默认监听 `127.0.0.1:8080`。本机开发且明确不需要认证时，可用 `itb serve --no-auth`；该选项仅允许回环地址。远程部署要求令牌与反向代理保护，监听范围按实际可信网络配置。按用户授权启动长期服务。

健康检查不需要认证：

```bash
curl --fail-with-body http://127.0.0.1:8080/api/v1/health
```

图片接口使用 `Authorization: Bearer $ITB_API_TOKEN`。令牌不要作为字面量写进命令或日志。

## 常见请求

```bash
curl --fail-with-body -X POST http://127.0.0.1:8080/api/v1/resize \
  -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@photo.jpg' -F 'width=1200' -F 'mode=fit' -o resized.jpg

curl --fail-with-body -X POST http://127.0.0.1:8080/api/v1/convert \
  -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@photo.jpg' -F 'to=webp' -F 'quality=85' -o converted.webp

curl --fail-with-body -X POST http://127.0.0.1:8080/api/v1/inspect \
  -H "Authorization: Bearer $ITB_API_TOKEN" -F 'input=@photo.jpg'
```

图片处理端点使用 `multipart/form-data`。`input` 是上传文件，选项通常沿用命令行长参数名，不带 `--`；HTTP `convert` 另用 `to` 指定目标格式。没有本地路径 `output` 或 `in-place` 参数。

图片结果是二进制，带 `Content-Disposition` 与 `X-ITB-*-Size` 响应头；检查和条码解码返回 JSON。保存响应后检查 HTTP 成功状态和实际文件类型，避免将错误响应当作图片。

## 条码接口

```bash
curl --fail-with-body -X POST http://127.0.0.1:8080/api/v1/barcode/generate \
  -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'symbology=qr' -F 'data=https://example.com' -o code.png

curl --fail-with-body -X POST http://127.0.0.1:8080/api/v1/barcode/decode \
  -H "Authorization: Bearer $ITB_API_TOKEN" \
  -F 'input=@code.png' -F 'symbology=qr,micro-qr,rmqr'
```

生成仅接受标量字段及生成选项，没有 `input` 文件；解码接受 `input` 和可选逗号分隔的 `symbology`。HTTP 不接受生成目标 `output`、覆盖开关 `force` 或结果格式 `format`。解码 `input.path` 是清洗后的客户端文件名，不泄漏临时目录。

## 资源限制与范围

| 参数 | 默认值 | 用途 |
|------|--------|------|
| `--addr` | `127.0.0.1:8080` | 监听地址 |
| `--max-upload` | `64MiB` | 整个 multipart 请求大小 |
| `--max-pixels` | `50000000` | 单图最大像素数 |
| `--max-dimension` | `16384` | 单边最大像素数 |
| `--max-concurrent` | `2` | 并发图片操作数 |
| `--max-working-bytes` | `512MiB` | 单操作工作内存上限 |
| `--timeout` | `2m` | 单操作超时 |
| `--no-auth` | `false` | 仅回环开发时关闭认证 |

支持 `/api/v1/compress`、`resize`、`crop`、`rotate`、`convert`、`watermark`、`inspect`、`barcode/generate`、`barcode/decode`，以及健康检查。API 不暴露 `compare` 或 S3 管理，也不提供 WebUI、用户系统、数据库、工作流或任务队列。

每个请求有独立临时目录，响应结束后清理。条码生成、解码和旋转等操作先进行尺寸、像素及工作集准入，再分配图像内存。图片参数约束见 [图片处理](image-operations.md)，码制与解码结果见 [二维码与条码](barcode.md)。
