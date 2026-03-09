package watermark

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Position defines the watermark position.
type Position string

const (
	BottomRight Position = "bottom-right"
	BottomLeft  Position = "bottom-left"
	TopRight    Position = "top-right"
	TopLeft     Position = "top-left"
	Center      Position = "center"
)

// RepeatOptions options for repeat watermark mode.
type RepeatOptions struct {
	Color          *string
	Space          *int
	Angle          *int
	Opacity        *float64
	FontPath       string
	FontSize       *int
	FontHeightCrop *float64
}

// PositionOptions options for position watermark mode.
type PositionOptions struct {
	Opacity       *float64
	Position      Position
	FontPath      string
	FontSize      *int
	MarginRatio   *float64
	JPGBackground *color.NRGBA
}

// WatermarkArgs configuration for watermark generation.
type WatermarkArgs struct {
	Mark           string
	Color          string
	Space          int
	Angle          int
	FontFamily     string
	FontHeightCrop float64
	Size           int
	Opacity        float64
}

// Watermarker provides watermark generation and application.
type Watermarker struct {
	args    WatermarkArgs
	markImg image.Image
}

// NewWatermarker creates a Watermarker and pre-generates the mark tile image.
func NewWatermarker(args WatermarkArgs) (*Watermarker, error) {
	if strings.TrimSpace(args.Mark) == "" {
		return nil, errors.New("args.Mark must not be empty")
	}
	wm := &Watermarker{args: args}
	mark, err := wm.generateMark()
	if err != nil {
		return nil, err
	}
	wm.markImg = mark
	if wm.markImg == nil {
		log.Printf("generated mark image is empty; check mark text and font path")
	}
	return wm, nil
}

// Apply overlays the repeated watermark onto the image.
func (w *Watermarker) Apply(im image.Image) (image.Image, error) {
	if w.markImg == nil {
		return nil, errors.New("mark image not generated")
	}

	base := imaging.Clone(im)
	bw := base.Bounds().Dx()
	bh := base.Bounds().Dy()

	mw := w.markImg.Bounds().Dx()
	mh := w.markImg.Bounds().Dy()

	c := int(math.Hypot(float64(bw), float64(bh))) + max(mw, mh)*2
	tiled := image.NewNRGBA(image.Rect(0, 0, c, c))

	y := 0
	rowShift := 0
	for y < c {
		x := -int(float64(mw+w.args.Space) * 0.5 * float64(rowShift))
		rowShift ^= 1
		for x < c {
			pasteWithAlpha(tiled, w.markImg, x, y)
			x += mw + w.args.Space
		}
		y += mh + w.args.Space
	}

	rotated := imaging.Rotate(tiled, float64(w.args.Angle), color.NRGBA{0, 0, 0, 0})

	overlay := image.NewNRGBA(image.Rect(0, 0, bw, bh))
	offX := (bw - rotated.Bounds().Dx()) / 2
	offY := (bh - rotated.Bounds().Dy()) / 2
	pasteWithAlpha(overlay, rotated, offX, offY)

	result := image.NewNRGBA(base.Bounds())
	draw.Draw(result, base.Bounds(), base, image.Point{}, draw.Src)
	draw.Draw(result, overlay.Bounds(), overlay, image.Point{}, draw.Over)

	if sameRGB(base, result) {
		log.Printf("result identical to source; watermark not visible (increase opacity or verify font)")
	}

	return result, nil
}

// SaveImage saves the image to disk with correct RGBA -> JPEG handling.
func SaveImage(img image.Image, path string, jpgBackground color.NRGBA) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lower := strings.ToLower(filepath.Ext(path))

	switch lower {
	case ".jpg", ".jpeg":
		flattened := flattenToRGB(img, jpgBackground)
		out, err := os.Create(path)
		if err != nil {
			return err
		}
		defer out.Close()
		return jpeg.Encode(out, flattened, &jpeg.Options{Quality: 100})
	case ".png":
		out, err := os.Create(path)
		if err != nil {
			return err
		}
		defer out.Close()
		return png.Encode(out, img)
	default:
		flattened := flattenToRGB(img, jpgBackground)
		return imaging.Save(flattened, path, imaging.JPEGQuality(100))
	}
}

