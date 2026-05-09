package inspect

import (
	"fmt"
	"io"
)

func PrintTable(w io.Writer, result *Result) error {
	fmt.Fprintf(w, "File\n")
	fmt.Fprintf(w, "  Path:        %s\n", result.File.Path)
	fmt.Fprintf(w, "  Size:        %d bytes\n", result.File.SizeBytes)
	fmt.Fprintf(w, "  MIME:        %s\n", result.File.MIMEType)
	fmt.Fprintf(w, "  Modified:    %s\n", result.File.ModifiedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "  Magic:       %s\n", result.File.MagicHex)

	if result.Image != nil {
		fmt.Fprintf(w, "\nImage\n")
		fmt.Fprintf(w, "  Format:      %s\n", result.Image.Format)
		fmt.Fprintf(w, "  Width:       %d\n", result.Image.Width)
		fmt.Fprintf(w, "  Height:      %d\n", result.Image.Height)
		fmt.Fprintf(w, "  Ratio:       %s\n", result.Image.AspectRatio)
		fmt.Fprintf(w, "  Megapixels:  %.4f\n", result.Image.Megapixels)
		fmt.Fprintf(w, "  ColorModel:  %s\n", result.Image.ColorModel)
		fmt.Fprintf(w, "  Alpha:       %t\n", result.Image.HasAlpha)
	}

	if result.Detail != nil {
		fmt.Fprintf(w, "\nDetail\n")
		fmt.Fprintf(w, "  MagicBytes:  %s\n", result.Detail.MagicBytes)
		fmt.Fprintf(w, "  HeaderBytes: %s\n", result.Detail.HeaderBytes)
		fmt.Fprintf(w, "  DetectedBy:  %s\n", result.Detail.DetectedBy)
		fmt.Fprintf(w, "  ExtMatch:    %t\n", result.Detail.ExtensionMatchesFormat)
	}

	if result.Hashes != nil {
		fmt.Fprintf(w, "\nHashes\n")
		fmt.Fprintf(w, "  SHA256:      %s\n", result.Hashes.SHA256)
		fmt.Fprintf(w, "  SHA1:        %s\n", result.Hashes.SHA1)
		fmt.Fprintf(w, "  MD5:         %s\n", result.Hashes.MD5)
		fmt.Fprintf(w, "  CRC32:       %s\n", result.Hashes.CRC32)
	}

	if result.Error != nil {
		fmt.Fprintf(w, "\nError\n")
		fmt.Fprintf(w, "  Code:        %s\n", result.Error.Code)
		fmt.Fprintf(w, "  Message:     %s\n", result.Error.Message)
	}

	return nil
}
