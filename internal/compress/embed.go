package compress

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// BinaryType 定义二进制文件类型
type BinaryType string

const (
	PngQuant BinaryType = "pngquant"
	OxiPng   BinaryType = "oxipng"
	DJpeg    BinaryType = "djpeg"
	CJpeg    BinaryType = "cjpeg"
)

// binaryPaths 定义不同平台的二进制文件路径
var binaryPaths = map[string]map[BinaryType]string{
	"darwin-amd64": {
		PngQuant: "bins/macos-amd64/pngquant",
		OxiPng:   "bins/macos-amd64/oxipng",
		DJpeg:    "bins/macos-amd64/djpeg-static",
		CJpeg:    "bins/macos-amd64/cjpeg-static",
	},
	"darwin-arm64": {
		PngQuant: "bins/macos-arm64/pngquant",
		OxiPng:   "bins/macos-arm64/oxipng",
		DJpeg:    "bins/macos-arm64/djpeg-static",
		CJpeg:    "bins/macos-arm64/cjpeg-static",
	},
	"linux-amd64": {
		PngQuant: "bins/linux-amd64/pngquant",
		OxiPng:   "bins/linux-amd64/oxipng",
		DJpeg:    "bins/linux-amd64/djpeg-static",
		CJpeg:    "bins/linux-amd64/cjpeg-static",
	},
	"linux-arm64": {
		PngQuant: "bins/linux-arm64/pngquant",
		OxiPng:   "bins/linux-arm64/oxipng",
		DJpeg:    "bins/linux-arm64/djpeg-static",
		CJpeg:    "bins/linux-arm64/cjpeg-static",
	},
}

var (
	binariesFS     embed.FS
	extractedPaths = make(map[BinaryType]string)
	extractMutex   sync.Once
	extractError   error
)

// InitBinaries 初始化二进制文件（从 main.go 调用）
func InitBinaries(fs embed.FS) {
	binariesFS = fs
}

// getPlatformKey 获取当前平台的 key
func getPlatformKey() string {
	return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
}

// EnsureBinary 确保二进制文件可用，返回临时文件路径
func EnsureBinary(binType BinaryType) (string, error) {
	extractMutex.Do(func() {
		extractError = extractAllBinaries()
	})
	if extractError != nil {
		return "", extractError
	}

	path, ok := extractedPaths[binType]
	if !ok {
		return "", fmt.Errorf("binary %s not found", binType)
	}
	return path, nil
}

// extractAllBinaries 提取所有二进制文件到临时目录
func extractAllBinaries() error {
	platformKey := getPlatformKey()
	paths, ok := binaryPaths[platformKey]
	if !ok {
		return fmt.Errorf("unsupported platform: %s", platformKey)
	}

	// 创建临时目录
	tmpDir := filepath.Join(os.TempDir(), "img-compress-bins")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return err
	}

	for binType, relPath := range paths {
		data, err := binariesFS.ReadFile(relPath)
		if err != nil {
			return fmt.Errorf("failed to read embedded binary %s: %w", binType, err)
		}

		targetPath := filepath.Join(tmpDir, string(binType))

		// 检查是否已存在且大小相同
		if info, err := os.Stat(targetPath); err == nil {
			if int(info.Size()) == len(data) {
				extractedPaths[binType] = targetPath
				continue
			}
		}

		if err := os.WriteFile(targetPath, data, 0755); err != nil {
			return fmt.Errorf("failed to write binary %s: %w", binType, err)
		}

		extractedPaths[binType] = targetPath
	}

	return nil
}