// AddRepeatWatermark adds a repeated text watermark and saves the output.
func AddRepeatWatermark(inputPath, outputPath, text string, opts *RepeatOptions) (image.Image, error) {
	var colorVal = "#4db6ac"
	var space *int
	var angleVal = 30
	var opacityVal = 0.5
	var fontSize *int
	var fontHeightCropVal = 1.0
	var fontPath string

	if opts != nil {
		if opts.Color != nil {
			colorVal = *opts.Color
		}
		space = opts.Space
		if opts.Angle != nil {
			angleVal = *opts.Angle
		}
		if opts.Opacity != nil {
			opacityVal = *opts.Opacity
		}
		fontSize = opts.FontSize
		if opts.FontHeightCrop != nil {
			fontHeightCropVal = *opts.FontHeightCrop
		}
		fontPath = opts.FontPath
	}

	// 先打开图片以获取尺寸
	im, err := imaging.Open(inputPath)
	if err != nil {
		return nil, err
	}

	// 如果未指定字体大小，则自动计算
	var fontSizeVal int
	if fontSize != nil && *fontSize > 0 {
		fontSizeVal = *fontSize
	} else {
		width := im.Bounds().Dx()
		height := im.Bounds().Dy()
		fontSizeVal = max(min(width, height)/25, 16)
	}

	// 如果未指定间距，则根据字体大小自动计算
	var spaceVal int
	if space != nil && *space > 0 {
		spaceVal = *space
	} else {
		spaceVal = fontSizeVal * 2
	}

	args := WatermarkArgs{
		Mark:           text,
		Color:          colorVal,
		Space:          spaceVal,
		Angle:          angleVal,
		FontFamily:     fontPath,
		FontHeightCrop: fontHeightCropVal,
		Size:           fontSizeVal,
		Opacity:        opacityVal,
	}
	wm, err := NewWatermarker(args)
	if err != nil {
		return nil, err
	}
	marked, err := wm.Apply(im)
	if err != nil {
		return nil, err
	}
	if err := SaveImage(marked, outputPath, color.NRGBA{255, 255, 255, 255}); err != nil {
		return nil, err
	}
	return marked, nil
}

