package lsky

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// UploadOptions 上传选项
type UploadOptions struct {
	StrategyID int
}

// Upload 上传文件到 LskyPro
func Upload(ctx context.Context, client *Client, inputPath string, opts *UploadOptions) (*UploadResponse, error) {
	if client == nil {
		return nil, fmt.Errorf("client 不能为空")
	}
	if inputPath == "" {
		return nil, fmt.Errorf("必须指定输入文件路径")
	}

	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("无法打开输入文件: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("无法读取输入文件信息: %w", err)
	}

	bodyReader, contentType, err := buildUploadBody(file, fileInfo.Name(), opts)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/upload", bodyReader)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+client.token)
	req.Header.Set("Content-Type", contentType)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("上传请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result UploadResponse
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if result.Message != "" {
			return nil, fmt.Errorf("上传失败（HTTP %d）: %s", resp.StatusCode, result.Message)
		}
		return nil, fmt.Errorf("上传失败（HTTP %d）", resp.StatusCode)
	}

	if !result.Status {
		if result.Message == "" {
			return nil, fmt.Errorf("上传失败")
		}
		return nil, fmt.Errorf("上传失败: %s", result.Message)
	}

	return &result, nil
}

func buildUploadBody(file *os.File, filename string, opts *UploadOptions) (io.Reader, string, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()

		part, err := writer.CreateFormFile("file", filepath.Base(filename))
		if err != nil {
			pw.CloseWithError(fmt.Errorf("创建文件表单失败: %w", err))
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(fmt.Errorf("写入上传文件失败: %w", err))
			return
		}

		if opts != nil && opts.StrategyID > 0 {
			if err := writer.WriteField("strategy_id", strconv.Itoa(opts.StrategyID)); err != nil {
				pw.CloseWithError(fmt.Errorf("写入 strategy_id 失败: %w", err))
				return
			}
		}
	}()

	return pr, writer.FormDataContentType(), nil
}

// PickLink 根据格式选择输出链接
func PickLink(links Links, format string) string {
	switch format {
	case "markdown":
		return links.Markdown
	case "bbcode":
		return links.BBCode
	case "html":
		return links.HTML
	case "markdown-with-link":
		return links.MarkdownWithLink
	case "thumbnail":
		return links.ThumbnailURL
	default:
		return links.URL
	}
}
