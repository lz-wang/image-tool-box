package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"imagetoolbox/internal/resize"
)

var (
	resizeInputFile  string
	resizeOutputFile string
	resizeWidth      int
	resizeHeight     int
	resizePercent    string
	resizeMode       string
	resizeAnchor     string
	resizeFilter     string
)

var resizeCmd = &cobra.Command{
	Use:   "resize",
	Short: "调整图片尺寸",
	Example: `  imagetoolbox resize -i photo.jpg --width 1200
  imagetoolbox resize -i photo.png --height 800
  imagetoolbox resize -i photo.jpg --percent 50%
  imagetoolbox resize -i photo.jpg --width 1200 --height 630 --mode fill --anchor top`,
	RunE: runResize,
}

func init() {
	rootCmd.AddCommand(resizeCmd)

	resizeCmd.Flags().StringVarP(&resizeInputFile, "input", "i", "", "输入图片文件路径")
	resizeCmd.Flags().StringVarP(&resizeOutputFile, "output", "o", "", "输出图片文件路径（默认在原文件名后加 _resized）")
	resizeCmd.Flags().IntVar(&resizeWidth, "width", 0, "目标宽度")
	resizeCmd.Flags().IntVar(&resizeHeight, "height", 0, "目标高度")
	resizeCmd.Flags().StringVar(&resizePercent, "percent", "", "按比例缩放，例如 50%")
	resizeCmd.Flags().StringVar(&resizeMode, "mode", "fit", "缩放模式: fit/fill/stretch")
	resizeCmd.Flags().StringVar(&resizeAnchor, "anchor", "center", "填充模式锚点: left/right/top/bottom/top-left/top-right/bottom-left/bottom-right/center")
	resizeCmd.Flags().StringVar(&resizeFilter, "filter", "lanczos", "采样器: nearest/linear/catmullrom/lanczos")
	resizeCmd.MarkFlagRequired("input")
}

func runResize(cmd *cobra.Command, args []string) error {
	if resizeInputFile == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}

	outputPath := resizeOutputFile
	if outputPath == "" {
		ext := filepath.Ext(resizeInputFile)
		base := strings.TrimSuffix(filepath.Base(resizeInputFile), ext)
		dir := filepath.Dir(resizeInputFile)
		outputPath = filepath.Join(dir, base+"_resized"+ext)
	}

	err := resize.ResizeFile(resizeInputFile, outputPath, resize.Options{
		Width:   resizeWidth,
		Height:  resizeHeight,
		Percent: resizePercent,
		Mode:    resize.Mode(resizeMode),
		Anchor:  resizeAnchor,
		Filter:  resizeFilter,
	})
	if err != nil {
		return fmt.Errorf("调整尺寸失败: %w", err)
	}

	fmt.Printf("调整尺寸完成: %s\n", outputPath)
	return nil
}