// AddPositionWatermark adds a single positioned watermark and saves the output.
func AddPositionWatermark(inputPath, outputPath, text string, opts *PositionOptions) (image.Image, error) {
	var opacityVal = 0.5
	var marginRatio = 0.04
	var fontPath string
	var pos Position = BottomRight
	var jpgBg color.NRGBA
	var fontSize *int

	if opts != nil {
		if opts.Opacity != nil {
			opacityVal = *opts.Opacity
		}
		if opts.MarginRatio != nil {
			marginRatio = *opts.MarginRatio
		}
		if opts.FontPath != "" {
			fontPath = opts.FontPath
		}
		if opts.Position != "" {
			pos = opts.Position
		}
		if opts.JPGBackground != nil {
			jpgBg = *opts.JPGBackground
		}
		fontSize = opts.FontSize
	}
	img, err := imaging.Open(inputPath)
	if err != nil {
		return nil, err
	}
	rgba := imaging.Clone(img)

	width := rgba.Bounds().Dx()
	height := rgba.Bounds().Dy()

	// 如果未指定字体大小，则自动计算
	var fontSizeVal int
	if fontSize != nil && *fontSize > 0 {
		fontSizeVal = *fontSize
	} else {
		fontSizeVal = max(min(width, height)/25, 16)
	}

	face, err := loadFontFaceWithFallback(fontPath, fontSizeVal)
	if err != nil {
		return nil, err
	}

	bounds, _ := font.BoundString(face, text)
	textW := fixedToInt(bounds.Max.X - bounds.Min.X)
	textH := fixedToInt(bounds.Max.Y - bounds.Min.Y)
	if textW <= 0 || textH <= 0 {
		return nil, errors.New("text bounds are empty")
	}

	sample := image.Rect(
		width/2-textW/2,
		height/2-textH/2,
		width/2+textW/2,
		height/2+textH/2,
	).Intersect(rgba.Bounds())
	if sample.Empty() {
		sample = rgba.Bounds()
	}

	brightness := meanRedChannel(rgba, sample)
	alpha := clampInt(int(math.Round(255*opacityVal)), 0, 255)
	outlineAlpha := clampInt(int(math.Round(255*opacityVal*0.6)), 0, 255)

	var fillColor, outlineColor color.NRGBA
	if brightness > 128 {
		fillColor = color.NRGBA{0, 0, 0, uint8(alpha)}
		outlineColor = color.NRGBA{255, 255, 255, uint8(outlineAlpha)}
	} else {
		fillColor = color.NRGBA{255, 255, 255, uint8(alpha)}
		outlineColor = color.NRGBA{0, 0, 0, uint8(outlineAlpha)}
	}

	marginW := int(float64(width) * marginRatio)
	marginH := int(float64(height) * marginRatio)

	positions := map[Position]image.Point{
		BottomRight: {X: width - textW - marginW, Y: height - textH - marginH},
		BottomLeft:  {X: marginW, Y: height - textH - marginH},
		TopRight:    {X: width - textW - marginW, Y: marginH},
		TopLeft:     {X: marginW, Y: marginH},
		Center:      {X: (width - textW) / 2, Y: (height - textH) / 2},
	}

	chosen, ok := positions[pos]
	if !ok {
		chosen = positions[BottomRight]
	}

	drawTextOutlined(rgba, face, chosen.X, chosen.Y, text, fillColor, outlineColor, 2)

	if jpgBg == (color.NRGBA{}) {
		jpgBg = color.NRGBA{255, 255, 255, 255}
	}
	if err := SaveImage(rgba, outputPath, jpgBg); err != nil {
		return nil, err
	}

	return rgba, nil
}

func (w *Watermarker) generateMark() (image.Image, error) {
	face, err := loadFontFaceWithFallback(w.args.FontFamily, w.args.Size)
	if err != nil {
		return nil, err
	}
	colorVal, err := parseHexColor(w.args.Color)
	if err != nil {
		return nil, err
	}

	markRunes := []rune(w.args.Mark)
	tmpW := max(200, w.args.Size*max(4, len(markRunes)))
	tmpH := max(64, int(float64(w.args.Size)*2.5))
	canvas := image.NewNRGBA(image.Rect(0, 0, tmpW, tmpH))

	d := &font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(colorVal),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.I(0),
			Y: fixed.I(0) + face.Metrics().Ascent,
		},
	}
	d.DrawString(w.args.Mark)

	bbox, ok := tightAlphaBounds(canvas)
	if !ok {
		return nil, nil
	}
	mark := imaging.Crop(canvas, bbox)

	hcrop := w.args.FontHeightCrop
	if hcrop > 0 && hcrop != 1.0 {
		newH := int(math.Max(1, math.Round(float64(w.args.Size)*hcrop)))
		mark = imaging.Resize(mark, mark.Bounds().Dx(), newH, imaging.Lanczos)
	}

	return setOpacity(mark, w.args.Opacity)
}

