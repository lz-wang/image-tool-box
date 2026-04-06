package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"imagetoolbox/internal/lsky"
)

var (
	lskyURL   string
	lskyToken string
)

var (
	lskyUploadInput      string
	lskyUploadStrategyID int
	lskyUploadLinkFormat string
)

var lskyCmd = &cobra.Command{
	Use:   "lsky",
	Short: "LskyPro 图床操作",
	Long: `LskyPro 图床操作，当前支持上传图片。

环境变量支持:
  LSKY_URL    LskyPro 服务地址（支持根地址或 /api/v1）
  LSKY_TOKEN  API Token`,
}

var lskyUploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "上传图片到 LskyPro",
	Long:  `上传本地图片到 LskyPro 图床。`,
	Example: `  # 使用环境变量上传
  imagetoolbox lsky upload -i photo.jpg

  # 显式指定地址和 Token
  imagetoolbox lsky upload -i photo.jpg --url https://img.example.com --token your-token

  # 指定存储策略
  imagetoolbox lsky upload -i photo.jpg --strategy 2

  # 输出 Markdown 链接
  imagetoolbox lsky upload -i photo.jpg --link-format markdown`,
	RunE: runLskyUpload,
}

func init() {
	rootCmd.AddCommand(lskyCmd)

	lskyCmd.PersistentFlags().StringVar(&lskyURL, "url", "", "LskyPro 服务地址（支持根地址或 /api/v1）")
	lskyCmd.PersistentFlags().StringVar(&lskyToken, "token", "", "LskyPro API Token（默认从环境变量读取）")

	lskyCmd.AddCommand(lskyUploadCmd)

	lskyUploadCmd.Flags().StringVarP(&lskyUploadInput, "input", "i", "", "本地图片路径")
	lskyUploadCmd.Flags().IntVarP(&lskyUploadStrategyID, "strategy", "s", 0, "存储策略 ID")
	lskyUploadCmd.Flags().StringVar(&lskyUploadLinkFormat, "link-format", "url", "快捷输出链接格式: url/markdown/bbcode/html/markdown-with-link/thumbnail")
	lskyUploadCmd.MarkFlagRequired("input")
}

func runLskyUpload(cmd *cobra.Command, args []string) error {
	if lskyUploadInput == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}

	client, err := lsky.NewClient(&lsky.Config{
		BaseURL: lskyURL,
		Token:   lskyToken,
	})
	if err != nil {
		return err
	}

	result, err := lsky.Upload(cmd.Context(), client, lskyUploadInput, &lsky.UploadOptions{
		StrategyID: lskyUploadStrategyID,
	})
	if err != nil {
		return err
	}

	fmt.Printf("上传完成: %s\n", lskyUploadInput)
	fmt.Printf("Key: %s\n", result.Data.Key)
	fmt.Printf("URL: %s\n", result.Data.Links.URL)
	if result.Data.Links.ThumbnailURL != "" {
		fmt.Printf("Thumbnail: %s\n", result.Data.Links.ThumbnailURL)
	}
	if result.Data.Mimetype != "" {
		fmt.Printf("MIME: %s\n", result.Data.Mimetype)
	}
	if result.Data.Width > 0 && result.Data.Height > 0 {
		fmt.Printf("尺寸: %dx%d\n", result.Data.Width, result.Data.Height)
	}
	if result.Data.Size > 0 {
		fmt.Printf("大小: %.2f KB\n", result.Data.Size)
	}

	quickLink := lsky.PickLink(result.Data.Links, lskyUploadLinkFormat)
	if quickLink != "" {
		fmt.Printf("快捷链接(%s): %s\n", lskyUploadLinkFormat, quickLink)
	}

	return nil
}
