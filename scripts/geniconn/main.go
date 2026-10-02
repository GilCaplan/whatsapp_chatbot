// Command geniconn renders the WhatsApp Doppel app icon (stdlib only):
// a macOS-style squircle with a green→teal→violet "liquid glass" gradient and
// two overlapping speech bubbles — a solid one and its translucent twin.
//
//	go run ./scripts/geniconn -o build/icon_1024.png
//	go run ./scripts/geniconn -size 256 -o build/icon_256.png
//	go run ./scripts/geniconn -ico build/icon.ico     (Windows: 16…256 px, each rendered at its size)
//
// Every shape is a signed-distance function evaluated on a 4×4 sub-pixel grid,
// so edges are anti-aliased without any image library. assets/icon/icon.svg is
// the hand-editable equivalent of this artwork.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const canvas = 1024.0 // design units; output is scaled to -size

type rgba struct{ r, g, b, a float64 } // premultiplied, 0..1

// over composites src over dst (both premultiplied).
func over(dst, src rgba) rgba {
	k := 1 - src.a
	return rgba{src.r + dst.r*k, src.g + dst.g*k, src.b + dst.b*k, src.a + dst.a*k}
}

// paint returns color c (straight alpha) with coverage/opacity a, premultiplied.
func paint(c [3]float64, a float64) rgba {
	a = clamp01(a)
	return rgba{c[0] * a, c[1] * a, c[2] * a, a}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func smoothstep(e0, e1, x float64) float64 {
	t := clamp01((x - e0) / (e1 - e0))
	return t * t * (3 - 2*t)
}

func mix(a, b float64, t float64) float64 { return a + (b-a)*t }

func mixc(a, b [3]float64, t float64) [3]float64 {
	return [3]float64{mix(a[0], b[0], t), mix(a[1], b[1], t), mix(a[2], b[2], t)}
}

func hex(h uint32) [3]float64 {
	return [3]float64{float64(h>>16&0xff) / 255, float64(h>>8&0xff) / 255, float64(h&0xff) / 255}
}

// ─── signed distance functions (negative inside) ─────────────

// sdSquircle is a rounded square of half-size a whose corners use an Lp norm
// (p = n) instead of a circle, approximating the "continuous corner" shape of
// macOS icons (no visible kink where the straight edge meets the curve).
func sdSquircle(x, y, cx, cy, a, n float64) float64 {
	r := a * cornerFrac
	qx := math.Abs(x-cx) - a + r
	qy := math.Abs(y-cy) - a + r
	ox, oy := math.Max(qx, 0), math.Max(qy, 0)
	outside := math.Pow(math.Pow(ox, n)+math.Pow(oy, n), 1/n)
	return outside + math.Min(math.Max(qx, qy), 0) - r
}

func sdRoundRect(x, y, cx, cy, hw, hh, r float64) float64 {
	qx := math.Abs(x-cx) - hw + r
	qy := math.Abs(y-cy) - hh + r
	ox, oy := math.Max(qx, 0), math.Max(qy, 0)
	return math.Hypot(ox, oy) + math.Min(math.Max(qx, qy), 0) - r
}

func sdCircle(x, y, cx, cy, r float64) float64 { return math.Hypot(x-cx, y-cy) - r }

// sdTriangle is the exact distance to triangle p0 p1 p2 (Inigo Quilez).
func sdTriangle(px, py float64, p [3][2]float64) float64 {
	e := [3][2]float64{
		{p[1][0] - p[0][0], p[1][1] - p[0][1]},
		{p[2][0] - p[1][0], p[2][1] - p[1][1]},
		{p[0][0] - p[2][0], p[0][1] - p[2][1]},
	}
	d := math.Inf(1)
	s := e[0][0]*e[2][1] - e[0][1]*e[2][0]
	sign := 1.0
	minCross := math.Inf(1)
	for i := range 3 {
		vx, vy := px-p[i][0], py-p[i][1]
		h := clamp01((vx*e[i][0] + vy*e[i][1]) / (e[i][0]*e[i][0] + e[i][1]*e[i][1]))
		qx, qy := vx-e[i][0]*h, vy-e[i][1]*h
		d = math.Min(d, qx*qx+qy*qy)
		c := s * (vx*e[i][1] - vy*e[i][0])
		minCross = math.Min(minCross, c)
	}
	if minCross > 0 {
		sign = -1
	}
	return sign * math.Sqrt(d)
}

// smin is a smooth union (polynomial, radius k).
func smin(a, b, k float64) float64 {
	h := clamp01(0.5 + 0.5*(b-a)/k)
	return mix(b, a, h) - k*h*(1-h)
}

// poly is a closed polygon with a cached bounding box.
type poly struct {
	pts                    [][2]float64
	minX, minY, maxX, maxY float64
}

func newPoly(pts [][2]float64) poly {
	p := poly{pts: pts, minX: math.Inf(1), minY: math.Inf(1), maxX: math.Inf(-1), maxY: math.Inf(-1)}
	for _, q := range pts {
		p.minX, p.maxX = math.Min(p.minX, q[0]), math.Max(p.maxX, q[0])
		p.minY, p.maxY = math.Min(p.minY, q[1]), math.Max(p.maxY, q[1])
	}
	return p
}

// boxDist is a cheap lower bound of the distance to the polygon.
func (p poly) boxDist(x, y float64) float64 {
	dx := math.Max(math.Max(p.minX-x, x-p.maxX), 0)
	dy := math.Max(math.Max(p.minY-y, y-p.maxY), 0)
	return math.Hypot(dx, dy)
}

// sd is the exact signed distance (even-odd rule for the sign).
func (p poly) sd(x, y float64) float64 {
	d := math.Inf(1)
	inside := false
	n := len(p.pts)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a, b := p.pts[j], p.pts[i]
		ex, ey := b[0]-a[0], b[1]-a[1]
		vx, vy := x-a[0], y-a[1]
		h := clamp01((vx*ex + vy*ey) / (ex*ex + ey*ey))
		d = math.Min(d, math.Hypot(vx-ex*h, vy-ey*h))
		if (a[1] > y) != (b[1] > y) && x < a[0]+(y-a[1])*ex/ey {
			inside = !inside
		}
	}
	if inside {
		return -d
	}
	return d
}

