package compress

import (
	"io/fs"

	"imagetoolbox/internal/nativebin"
)

// BinaryType and these wrappers preserve the compression package API.
type BinaryType = nativebin.ID

const (
	PngQuant = nativebin.PNGQuant
	OxiPng   = nativebin.OxiPNG
	DJpeg    = nativebin.DJPEG
	CJpeg    = nativebin.CJPEG
)

func InitBinaries(source fs.FS)                  { nativebin.Init(source) }
func EnsureBinary(id BinaryType) (string, error) { return nativebin.Ensure(id) }
