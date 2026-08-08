package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	figW     = 900
	fontSans = `system-ui, -apple-system, "Segoe UI", sans-serif`
)

// figWidth is the width the figure being drawn is laid out at. Most figures use
// figW; the effort sweep fits three panels per machine row and sets its own.
var figWidth = float64(figW)

type canvas struct {
	b strings.Builder
}

func (c *canvas) printf(format string, args ...any) {
	fmt.Fprintf(&c.b, format, args...)
	c.b.WriteByte('\n')
}

func (c *canvas) text(x, y float64, size float64, fill, anchor, weight, s string) {
	w := ""
	if weight != "" {
		w = fmt.Sprintf(` font-weight="%s"`, weight)
	}
	c.printf(`<text x="%.1f" y="%.1f" font-family='%s' font-size="%.0f" fill="%s" text-anchor="%s"%s>%s</text>`,
		x, y, fontSans, size, fill, anchor, w, escape(s))
}

// tick text keeps digits aligned in columns.
func (c *canvas) tickText(x, y float64, size float64, fill, anchor, s string) {
	c.printf(`<text x="%.1f" y="%.1f" font-family='%s' font-size="%.0f" fill="%s" text-anchor="%s" style="font-variant-numeric:tabular-nums">%s</text>`,
		x, y, fontSans, size, fill, anchor, escape(s))
}

// haloText paints a surface-colored outline behind the glyphs, so a value label
// stays legible where it crosses a neighbouring bar.
func (c *canvas) haloText(x, y, size float64, fill, halo, s string) {
	c.printf(`<text x="%.1f" y="%.1f" font-family='%s' font-size="%.0f" fill="%s" text-anchor="middle" stroke="%s" stroke-width="3" paint-order="stroke" style="font-variant-numeric:tabular-nums">%s</text>`,
		x, y, fontSans, size, fill, halo, escape(s))
}

func (c *canvas) line(x1, y1, x2, y2 float64, stroke string, width float64) {
	c.printf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f"/>`,
		x1, y1, x2, y2, stroke, width)
}

func (c *canvas) circle(x, y, r float64, fill, ring string) {
	if ring != "" {
		c.printf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, x, y, r+2, ring)
	}
	c.printf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, x, y, r, fill)
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func textWidth(s string, size float64) float64 { return float64(len(s)) * size * 0.55 }

func header(c *canvas, th theme, h float64, title, subtitle string) {
	c.printf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" role="img">`,
		figWidth, h, figWidth, h)
	c.printf(`<rect width="%.0f" height="%.0f" fill="%s"/>`, figWidth, h, th.surface)
	c.text(40, 34, 19, th.inkPrimary, "start", "600", title)
	c.text(40, 56, 13, th.inkSecondary, "start", "", subtitle)
}

const (
	footSize  = 11.5
	footLineH = 16.0
)