// quad appends samples of the quadratic Bézier p0→c→p1 (excluding p0).
func quad(pts [][2]float64, p0, c, p1 [2]float64, steps int) [][2]float64 {
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		u := 1 - t
		pts = append(pts, [2]float64{
			u*u*p0[0] + 2*u*t*c[0] + t*t*p1[0],
			u*u*p0[1] + 2*u*t*c[1] + t*t*p1[1],
		})
	}
	return pts
}

// tailShape builds an iMessage-style tail: an outer curve leaving the bubble
// side tangentially, a sharp tip, and a concave inner curve that rejoins the
// bottom edge. in1/in2 close the polygon inside the bubble body.
func tailShape(start, c1, tip, c2, end, in1, in2 [2]float64) poly {
	pts := [][2]float64{start}
	pts = quad(pts, start, c1, tip, 40)
	pts = quad(pts, tip, c2, end, 40)
	pts = append(pts, in1, in2)
	return newPoly(pts)
}

// bubble is a rounded speech bubble with a curved tail.
type bubble struct {
	cx, cy, hw, hh, r float64
	tail              poly
}

func (b bubble) sd(x, y float64) float64 {
	body := sdRoundRect(x, y, b.cx, b.cy, b.hw, b.hh, b.r)
	if b.tail.boxDist(x, y) > 40 {
		return body
	}
	return smin(body, b.tail.sd(x, y), 2)
}

// ─── artwork ─────────────────────────────────────────────────

