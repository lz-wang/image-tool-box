package barcode

import (
	"context"
	"fmt"
	"image"
	"image/color"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func render(ctx context.Context, p Plan) (*image.Gray, error) {
	if p.code == nil {
		return nil, fmt.Errorf("%w: uninitialized plan", ErrInvalidOptions)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// A CLI safeguard as well as the finer configurable HTTP admission.
	if p.WorkingBytes > 512<<20 {
		return nil, fmt.Errorf("%w: generation working set exceeds 512 MiB", ErrInvalidOptions)
	}
	img := image.NewGray(image.Rect(0, 0, p.Width, p.Height))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	sx, sy, ox, oy := p.opts.ModuleWidth, p.opts.ModuleHeight, 10*p.opts.ModuleWidth, 10
	if p.opts.Symbology == QRCode {
		sx = p.opts.ModuleSize
		sy = sx
		ox = p.opts.Border * sx
		oy = ox
	}
	bounds := p.code.Bounds()
	for y := 0; y < bounds.Dy(); y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < bounds.Dx(); x++ {
			if color.GrayModel.Convert(p.code.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.Gray).Y >= 128 {
				continue
			}
			for dy := 0; dy < sy; dy++ {
				for dx := 0; dx < sx; dx++ {
					img.SetGray(ox+x*sx+dx, oy+y*sy+dy, color.Gray{Y: 0})
				}
			}
		}
	}
	if p.opts.DrawText {
		d := font.Drawer{Dst: img, Src: image.Black, Face: basicfont.Face7x13}
		text := p.code.Content()
		width := d.MeasureString(text).Ceil()
		d.Dot = fixed.P((p.Width-width)/2, 10+p.opts.ModuleHeight+16)
		d.DrawString(text)
	}
	return img, nil
}