func loadFontFace(path string, size int) (font.Face, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("font path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fnt, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(fnt, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func loadFontFaceWithFallback(path string, size int) (font.Face, error) {
	if strings.TrimSpace(path) != "" {
		face, err := loadFontFace(path, size)
		if err == nil {
			return face, nil
		}
		log.Printf("failed to load font %q, falling back to Go Regular: %v", path, err)
	}
	if strings.TrimSpace(path) == "" {
		if arial := firstExistingFontPath([]string{
			"arial.ttf",
			"/Library/Fonts/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"C:\\Windows\\Fonts\\arial.ttf",
			"/usr/share/fonts/truetype/msttcorefonts/Arial.ttf",
			"/usr/share/fonts/truetype/msttcorefonts/arial.ttf",
		}); arial != "" {
			face, err := loadFontFace(arial, size)
			if err == nil {
				return face, nil
			}
			log.Printf("failed to load fallback Arial font %q, using Go Regular: %v", arial, err)
		}
	}
	fnt, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(fnt, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func firstExistingFontPath(candidates []string) string {
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func parseHexColor(s string) (color.NRGBA, error) {
	str := strings.TrimSpace(s)
	if str == "" {
		return color.NRGBA{}, errors.New("color must not be empty")
	}
	str = strings.TrimPrefix(str, "#")
	switch len(str) {
	case 3:
		str = fmt.Sprintf("%c%c%c%c%c%c", str[0], str[0], str[1], str[1], str[2], str[2])
	case 6, 8:
	default:
		return color.NRGBA{}, fmt.Errorf("invalid color format: %q", s)
	}

	var r, g, b, a uint8
	hexRGB := str
	if len(str) == 8 {
		hexRGB = str[:6]
	}
	_, err := fmt.Sscanf(hexRGB, "%02x%02x%02x", &r, &g, &b)
	if err != nil {
		return color.NRGBA{}, err
	}
	if len(str) == 8 {
		_, err = fmt.Sscanf(str[6:], "%02x", &a)
		if err != nil {
			return color.NRGBA{}, err
		}
	} else {
		a = 255
	}

	return color.NRGBA{R: r, G: g, B: b, A: a}, nil
}

func setOpacity(img image.Image, opacity float64) (image.Image, error) {
	if opacity < 0 || opacity > 1 {
		return nil, errors.New("opacity must be between 0 and 1")
	}
	out := imaging.Clone(img)
	for i := 0; i < len(out.Pix); i += 4 {
		out.Pix[i+3] = uint8(math.Round(float64(out.Pix[i+3]) * opacity))
	}
	return out, nil
}

func tightAlphaBounds(img *image.NRGBA) (image.Rectangle, bool) {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A != 0 {
				if !found {
					minX, minY = x, y
					maxX, maxY = x, y
					found = true
				} else {
					if x < minX {
						minX = x
					}
					if y < minY {
						minY = y
					}
					if x > maxX {
						maxX = x
					}
					if y > maxY {
						maxY = y
					}
				}
			}
		}
	}
	if !found {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

func pasteWithAlpha(dst *image.NRGBA, src image.Image, x, y int) {
	r := image.Rect(x, y, x+src.Bounds().Dx(), y+src.Bounds().Dy())
	draw.DrawMask(dst, r, src, src.Bounds().Min, src, src.Bounds().Min, draw.Over)
}

func flattenToRGB(img image.Image, bg color.NRGBA) image.Image {
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, &image.Uniform{C: bg}, image.Point{}, draw.Src)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Over)
	return rgba
}

func meanRedChannel(img *image.NRGBA, r image.Rectangle) float64 {
	var sum uint64
	var count uint64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			pix := img.NRGBAAt(x, y)
			sum += uint64(pix.R)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

func drawTextOutlined(dst *image.NRGBA, face font.Face, x, y int, text string, fill, outline color.NRGBA, outlineRange int) {
	for dx := -outlineRange; dx <= outlineRange; dx++ {
		for dy := -outlineRange; dy <= outlineRange; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			drawTextAt(dst, face, x+dx, y+dy, text, outline)
		}
	}
	drawTextAt(dst, face, x, y, text, fill)
}

func drawTextAt(dst *image.NRGBA, face font.Face, x, y int, text string, col color.NRGBA) {
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.I(x),
			Y: fixed.I(y) + face.Metrics().Ascent,
		},
	}
	d.DrawString(text)
}

func sameRGB(a, b image.Image) bool {
	ab := a.Bounds()
	bb := b.Bounds()
	if ab.Dx() != bb.Dx() || ab.Dy() != bb.Dy() {
		return false
	}
	for y := 0; y < ab.Dy(); y++ {
		for x := 0; x < ab.Dx(); x++ {
			ar, ag, abv, _ := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			br, bg, bbv, _ := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			if ar != br || ag != bg || abv != bbv {
				return false
			}
		}
	}
	return true
}

func fixedToInt(v fixed.Int26_6) int {
	return int(math.Ceil(float64(v) / 64.0))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