var (
	green  = hex(0x3BE37F)
	teal   = hex(0x10B3A3)
	violet = hex(0x7B5CF5)
	ink    = hex(0x0B3B3F)
	white  = [3]float64{1, 1, 1}

	// Back bubble: the translucent "twin", up and to the left, tail bottom-left.
	back = bubble{cx: 420, cy: 410, hw: 225, hh: 160, r: 118,
		tail: tailShape([2]float64{195, 430}, [2]float64{195, 566}, [2]float64{160, 628},
			[2]float64{226, 581}, [2]float64{322, 570}, [2]float64{322, 500}, [2]float64{240, 440})}
	// Front bubble: solid white, down and to the right, tail bottom-right.
	front = bubble{cx: 600, cy: 600, hw: 235, hh: 168, r: 124,
		tail: tailShape([2]float64{835, 630}, [2]float64{835, 774}, [2]float64{872, 828},
			[2]float64{794, 779}, [2]float64{696, 768}, [2]float64{696, 700}, [2]float64{790, 640})}
)

const (
	bodyCX, bodyCY = 512.0, 512.0
	bodyR          = 412.0 // half-size of the 824px icon body (Apple grid)
	squircleN      = 2.8
	cornerFrac     = 0.56 // corner extent as a fraction of the half-size
)

// gradient maps the diagonal (top-left → bottom-right) through the three stops.
func gradient(x, y float64) [3]float64 {
	t := clamp01(((x - 100) + (y - 100)) / (2 * 824))
	t = smoothstep(0, 1, t)*0.6 + t*0.4
	if t < 0.5 {
		return mixc(green, teal, t/0.5)
	}
	return mixc(teal, violet, (t-0.5)/0.5)
}

func shade(x, y, aa float64) rgba {
	var out rgba
	cov := func(d float64) float64 { return clamp01(0.5 - d/aa) }

	// 1. Soft drop shadow under the icon body.
	ds := sdSquircle(x, y-14, bodyCX, bodyCY, bodyR-6, squircleN)
	out = over(out, paint(ink, 0.38*(1-smoothstep(-18, 34, ds))))

	// 2. Icon body.
	d := sdSquircle(x, y, bodyCX, bodyCY, bodyR, squircleN)
	bc := cov(d)
	if bc <= 0 {
		return out
	}
	col := gradient(x, y)
	// Gentle radial light from the top-left, darker toward the bottom-right.
	light := 1 - math.Hypot(x-260, y-200)/1100
	col = mixc(col, white, 0.10*clamp01(light))
	col = mixc(col, ink, 0.12*smoothstep(0.55, 1.25, ((x-100)+(y-100))/(824)))
	body := paint(col, bc)

	// Inner glow along the rim (liquid-glass edge), stronger at the top.
	inside := -d
	rim := math.Exp(-inside/22) * (0.30 + 0.25*clamp01((512-y)/412))
	body = over(body, paint(white, rim*bc))
	// Thin bright edge line.
	body = over(body, paint(white, 0.35*clamp01(1-math.Abs(inside-2.5)/2.5)*bc*clamp01((620-y)/300)))
	// Bottom inner shade so the glass feels thick.
	body = over(body, paint(ink, 0.18*math.Exp(-inside/40)*smoothstep(500, 900, y)*bc))

	// Specular sheen: a large ellipse across the top half.
	ex, ey := (x-470)/520, (y-150)/330
	if e := ex*ex + ey*ey; e < 1 {
		a := 0.24 * (1 - smoothstep(0.55, 1.0, e)) * (1 - smoothstep(60, 520, y))
		body = over(body, paint(white, a*bc))
	}
	out = over(out, body)

	// 3. Back bubble: frosted, translucent, with a bright edge.
	db := back.sd(x, y)
	out = over(out, paint(ink, 0.12*(1-smoothstep(-10, 40, back.sd(x-6, y-16)))*bc))
	backFill := 0.30 + 0.10*clamp01((520-y)/300)
	out = over(out, paint(white, backFill*cov(db)))
	out = over(out, paint(white, 0.55*clamp01(1-math.Abs(db+3)/3.2)*bc))
	// Faint mirrored typing dots in the twin.
	for _, dx := range []float64{-88, 0, 88} {
		out = over(out, paint(white, 0.55*cov(sdCircle(x, y, back.cx+dx, back.cy-6, 24))))
	}

	// 4. Front bubble: solid white with a soft shadow and a whisper of gradient.
	df := front.sd(x, y)
	out = over(out, paint(ink, 0.30*(1-smoothstep(-24, 46, front.sd(x-4, y-22)))))
	fc := mixc(white, hex(0xE6F7F3), smoothstep(440, 820, y))
	out = over(out, paint(fc, cov(df)))
	// Top highlight inside the bubble edge.
	out = over(out, paint(white, 0.8*clamp01(1-math.Abs(df+2)/2.5)*smoothstep(700, 430, y)))

	// 5. Typing dots in the front bubble, painted with the icon gradient.
	for i, dx := range []float64{-96, 0, 96} {
		cx, cy := front.cx+dx, front.cy-4
		c := mixc(mixc(green, teal, 0.35), violet, float64(i)/2*0.9)
		c = mixc(c, ink, 0.10)
		out = over(out, paint(c, cov(sdCircle(x, y, cx, cy, 30))))
		// tiny specular dot on each
		out = over(out, paint(white, 0.45*cov(sdCircle(x, y, cx-9, cy-10, 8))))
	}
	return out
}

