package cmd

import (
	"github.com/urfave/cli/v3"
)

// 命令分类：root help 的 COMMANDS 部分按此分组展示。
const (
	categoryImageTransforms = "Image transforms"
	categoryAnalysis        = "Analysis"
	categoryStorage         = "Storage"
	categoryService         = "Service"
	categoryUtility         = "Utility"
)

// New 构造 itb 根命令。version 由 main 注入。
func New(version string) *cli.Command {
	return &cli.Command{
		Name:    "itb",
		Usage:   "Image processing and S3-compatible storage toolbox",
		Version: version,
		Suggest: true,
		// 命令清单由 urfave 根据 Commands 自动生成，
		// 禁止在 Description 中手写命令目录，避免与注册表漂移。
		Description: `Process and inspect images, compare image quality,
operate S3-compatible storage, or run the trusted HTTP API.

Use "itb <command> --help" for exact syntax, defaults,
constraints, and examples.`,
		Commands: []*cli.Command{
			newCompressCommand(),
			newResizeCommand(),
			newCropCommand(),
			newRotateCommand(),
			newConvertCommand(),
			newWatermarkCommand(),
			newCompareCommand(),
			newInspectCommand(),
			newS3Command(),
			newServeCommand(),
			newBarcodeCommand(),
			newVersionCommand(version),
		},
	}
}
