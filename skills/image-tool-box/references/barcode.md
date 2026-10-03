# 二维码与条码

仅在生成或解码二维码、条码时读取。先检查 `itb barcode generate --help` 或 `itb barcode decode --help`。

## 常见用法

```bash
itb barcode generate qr 'https://example.com' code.png --format json
itb barcode generate qr '你好，世界' code.png --error-correction H --module-size 8 --border 4
itb barcode generate code128 ABC123 label.png --module-width 2 --module-height 80
itb barcode generate ean13 690123456789 label.png --no-draw-text
itb barcode decode code.png --format json
itb barcode decode code.png --symbology qr --symbology micro-qr --symbology rmqr --format json
```

每条生成示例单独执行或使用不同目标；目标已存在时，只有明确要覆盖才加 `--force`。

## 码制与输入

| 码制 | 生成输入 | 解码 |
|------|----------|------|
| `qr` | UTF-8 文字，纠错等级 `L/M/Q/H` | 支持 |
| `micro-qr` / `rmqr` | 仅支持解码 | 支持 |
| `code128` | 1–80 个 ASCII 字符 | 支持 |
| `code39` | 大写 A–Z、0–9、空格及 `-.$/+%`，无校验位 | 支持 |
| `ean13` | 12 位数字，或校验位正确的完整 13 位 | 支持 |
| `ean8` | 7 位数字，或校验位正确的完整 8 位 | 支持 |

生成格式固定为 PNG，目的路径必须以 `.png` 结尾；不存在的父目录会自动创建。尺寸单位为整数像素或模块，不提供打印毫米、DPI 或系统字体行为。生成先写同目录临时文件，再原子提交；默认不覆盖，`--force` 原子替换，失败保留旧文件。

## 参数与尺寸

| 参数 | 默认值 | 用途 |
|------|--------|------|
| `--error-correction` | `M` | QR 纠错等级 |
| `--module-size` | `10` | QR 单模块边长，像素 |
| `--border` | `4` | QR 每边静区，模块数，可为 `0` |
| `--module-width` | `2` | 线性码最窄模块宽度，像素 |
| `--module-height` | `80` | 条高度，像素，不含文字和边距 |
| `--no-draw-text` | `false` | 隐藏线性码下方文字 |
| `--force` | `false` | 覆盖已有生成目标 |
| `--symbology` | 全部七码制 | 解码过滤器，可重复；生成的码制使用位置参数 |
| `--format` | `table` | 命令行输出 `table/json` |

QR 边长为 `(矩阵模块数 + 2 × border) × module-size`。线性码居中，左右至少各留十个模块静区，上下各留 10 像素；默认使用内置 `basicfont.Face7x13` 绘制完整文字，额外增加 20 像素高度。输出宽度取含静区的条码宽度与完整文字加左右各 10 像素留白的宽度最大值。生成工作集上限为 512 MiB。

## JSON 结果

生成契约为 `itb.barcode.generate.v1`，包含 `symbology`、`data`、`encoded_text`、`draw_text` 和 `output{path,format,width,height,size_bytes}`；QR 另含 `error_correction`、`module_size`、`border`。EAN 的 `data` 保留传入值，`encoded_text` 是含校验位的完整编码值。

解码契约为 `itb.barcode.decode.v1`：

```json
{
  "schema_version": "itb.barcode.decode.v1",
  "input": {"path": "code.png", "width": 290, "height": 290},
  "codes": [
    {
      "text": "你好",
      "symbology": "qr",
      "points": [{"x":40,"y":40},{"x":250,"y":40},{"x":250,"y":250},{"x":40,"y":250}]
    }
  ]
}
```

该 JSON 只说明字段形状，实际尺寸、文本和坐标由图片决定。

## 解码约束

- 输入为 JPEG/PNG/WebP，JPEG EXIF 方向先归一化，透明区域铺白再扫描。默认搜索全部七码制，可返回多个码。
- 坐标单位为像素，原点是归一化输入的左上角；四点顺序为左上、右上、右下、左下。
- 相同文本位于不同位置时保留多条结果，不按文字去重。
- 没有检出码是成功结果 `"codes": []`；损坏图片、非法码制和不可用解码器才失败。
- 输入最多 64 Mi 像素，单边最多 32768 像素。解码使用内嵌 ZXing-C++，不额外安装 Python、uv 或独立 reader。
- `--format json` 失败使用统一 `itb.error.v1`，标准输出只有一份文档。

需要 HTTP 调用时读取 [HTTP API](http-api.md#条码接口)。