// footnoteWrap breaks a note at the figure width so a long one cannot run off
// the edge. Callers size the figure from the line count before drawing it.
func footnoteWrap(s string) []string {
	var lines []string
	var line string
	for _, word := range strings.Fields(s) {
		next := strings.TrimSpace(line + " " + word)
		if textWidth(next, footSize) > figWidth-80 && line != "" {
			lines = append(lines, line)
			line = word
			continue
		}
		line = next
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func footnote(c *canvas, th theme, x, y float64, s string) {
	for _, line := range footnoteWrap(s) {
		c.text(x, y, footSize, th.muted, "start", "", line)
		y += footLineH
	}
}

// legend draws swatch+label pairs; identity never rests on color alone.
func legend(c *canvas, th theme, x, y float64, entries []struct{ color, label string }) {
	cx := x
	for _, e := range entries {
		c.circle(cx+5, y-4, 5, e.color, "")
		c.text(cx+16, y, 12.5, th.inkSecondary, "start", "", e.label)
		cx += 16 + textWidth(e.label, 12.5) + 26
	}
}

// ---------------------------------------------------------------- figure one

// rateDistortion plots output size and PSNR relative to libwebp, one point per
// image, faceted by mode on shared scales. libwebp is the origin; wasm encodes
// identically to it, so the two share that reference marker.
func rateDistortion(d dataset, th theme) string {
	type point struct {
		engine, file string
		x, y         float64
	}
	panels := map[string][]point{}
	xMin, xMax := math.Inf(1), math.Inf(-1)
	yMin, yMax := math.Inf(1), math.Inf(-1)

	for _, mode := range lossyModes {
		for _, file := range d.files() {
			base, ok := d.find(engLibwebp, mode, file)
			if !ok {
				continue
			}
			for _, eng := range []string{engOurs} {
				r, ok := d.find(eng, mode, file)
				if !ok || !r.hasPSNR || !base.hasPSNR {
					continue
				}
				p := point{eng, file, float64(r.bytes) / float64(base.bytes), r.psnr - base.psnr}
				panels[mode] = append(panels[mode], p)
				xMin, xMax = math.Min(xMin, p.x), math.Max(xMax, p.x)
				yMin, yMax = math.Min(yMin, p.y), math.Max(yMax, p.y)
			}
		}
	}
	xMin, xMax = math.Min(xMin, 1)-0.06, math.Max(xMax, 1)+0.06
	yMin, yMax = math.Floor(yMin)-0.4, math.Max(yMax, 0)+0.5

	const (
		h        = 470.0
		plotTop  = 118.0
		plotBot  = 372.0
		leftPad  = 78.0
		gap      = 74.0
		rightPad = 34.0
	)
	panelW := (figW - leftPad - rightPad - gap) / 2.0

	c := &canvas{}
	header(c, th, h, "Size and quality against libwebp",
		"Each point is one test image, encoded at quality 90. Up and to the left is better: smaller file, higher PSNR.")
	legend(c, th, 40, 84, []struct{ color, label string }{
		{th.series[engOurs], "webp-go-pure"},
		{th.muted, "libwebp and wasm (reference, at the crosshair)"},
	})

	sx := func(px float64, v float64) float64 { return px + (v-xMin)/(xMax-xMin)*panelW }
	sy := func(v float64) float64 {
		return plotBot - (v-yMin)/(yMax-yMin)*(plotBot-plotTop)
	}

	for i, mode := range lossyModes {
		px := leftPad + float64(i)*(panelW+gap)

		for v := math.Ceil(yMin); v <= yMax; v++ {
			y := sy(v)
			c.line(px, y, px+panelW, y, th.grid, 1)
			if i == 0 {
				c.tickText(px-10, y+4, 11.5, th.muted, "end", fmt.Sprintf("%+g", v))
			}
		}
		for v := 0.4; v <= xMax; v += 0.2 {
			if v < xMin {
				continue
			}
			c.tickText(sx(px, v), plotBot+20, 11.5, th.muted, "middle", fmt.Sprintf("%.1f", v))
		}

		// Reference crosshair: libwebp is (1.0, 0) by construction, and wasm
		// encodes byte-identically, so both sit on this marker.
		c.line(sx(px, 1), plotTop, sx(px, 1), plotBot, th.axis, 1)
		c.line(px, sy(0), px+panelW, sy(0), th.axis, 1)
		c.circle(sx(px, 1), sy(0), 5, th.muted, th.surface)
		c.text(px+panelW/2, plotTop-16, 13.5, th.inkPrimary, "middle", "600", mode)

		for _, p := range panels[mode] {
			c.circle(sx(px, p.x), sy(p.y), 5, th.series[p.engine], th.surface)
		}
	}

	c.text(leftPad+panelW+gap/2, 416, 12.5, th.inkSecondary, "middle", "",
		"output size relative to libwebp  (1.0 = same size, left = smaller)")
	c.printf(`<text transform="translate(26,%.1f) rotate(-90)" font-family='%s' font-size="12.5" fill="%s" text-anchor="middle">%s</text>`,
		(plotTop+plotBot)/2, fontSans, th.inkSecondary, "PSNR vs libwebp (dB)")
	footnote(c, th, 40, 442,
		"lossy-fast is each encoder's fastest setting and lossy-slow its slowest, which are not the same amount of work across encoders; the effort sweep figure shows every setting in between.")
	c.printf(`</svg>`)
	return c.b.String()
}

// ---------------------------------------------------------------- figure two

// encodeTime plots encode time itself: the geometric mean of each engine's
// ms/op over the test images. Each mode gets its own linear panel, because the
// modes are three orders of magnitude apart and a shared axis would flatten the
// fast ones to nothing. Every bar carries its value, so a bar too short to see
// still reports its number.
func encodeTime(sets []dataset, th theme) string {
	return barPanels(sets, th, barSpec{
		title:    "Encode time",
		subtitle: "Geometric mean of each engine's ms/op over the test images, at quality 90. Each panel has its own scale.",
		footnote: "Each panel has its own scale, so compare bars within a panel and read the labels across panels. A bar that fades out with an arrow runs off the top of its panel. wasm is libwebp compiled to WebAssembly: that gap is the cost of dropping cgo. nativewebp encodes VP8L only, so it has a bar in the lossless panel alone.",
		value:    func(r row) float64 { return r.ms },
		format:   duration,
	})
}

// decodeTime plots the other direction, on the same bar layout. Every engine
// decoded the same libwebp-encoded files and had to end at packed RGBA, so the
// bars include the color conversion for the engines that return YCbCr planes.
func decodeTime(sets []dataset, th theme) string {
	return barPanels(sets, th, barSpec{
		title:    "Decode time",
		subtitle: "Geometric mean of each engine's ms/op over the test images, decoding files libwebp encoded. Each panel has its own scale.",
		footnote: "Every engine ends at packed RGBA, so x/image and wasm pay for converting their YCbCr planes inside the measurement, as an application would. x/image is golang.org/x/image/webp, the Go project's own decoder, which has no encoder and so appears in this figure alone.",
		modes:    decodeModes,
		engines:  []string{engOurs, engLibwebp, engWasm, engXImage},
		value:    func(r row) float64 { return r.ms },
		format:   duration,
	})
}

func peakMemory(sets []dataset, th theme) string {
	return barPanels(sets, th, barSpec{
		title:    "Peak memory",
		subtitle: "Geometric mean of each engine's peak RSS in MiB per megapixel of source image, at quality 90. Each panel has its own scale.",
		footnote: "One encode per process: source bitmap, runtime and encoder together, which is what an application pays. A 1080p frame is 2.1 MP and a 4K frame roughly 10 MP, so multiply through to size a workload. wasm carries a WebAssembly runtime and its own linear memory, which is why it costs more than the same encoder as C. nativewebp encodes VP8L only, so it has a bar in the lossless panel alone.",
		value:    func(r row) float64 { return r.mibPerMP },
		format:   func(v float64) string { return fmt.Sprintf("%.0f", v) },
	})
}

type barSpec struct {
	title, subtitle, footnote string
	// modes and engines default to the encode pass's three modes and five
	// engines.
	modes, engines []string
	value          func(row) float64
	format         func(float64) string
}

func barPanels(sets []dataset, th theme, spec barSpec) string {
	modes := spec.modes
	if modes == nil {
		modes = allModes
	}
	engines := spec.engines
	if engines == nil {
		// nativewebp goes last: it is lossless-only, so its slot is empty in the
		// lossy panels, and an empty slot at the edge of the group reads as
		// absence where one in the middle would read as a gap.
		engines = []string{engOurs, engLibwebp, engWasm, engNative}
	}
	names := map[string]string{
		engOurs: "webp-go-pure", engLibwebp: "libwebp", engWasm: "wasm",
		engXImage: "x/image", engNative: "nativewebp",
	}
	color := func(eng string) string {
		if eng == engLibwebp {
			return th.muted // the reference encoder, kept neutral in both figures
		}
		return th.series[eng]
	}

	times := make([]map[string]map[string]float64, len(sets))
	for i, d := range sets {
		times[i] = map[string]map[string]float64{}
		for _, mode := range modes {
			times[i][mode] = map[string]float64{}
			for _, eng := range engines {
				var sum float64
				var n int
				for _, file := range d.files() {
					r, ok := d.find(eng, mode, file)
					if !ok || spec.value(r) <= 0 {
						continue
					}
					sum += math.Log(spec.value(r))
					n++
				}
				if n > 0 {
					times[i][mode][eng] = math.Exp(sum / float64(n))
				}
			}
		}
	}

	// The footnote wraps, so the figure grows with it rather than clipping the
	// last line: nativewebp's note is what first pushed one past three lines.
	const footTop = 550.0
	h := footTop + footLineH*float64(len(footnoteWrap(spec.footnote))) - 2

	const (
		gutter  = 134.0
		colGap  = 48.0
		rightPd = 34.0
		rowH    = 140.0
		rowGap  = 70.0
		headerY = 128.0
		row0Top = 172.0
	)
	n := float64(len(modes))
	colW := (figW - gutter - rightPd - (n-1)*colGap) / n
	const barW = 24.0

	fadeID := 0
	c := &canvas{}
	header(c, th, h, spec.title, spec.subtitle)
	var entries []struct{ color, label string }
	for _, eng := range engines {
		entries = append(entries, struct{ color, label string }{color(eng), names[eng]})
	}
	legend(c, th, 40, 84, entries)

	for gi, mode := range modes {
		cx := gutter + float64(gi)*(colW+colGap)
		c.text(cx+colW/2, headerY, 13.5, th.inkPrimary, "middle", "600", strings.TrimPrefix(mode, decodePrefix))
	}

	// Whether to clip is decided per mode rather than per panel: with two machines
	// landing either side of the threshold, the same mode would otherwise be drawn
	// clipped in one row and not the other, which reads as a difference in the data.
	panelValues := func(ri int, mode string) []float64 {
		var vs []float64
		for _, eng := range engines {
			if v, ok := times[ri][mode][eng]; ok {
				vs = append(vs, v)
			}
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(vs)))
		return vs
	}
	clipMode := map[string]bool{}
	for _, mode := range modes {
		for ri := range sets {
			vs := panelValues(ri, mode)
			if len(vs) > 1 && vs[0] > 2*vs[1] {
				clipMode[mode] = true
			}
		}
	}

	for ri, d := range sets {
		rowTop := row0Top + float64(ri)*(rowH+rowGap)
		rowBot := rowTop + rowH
		c.text(gutter-18, rowTop+rowH/2, 12.5, th.inkSecondary, "end", "600", panelTitle(d.label))
		c.text(gutter-18, rowTop+rowH/2+16, 11, th.muted, "end", "", machineDetail(d.label))

		for gi, mode := range modes {
			cx := gutter + float64(gi)*(colW+colGap)

			// A bar more than twice the next tallest would flatten the rest of
			// the panel, so the panel scales to the runner-up and the outlier is
			// drawn clipped, with its value.
			sorted := panelValues(ri, mode)
			if len(sorted) == 0 {
				continue
			}
			scaleMax := sorted[0]
			if len(sorted) > 1 && clipMode[mode] {
				scaleMax = sorted[1]
			}
			c.line(cx, rowBot, cx+colW, rowBot, th.axis, 1)

			span := float64(len(engines))*barW + float64(len(engines)-1)*2
			// Leave headroom when a bar is clipped, so the clipped one visibly
			// overshoots the rest instead of ending a few pixels above them.
			usable := rowH - 16
			if scaleMax < sorted[0] {
				usable = rowH * 0.72
			}

			type pending struct {
				x, y  float64
				label string
			}
			var labels []pending
			for ei, eng := range engines {
				v, ok := times[ri][mode][eng]
				if !ok {
					continue
				}
				x := cx + (colW-span)/2 + float64(ei)*(barW+2)
				var labelY float64
				if v > scaleMax {
					// Runs off the panel: the bar fades out at the top and an
					// arrow carries it onward, so the eye reads "continues"
					// rather than "ends here".
					fadeID++
					id := fmt.Sprintf("fade%d", fadeID)
					c.printf(`<linearGradient id="%s" gradientUnits="userSpaceOnUse" x1="0" y1="%.1f" x2="0" y2="%.1f"><stop offset="0" stop-color="%s" stop-opacity="0"/><stop offset="1" stop-color="%s" stop-opacity="1"/></linearGradient>`,
						id, rowTop, rowTop+64, color(eng), color(eng))
					c.printf(`<path d="%s" fill="url(#%s)"/>`, barPath(x, barW, rowBot, rowTop), id)
					// The arrow floats clear of the faded tip, so the gap itself
					// reads as "keeps going".
					c.printf(`<path d="M %.1f %.1f l 7 10 l -14 0 z" fill="%s"/>`,
						x+barW/2, rowTop-20, color(eng))
					labelY = rowTop - 28
				} else {
					c.printf(`<path d="%s" fill="%s"/>`, barPath(x, barW, rowBot, rowBot-(v/scaleMax)*usable), color(eng))
					labelY = rowBot - (v/scaleMax)*usable - 7
				}
				labels = append(labels, pending{x + barW/2, labelY, spec.format(v)})
			}

			// Labels go on after every bar in the panel, so a taller neighbour
			// cannot paint over one. Short bars sit their labels at the same
			// height, so lift one when two would overlap.
			var prevRight, prevY float64
			for li, p := range labels {
				w := textWidth(p.label, 11)
				if li > 0 && p.x-w/2 < prevRight+3 && math.Abs(p.y-prevY) < 12 {
					p.y = prevY - 14
				}
				c.haloText(p.x, p.y, 11, th.inkSecondary, th.surface, p.label)
				prevRight, prevY = p.x+w/2, p.y
			}
		}
	}

	footnote(c, th, 40, footTop, spec.footnote)
	c.printf(`</svg>`)
	return c.b.String()
}

// duration formats a time in the unit that keeps it short and readable.
func duration(ms float64) string {
	switch {
	case ms >= 10000:
		return fmt.Sprintf("%.0f s", ms/1000)
	case ms >= 1000:
		return fmt.Sprintf("%.1f s", ms/1000)
	case ms >= 100:
		return fmt.Sprintf("%.0f ms", ms)
	default:
		return fmt.Sprintf("%.0f ms", ms)
	}
}

// machineDetail pulls "Apple M4 Pro" out of "arm64 (Apple M4 Pro)".
func machineDetail(s string) string {
	i, j := strings.Index(s, "("), strings.LastIndex(s, ")")
	if i < 0 || j < i {
		return ""
	}
	return s[i+1 : j]
}

// barPath draws a bar as one path, rounded on the data end and square where it
// meets the baseline. base is the baseline y, tip is the value y.
func barPath(x, w, base, tip float64) string {
	r := math.Min(4, w/2)
	if math.Abs(tip-base) < r {
		// Too short to round: a plain rectangle avoids a distorted cap.
		top, h := math.Min(base, tip), math.Abs(tip-base)
		return fmt.Sprintf("M %.1f %.1f h %.1f v %.1f h %.1f z", x, top, w, math.Max(h, 1), -w)
	}
	if tip < base { // grows upward
		return fmt.Sprintf("M %.1f %.1f L %.1f %.1f Q %.1f %.1f %.1f %.1f L %.1f %.1f Q %.1f %.1f %.1f %.1f L %.1f %.1f Z",
			x, base, x, tip+r, x, tip, x+r, tip, x+w-r, tip, x+w, tip, x+w, tip+r, x+w, base)
	}
	return fmt.Sprintf("M %.1f %.1f L %.1f %.1f Q %.1f %.1f %.1f %.1f L %.1f %.1f Q %.1f %.1f %.1f %.1f L %.1f %.1f Z",
		x, base, x, tip-r, x, tip, x+r, tip, x+w-r, tip, x+w, tip, x+w, tip-r, x+w, base)
}

// panelTitle keeps the architecture from an "arm64 (Apple M4 Pro)" heading; the
// full machine names live in the figure's footnote, where they cannot overflow
// a panel.
func panelTitle(s string) string {
	if i := strings.Index(s, "("); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