func render(size, ss int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := canvas / float64(size)
	aa := scale / float64(ss) // one sub-sample, in design units
	var wg sync.WaitGroup
	rows := make(chan int, size)
	for y := range size {
		rows <- y
	}
	close(rows)
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for py := range rows {
				for px := range size {
					var acc rgba
					for sy := range ss {
						for sx := range ss {
							x := (float64(px) + (float64(sx)+0.5)/float64(ss)) * scale
							y := (float64(py) + (float64(sy)+0.5)/float64(ss)) * scale
							c := shade(x, y, aa)
							acc.r += c.r
							acc.g += c.g
							acc.b += c.b
							acc.a += c.a
						}
					}
					n := float64(ss * ss)
					a := acc.a / n
					var c color.NRGBA
					if a > 0 {
						c = color.NRGBA{
							R: uint8(clamp01(acc.r/n/a)*255 + 0.5),
							G: uint8(clamp01(acc.g/n/a)*255 + 0.5),
							B: uint8(clamp01(acc.b/n/a)*255 + 0.5),
							A: uint8(clamp01(a)*255 + 0.5),
						}
					}
					img.SetNRGBA(px, py, c)
				}
			}
		}()
	}
	wg.Wait()
	return img
}

func main() {
	out := flag.String("o", "build/icon_1024.png", "output PNG path")
	size := flag.Int("size", 1024, "output size in pixels")
	ss := flag.Int("ss", 4, "supersampling factor per axis")
	ico := flag.String("ico", "", "write a multi-size Windows .ico here instead of a PNG")
	flag.Parse()

	if *ico != "" {
		if err := writeICO(*ico, *ss); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", *ico)
		return
	}

	img := render(*size, *ss)
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out)
}

// writeICO writes a .ico whose entries are PNGs rendered at each size
// (supported since Windows Vista; 256 px must be PNG anyway).
func writeICO(path string, ss int) error {
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	var pngs [][]byte
	for _, sz := range sizes {
		var b bytes.Buffer
		if err := png.Encode(&b, render(sz, ss)); err != nil {
			return err
		}
		pngs = append(pngs, b.Bytes())
	}
	var out bytes.Buffer
	le := func(v any) { _ = binary.Write(&out, binary.LittleEndian, v) }
	le(uint16(0))          // reserved
	le(uint16(1))          // type: icon
	le(uint16(len(sizes))) // count
	offset := 6 + 16*len(sizes)
	for i, sz := range sizes {
		dim := uint8(sz)
		if sz >= 256 {
			dim = 0 // 0 means 256
		}
		out.WriteByte(dim) // width
		out.WriteByte(dim) // height
		out.WriteByte(0)   // palette colours
		out.WriteByte(0)   // reserved
		le(uint16(1))      // colour planes
		le(uint16(32))     // bits per pixel
		le(uint32(len(pngs[i])))
		le(uint32(offset))
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		out.Write(p)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}
