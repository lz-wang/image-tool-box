package inspect

import "time"

const SchemaVersion = "itb.inspect.v1"

type Options struct {
	Detail bool
	NoHash bool
	Strict bool
}

type Result struct {
	SchemaVersion string      `json:"schema_version"`
	File          FileInfo    `json:"file"`
	Image         *ImageInfo  `json:"image,omitempty"`
	Detail        *DetailInfo `json:"detail,omitempty"`
	Hashes        *HashInfo   `json:"hashes,omitempty"`
	Warnings      []string    `json:"warnings,omitempty"`
	Error         *InfoError  `json:"error,omitempty"`
}

type FileInfo struct {
	Path       string    `json:"path"`
	AbsPath    string    `json:"abs_path,omitempty"`
	Name       string    `json:"name"`
	Ext        string    `json:"ext"`
	SizeBytes  int64     `json:"size_bytes"`
	Mode       string    `json:"mode"`
	ModifiedAt time.Time `json:"modified_at"`
	MIMEType   string    `json:"mime_type,omitempty"`
	MagicHex   string    `json:"magic_hex,omitempty"`
}

type ImageInfo struct {
	Format         string  `json:"format"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	AspectRatio    string  `json:"aspect_ratio"`
	Megapixels     float64 `json:"megapixels"`
	ColorModel     string  `json:"color_model,omitempty"`
	HasAlpha       bool    `json:"has_alpha"`
	Animated       bool    `json:"animated"`
	DecodeConfigOK bool    `json:"decode_config_ok"`
}

type DetailInfo struct {
	MagicBytes             string `json:"magic_bytes,omitempty"`
	HeaderBytes            string `json:"header_bytes,omitempty"`
	DetectedBy             string `json:"detected_by,omitempty"`
	ExtensionMatchesFormat bool   `json:"extension_matches_format"`
}

type HashInfo struct {
	SHA256 string `json:"sha256"`
	SHA1   string `json:"sha1"`
	MD5    string `json:"md5"`
	CRC32  string `json:"crc32"`
}

type InfoError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
