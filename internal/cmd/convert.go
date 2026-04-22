package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"imagetoolbox/internal/convert"
)

var (
	convertInputFile  string
	convertOutputFile string
	convertTo         string
	convertQuality    int
	convertLossless   bool
	convertBackground string
)

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "转换图片格式",
	Example: `  imagetoolbox convert -i photo.png --to webp
  imagetoolbox convert -i photo.png --to jpg --background "#FFFFFF"
  imagetoolbox convert -i photo.jpg --to png -o converted.png`,
	RunE: runConvert,
}

func init() {
	rootCmd.AddCommand(convertCmd)

	convertCmd.Flags().StringVarP(&convertInputFile, "input", "i", "", "输入图片文件路径")
	convertCmd.Flags().StringVarP(&convertOutputFile, "output", "o", "", "输出图片文件路径")
	convertCmd.Flags().StringVar(&convertTo, "to", "", "目标格式: jpg/jpeg/png/webp")
	convertCmd.Flags().IntVarP(&convertQuality, "quality", "q", 80, "输出质量 (1-100)")
	convertCmd.Flags().BoolVar(&convertLossless, "lossless", false, "使用无损编码（webp/png）")
	convertCmd.Flags().StringVar(&convertBackground, "background", "#FFFFFF", "透明图转不透明格式时的背景色")
	convertCmd.MarkFlagRequired("input")
	convertCmd.MarkFlagRequired("to")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if convertInputFile == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}
	if convertTo == "" {
		return fmt.Errorf("必须指定目标格式 (--to)")
	}

	outputPath := convertOutputFile
	if outputPath == "" {
		var err error
		outputPath, err = convert.DefaultOutputPath(convertInputFile, convertTo)
		if err != nil {
			return err
		}
	}

	if err := convert.ConvertFile(convertInputFile, outputPath, convert.Options{
		To:         convertTo,
		Quality:    convertQuality,
		Lossless:   convertLossless,
		Background: convertBackground,
	}); err != nil {
		return fmt.Errorf("转换失败: %w", err)
	}

	fmt.Printf("转换完成: %s\n", outputPath)
	return nil
}
