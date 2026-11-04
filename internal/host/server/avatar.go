package server

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // register decoders
	_ "image/jpeg"
	"image/png"
	"math"
)

// maxAvatarPixels guards against decompression bombs (a tiny file that
// decodes to an enormous bitmap).
const maxAvatarPixels = 50_000_000

// ProcessAvatar decodes a PNG/JPEG/GIF, center-crops it to a square and
// resamples it to size×size, returning PNG bytes.
func ProcessAvatar(data []byte, size int) ([]byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("unsupported picture: use a PNG, JPEG or GIF")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxAvatarPixels {
		return nil, fmt.Errorf("picture is too large (%d×%d)", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("could not decode %s picture: %v", format, err)
	}
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	crop := image.Rect(0, 0, side, side)
	src := image.NewRGBA(crop) // premultiplied, so averaging doesn't fringe on transparency
	off := image.Pt(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2)
	draw.Draw(src, crop, img, off, draw.Src)

	var dst *image.RGBA
	if side >= size {
		dst = resampleArea(src, size)
	} else {
		dst = resampleBilinear(src, size)
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// resampleArea downsamples a square image with exact box (area-average) filtering.
func resampleArea(src *image.RGBA, size int) *image.RGBA {
	n := src.Bounds().Dx()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := float64(n) / float64(size)
	// Precompute per-axis coverage spans (identical for x and y since square).
	type span struct {
		lo, hi int
		w      []float64
	}
	spans := make([]span, size)
	for i := range size {
		f0 := float64(i) * scale
		f1 := f0 + scale
		lo := int(math.Floor(f0))
		hi := min(int(math.Ceil(f1)), n)
		sp := span{lo: lo, hi: hi, w: make([]float64, hi-lo)}
		for k := lo; k < hi; k++ {
			a := math.Max(f0, float64(k))
			bnd := math.Min(f1, float64(k+1))
			sp.w[k-lo] = math.Max(0, bnd-a)
		}
		spans[i] = sp
	}
	for y := range size {
		sy := spans[y]
		for x := range size {
			sx := spans[x]
			var r, g, b, a, wsum float64
			for j := sy.lo; j < sy.hi; j++ {
				wy := sy.w[j-sy.lo]
				row := src.Pix[j*src.Stride:]
				for i := sx.lo; i < sx.hi; i++ {
					wgt := wy * sx.w[i-sx.lo]
					p := row[i*4 : i*4+4]
					r += float64(p[0]) * wgt
					g += float64(p[1]) * wgt
					b += float64(p[2]) * wgt
					a += float64(p[3]) * wgt
					wsum += wgt
				}
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o+0] = clamp8(r / wsum)
			dst.Pix[o+1] = clamp8(g / wsum)
			dst.Pix[o+2] = clamp8(b / wsum)
			dst.Pix[o+3] = clamp8(a / wsum)
		}
	}
	return dst
}

// resampleBilinear upsamples a square image with bilinear interpolation.
func resampleBilinear(src *image.RGBA, size int) *image.RGBA {
	n := src.Bounds().Dx()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := float64(n) / float64(size)
	at := func(x, y int) []uint8 {
		x = max(0, min(x, n-1))
		y = max(0, min(y, n-1))
		o := y*src.Stride + x*4
		return src.Pix[o : o+4]
	}
	for y := range size {
		fy := (float64(y)+0.5)*scale - 0.5
		y0 := int(math.Floor(fy))
		ty := fy - float64(y0)
		for x := range size {
			fx := (float64(x)+0.5)*scale - 0.5
			x0 := int(math.Floor(fx))
			tx := fx - float64(x0)
			p00, p10, p01, p11 := at(x0, y0), at(x0+1, y0), at(x0, y0+1), at(x0+1, y0+1)
			o := dst.PixOffset(x, y)
			for c := range 4 {
				top := float64(p00[c])*(1-tx) + float64(p10[c])*tx
				bot := float64(p01[c])*(1-tx) + float64(p11[c])*tx
				dst.Pix[o+c] = clamp8(top*(1-ty) + bot*ty)
			}
		}
	}
	return dst
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}
