package cmd

import (
	"github.com/spf13/cobra"
)

var (
	// Version 由 main 通过 Execute 传入
	version string
)

// rootCmd 根命令
var rootCmd = &cobra.Command{
	Use:   "imagetoolbox",
	Short: "图片处理工具箱",
	Long: `一个图片处理 CLI 工具箱，提供压缩、水印、S3 存储操作等功能。

功能:
  - compress: 图片压缩（PNG/JPEG），基于 pngquant、oxipng 和 libjpeg-turbo
  - watermark: 添加文字水印，支持位置和重复平铺两种模式
  - s3: S3 兼容存储操作（上传、下载、删除、列表）
  - lsky: 上传图片到 LskyPro 图床

所有依赖二进制已内嵌，无需外部依赖。`,
}

// Execute 执行根命令
func Execute(v string) error {
	version = v
	return rootCmd.Execute()
}
