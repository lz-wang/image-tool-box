package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"imagetoolbox/internal/inspect"
)

var (
	inspectInput  string
	inspectFormat string
	inspectDetail bool
	inspectNoHash bool
	inspectStrict bool
)

var inspectCmd = &cobra.Command{
	Use:     "inspect",
	Aliases: []string{"metadata"},
	Short:   "检查图片元数据和文件 hash",
	Long: `检查本地图片文件的文件信息、图像基本信息、详细元数据和文件 hash。

该命令为只读操作，不会修改原始图片。`,
	Example: `  imagetoolbox inspect -i photo.jpg
  imagetoolbox inspect -i photo.jpg --format json
  imagetoolbox inspect -i photo.jpg --format plain
  imagetoolbox inspect -i photo.jpg --detail=false
  imagetoolbox inspect -i photo.jpg --no-hash`,
	RunE: runInspect,
}

func init() {
	rootCmd.AddCommand(inspectCmd)

	inspectCmd.Flags().StringVarP(&inspectInput, "input", "i", "", "输入图片文件路径")
	inspectCmd.Flags().StringVar(&inspectFormat, "format", "table", "输出格式: table/json/plain")
	inspectCmd.Flags().BoolVar(&inspectDetail, "detail", true, "输出详细元数据")
	inspectCmd.Flags().BoolVar(&inspectNoHash, "no-hash", false, "不计算文件 hash")
	inspectCmd.Flags().BoolVar(&inspectStrict, "strict", false, "图像解析失败时直接返回错误")

	_ = inspectCmd.MarkFlagRequired("input")
}

func runInspect(cmd *cobra.Command, args []string) error {
	if inspectInput == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}

	result, err := inspect.File(inspectInput, inspect.Options{
		Detail: inspectDetail,
		NoHash: inspectNoHash,
		Strict: inspectStrict,
	})
	if err != nil {
		return err
	}

	switch inspectFormat {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)

	case "plain":
		if result.Hashes == nil || result.Hashes.SHA256 == "" {
			return fmt.Errorf("plain 输出需要 sha256；请移除 --no-hash")
		}
		fmt.Fprintln(os.Stdout, result.Hashes.SHA256)
		return nil

	case "table":
		return inspect.PrintTable(os.Stdout, result)

	default:
		return fmt.Errorf("不支持的输出格式: %s（支持: table, json, plain）", inspectFormat)
	}
}
