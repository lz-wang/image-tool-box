package nativebin

// ID 定义二进制文件类型。
type ID string

const (
	PNGQuant ID = "pngquant"
	OxiPNG   ID = "oxipng"
	DJPEG    ID = "djpeg"
	CJPEG    ID = "cjpeg"
)

// binaryPaths 定义不同平台的二进制文件路径。
var binaryPaths = map[string]map[ID]string{
	"darwin-amd64": {
		PNGQuant: "bins/macos-amd64/pngquant", OxiPNG: "bins/macos-amd64/oxipng", DJPEG: "bins/macos-amd64/djpeg-static", CJPEG: "bins/macos-amd64/cjpeg-static",
	},
	"darwin-arm64": {
		PNGQuant: "bins/macos-arm64/pngquant", OxiPNG: "bins/macos-arm64/oxipng", DJPEG: "bins/macos-arm64/djpeg-static", CJPEG: "bins/macos-arm64/cjpeg-static",
	},
	"linux-amd64": {
		PNGQuant: "bins/linux-amd64/pngquant", OxiPNG: "bins/linux-amd64/oxipng", DJPEG: "bins/linux-amd64/djpeg-static", CJPEG: "bins/linux-amd64/cjpeg-static",
	},
	"linux-arm64": {
		PNGQuant: "bins/linux-arm64/pngquant", OxiPNG: "bins/linux-arm64/oxipng", DJPEG: "bins/linux-arm64/djpeg-static", CJPEG: "bins/linux-arm64/cjpeg-static",
	},
	"windows-amd64": {
		PNGQuant: "bins/windows-amd64/pngquant.exe", OxiPNG: "bins/windows-amd64/oxipng.exe", DJPEG: "bins/windows-amd64/djpeg-static.exe", CJPEG: "bins/windows-amd64/cjpeg-static.exe",
	},
	"windows-arm64": {
		PNGQuant: "bins/windows-arm64/pngquant.exe", OxiPNG: "bins/windows-arm64/oxipng.exe", DJPEG: "bins/windows-arm64/djpeg-static.exe", CJPEG: "bins/windows-arm64/cjpeg-static.exe",
	},
}
