package nativebin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type binaryState struct {
	once sync.Once
	path string
	err  error
}

var (
	binariesMu sync.Mutex
	binariesFS fs.FS
	states     = newBinaryStates()

	cacheBaseDir = defaultCacheBaseDir
)

func newBinaryStates() map[ID]*binaryState {
	return map[ID]*binaryState{PNGQuant: {}, OxiPNG: {}, DJPEG: {}, CJPEG: {}, ZXingReader: {}}
}

// Init 初始化二进制文件（从 main.go 调用，传入 //go:embed bins/** 的 embed.FS）。
// 参数为 fs.FS 以便测试注入其他文件系统。调用它会丢弃当前进程中已缓存的二进制路径。
func Init(source fs.FS) {
	binariesMu.Lock()
	defer binariesMu.Unlock()
	binariesFS = source
	states = newBinaryStates()
}

// getPlatformKey 获取当前平台的 key。
func getPlatformKey() string { return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH) }

func extractedBinaryName(binType ID) string {
	if runtime.GOOS == "windows" {
		return string(binType) + ".exe"
	}
	return string(binType)
}

// Ensure 确保指定二进制文件可用，返回内容寻址缓存中的可执行文件路径。
func Ensure(binType ID) (string, error) {
	binariesMu.Lock()
	state, ok := states[binType]
	binariesMu.Unlock()
	if !ok {
		return "", fmt.Errorf("unknown binary: %s", binType)
	}
	state.once.Do(func() { state.path, state.err = extractBinary(binType) })
	if state.err != nil {
		return "", state.err
	}
	return state.path, nil
}

func extractBinary(binType ID) (string, error) {
	platformKey := getPlatformKey()
	paths, ok := binaryPaths[platformKey]
	if !ok {
		return "", fmt.Errorf("unsupported platform: %s", platformKey)
	}
	relPath, ok := paths[binType]
	if !ok {
		return "", fmt.Errorf("binary %s is not available for %s", binType, platformKey)
	}
	binariesMu.Lock()
	source := binariesFS
	binariesMu.Unlock()
	if source == nil {
		return "", fmt.Errorf("embedded binaries are not initialized")
	}
	data, err := fs.ReadFile(source, relPath)
	if err != nil {
		return "", fmt.Errorf("read embedded binary %s: %w", binType, err)
	}

	sum := sha256.Sum256(data)
	cacheDir, err := cacheBaseDir()
	if err != nil {
		return "", fmt.Errorf("resolve binary cache directory: %w", err)
	}
	targetDir := filepath.Join(cacheDir, "itb", "bins", platformKey, hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("create binary cache directory: %w", err)
	}
	targetPath := filepath.Join(targetDir, extractedBinaryName(binType))
	match, err := fileMatchesHash(targetPath, sum)
	if err != nil {
		return "", fmt.Errorf("verify cached binary %s: %w", binType, err)
	}
	if match {
		return targetPath, nil
	}
	if err := writeAtomically(targetDir, targetPath, data); err != nil {
		return "", fmt.Errorf("extract binary %s: %w", binType, err)
	}
	return targetPath, nil
}
