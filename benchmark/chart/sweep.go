package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// effortSweep plots what each engine's effort setting actually buys: time on the
// x axis, output size on the y axis, one line per engine and one labelled point
// per setting. Effort is not an axis because the numbers are each engine's own
// scale and mean nothing across engines; it is the label you read off once you
// have picked a point on a curve.
//
// Both axes are totals over the test corpus, so the panel reads as "encoding
// these images costs this many seconds and this many MiB". Time is logarithmic:
// the settings within one engine span two orders of magnitude, and a linear axis
// would pile every fast setting onto the y axis.
// panels are the questions the sweep answers, left to right. Lossy gets two: an
// encoder can spend its effort on size or on quality, and a size-only panel
// would credit an encoder that quietly landed below quality 90 with the win.
var sweepPanels = []struct {
	mode, title, unit string
	// width is the panel's share of the row. The lossless panel earns a larger
	// one: its curves span three decades of time and would otherwise crowd every
	// engine but ours into the left edge.
	width float64
	// value reads the y axis off a curve point, and labelStep is the smallest
	// change worth putting a number on.
	value     func(sweepPoint) float64
	labelStep func(prev float64) float64
	logY      bool
	// clipTop ends the axis below a run of far-out points rather than scaling to
	// them, for a panel where one engine's first settings are several times
	// everything else and would otherwise leave the rest in a band too thin to
	// read. Clipped points are drawn on the top edge with their value, so the
	// panel still says they exist and how far out they are.
	clipTop bool
	format  func(float64) string
}{
	{
		mode: sweepPrefix + "lossless", title: "lossless", unit: "total size (MiB)", width: 1.3,
		value:     func(p sweepPoint) float64 { return p.mib },
		labelStep: func(prev float64) float64 { return prev * 0.01 },
		logY:      true,
		clipTop:   true,
		format:    formatMiB,
	},
	{
		mode: sweepPrefix + "lossy", title: "lossy", unit: "total size (MiB)", width: 1,
		value:     func(p sweepPoint) float64 { return p.mib },
		labelStep: func(prev float64) float64 { return prev * 0.01 },
		logY:      true,
		format:    formatMiB,
	},
	{
		mode: sweepPrefix + "lossy", title: "lossy", unit: "mean PSNR (dB)", width: 1,
		value:     func(p sweepPoint) float64 { return p.psnr },
		labelStep: func(float64) float64 { return 0.1 },
		format:    func(v float64) string { return fmt.Sprintf("%.1f", v) },
	},
}

func effortSweep(sets []dataset, th theme) string {
	engines := []string{engOurs, engLibwebp, engWasm, engNative}
	names := map[string]string{
		engOurs: "webp-go-pure", engLibwebp: "libwebp", engWasm: "wasm", engNative: "nativewebp",
	}
	color := func(eng string) string {
		if eng == engLibwebp {
			return th.muted
		}
		return th.series[eng]
	}

	const (
		padL     = 74.0
		padR     = 26.0
		colGap   = 62.0
		rowH     = 250.0
		row0Top  = 186.0
		rowStrid = rowH + 108
	)
	var weight float64
	for _, p := range sweepPanels {
		weight += p.width
	}
	unit := (figW - padL - padR - float64(len(sweepPanels)-1)*colGap) / weight
	panelX := make([]float64, len(sweepPanels))
	panelWs := make([]float64, len(sweepPanels))
	x := padL
	for i, p := range sweepPanels {
		panelX[i], panelWs[i] = x, p.width*unit
		x += panelWs[i] + colGap
	}

	note := "Each point is one effort setting, labelled with its own number: our Effort 0-9, libwebp's method 0-6 for lossy and its lossless preset level 0-9, and nativewebp's three compression levels. Time and size are totals over the whole corpus and PSNR is the mean over it; time and size are on log scales. Faster is left, smaller is down, higher quality is up. Both machines share each panel's axes, so a curve sitting further right is that machine being slower, not a different scale. A triangle on a panel's top edge is a setting whose value is off the axis, labelled setting and value and reached by a dashed segment: our lossless efforts 0 and 1 write 54 and 45 MiB, and scaling the panel to them would flatten every other engine into one band. A setting is labelled only where it moves the panel's value (1% of size, 0.1 dB of PSNR), so an unlabelled point is a setting that costs time and changes nothing: read the nearest label to its left. " +
		"The lossy panels have to be read together, since an encoder can spend effort on either one: the same quality 90 request lands between 39 and 43 dB depending on engine and setting."
	footTop := row0Top + rowStrid + rowH + 92
	h := footTop + footLineH*float64(len(footnoteWrap(note))) - 2

	c := &canvas{}
	header(c, th, h, "What effort buys",
		"Every effort setting of every engine, encoding the same corpus. Pick the tradeoff you want, then read the setting off the point.")
	var entries []struct{ color, label string }
	for _, eng := range engines {
		entries = append(entries, struct{ color, label string }{color(eng), names[eng]})
	}
	legend(c, th, 40, 84, entries)
	c.text(40, 112, 12.5, th.muted, "start", "",
		"Lossy is quality 90 throughout, which each encoder hits differently; lossless is exact, so size is its only axis.")

	// Both rows share each panel's axes. The two machines encode the same corpus
	// to the same bytes, so a per-row range would rescale identical size and
	// quality data differently and make the rows look like different results;
	// shared, the only thing that moves between them is time, which is the one
	// thing the machine actually changes.
	type axisRange struct{ xMin, xMax, yMin, yMax float64 }
	panelRange := make([]axisRange, len(sweepPanels))
	for pi, panel := range sweepPanels {
		var xs, ys []float64
		for _, d := range sets {
			for _, eng := range engines {
				for _, p := range sweepCurve(d, panel.mode, eng) {
					if v := panel.value(p); v > 0 {
						xs = append(xs, p.seconds)
						ys = append(ys, v)
					}
				}
			}
		}
		if len(xs) == 0 {
			continue
		}
		xMin, xMax := minMax(xs)
		yMin, yMax := minMax(ys)
		if panel.clipTop {
			yMax = clipOutlierTop(ys)
		}
		panelRange[pi] = axisRange{xMin, xMax, yMin, yMax}
	}

	for ri, d := range sets {
		rowTop := row0Top + float64(ri)*rowStrid
		rowBot := rowTop + rowH
		c.text(40, rowTop-44, 13, th.inkSecondary, "start", "600", panelTitle(d.label))
		c.text(40+textWidth(panelTitle(d.label), 13)+18, rowTop-44, 11.5, th.muted, "start", "", machineDetail(d.label))

		for pi, panel := range sweepPanels {
			px, panelW := panelX[pi], panelWs[pi]
			curves := map[string][]sweepPoint{}
			for _, eng := range engines {
				var kept []sweepPoint
				for _, p := range sweepCurve(d, panel.mode, eng) {
					if panel.value(p) > 0 {
						kept = append(kept, p)
					}
				}
				if len(kept) > 0 {
					curves[eng] = kept
				}
			}
			rng := panelRange[pi]
			if rng.xMax == 0 {
				continue
			}
			c.text(px+panelW/2, rowTop-22, 13, th.inkPrimary, "middle", "600", panel.title)
			c.text(px+panelW/2, rowTop-8, 11.5, th.muted, "middle", "", panel.unit)

			xMin, xMax := rng.xMin, rng.xMax
			yMin, yMax := rng.yMin, rng.yMax
			// Time and size are logarithmic, padded so the outermost points are
			// not on the frame. Size needs it as much as time does: our two
			// fastest lossless settings write files several times the size of
			// everything else, and a linear axis would squash every other engine
			// into one band at the bottom. PSNR is already a log measure.
			lo, hi := math.Log10(xMin)-0.12, math.Log10(xMax)+0.12
			sx := func(v float64) float64 { return px + (math.Log10(v)-lo)/(hi-lo)*panelW }
			var sy func(float64) float64
			var yAxis []float64
			if panel.logY {
				yLo, yHi := math.Log10(yMin)-0.06, math.Log10(yMax)+0.06
				sy = func(v float64) float64 {
					y := rowBot - (math.Log10(v)-yLo)/(yHi-yLo)*rowH
					return math.Max(y, rowTop)
				}
				yAxis = axisTicks(math.Pow(10, yLo), math.Pow(10, yHi))
			} else {
				pad := math.Max((yMax-yMin)*0.1, 0.2)
				yLo, yHi := yMin-pad, yMax+pad
				sy = func(v float64) float64 { return rowBot - (v-yLo)/(yHi-yLo)*rowH }
				yAxis = evenTicks(yLo, yHi)
			}

			for _, t := range yAxis {
				y := sy(t)
				c.line(px, y, px+panelW, y, th.grid, 1)
				c.tickText(px-9, y+4, 11, th.muted, "end", panel.format(t))
			}
			// A log axis spanning three decades produces more ticks than the
			// panel is wide enough to label, so thin them out by position.
			lastX := math.Inf(-1)
			for _, t := range axisTicks(math.Pow(10, lo), math.Pow(10, hi)) {
				x := sx(t)
				c.line(x, rowTop, x, rowBot, th.grid, 1)
				if x-lastX < 42 {
					continue
				}
				lastX = x
				c.tickText(x, rowBot+18, 11, th.muted, "middle", formatSeconds(t))
			}
			c.line(px, rowBot, px+panelW, rowBot, th.axis, 1)

			var placed []label
			for _, eng := range engines {
				pts := curves[eng]
				if len(pts) == 0 {
					continue
				}
				// Segments that touch a clipped point are drawn dashed. Their
				// endpoint sits on the top edge rather than where the value is, so
				// a solid line there would draw a slope the data does not have.
				var onScale, offScale strings.Builder
				for i := 1; i < len(pts); i++ {
					a, b := pts[i-1], pts[i]
					target := &onScale
					if panel.value(a) > yMax || panel.value(b) > yMax {
						target = &offScale
					}
					fmt.Fprintf(target, "M %.1f %.1f L %.1f %.1f ",
						sx(a.seconds), sy(panel.value(a)), sx(b.seconds), sy(panel.value(b)))
				}
				if onScale.Len() > 0 {
					c.printf(`<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" opacity="0.85"/>`,
						strings.TrimSpace(onScale.String()), color(eng))
				}
				if offScale.Len() > 0 {
					c.printf(`<path d="%s" fill="none" stroke="%s" stroke-width="1.5" stroke-dasharray="3 3" stroke-linecap="round" opacity="0.45"/>`,
						strings.TrimSpace(offScale.String()), color(eng))
				}
				labels := effortLabels(pts, panel.value, panel.labelStep, sx, sy)
				moved := map[int]bool{}
				for _, l := range labels {
					moved[l.effort] = true
				}
				// A setting that does not move the panel's value is drawn as a
				// smaller dot with no ring. Several engines hold one size across
				// most of their range, and at full weight those runs read as a
				// row of choices rather than as one choice repeated.
				for _, p := range pts {
					x, v := sx(p.seconds), panel.value(p)
					if v > yMax {
						// Labelled with the setting and its value: every other label
						// in the panel is a setting number, so a bare value here
						// would read as one.
						c.printf(`<path d="M %.1f %.1f l 4.5 6 l -9 0 Z" fill="%s"/>`, x, rowTop+1, color(eng))
						c.haloText(x, rowTop+20, labelSize, color(eng), th.surface,
							fmt.Sprintf("%d: %s", p.effort, panel.format(v)))
						continue
					}
					if moved[p.effort] {
						c.circle(x, sy(v), 3.5, color(eng), th.surface)
					} else {
						c.circle(x, sy(v), 2, color(eng), "")
					}
				}
				for _, l := range labels {
					if l.value > yMax {
						continue
					}
					l.color = color(eng)
					placed = append(placed, place(l, placed, rowTop+30, rowBot-5))
				}
			}
			// Labels go on after every curve in the panel, so one engine's line
			// cannot paint over another's numbers.
			for _, l := range placed {
				c.haloText(l.x, l.y, labelSize, l.color, th.surface, l.text)
			}
		}
	}

	lastBot := row0Top + rowStrid + rowH
	c.text(figW/2, lastBot+46, 12.5, th.inkSecondary, "middle", "",
		"encode time for the whole corpus, log scale (left = faster)")
	footnote(c, th, 40, footTop, note)
	c.printf(`</svg>`)
	return c.b.String()
}

type sweepPoint struct {
	effort  int
	seconds float64
	mib     float64
	// psnr is the mean over the images, not a total: decibels do not add.
	psnr   float64
	images int
}

// sweepCurve folds one engine's sweep rows into a curve: each effort setting
// becomes the corpus totals for time and size. Summing rather than averaging
// keeps the two axes on the same footing, since a big image costs proportionally
// more of both.
func sweepCurve(d dataset, mode, engine string) []sweepPoint {
	// Only images measured at every setting count, so a half-finished capture
	// cannot put more images into one setting's totals than another's: that
	// reads as the higher setting being both faster and smaller, which is a
	// property of the capture rather than of the encoder.
	files := map[string]int{}
	efforts := map[int]bool{}
	for _, r := range d.rows {
		if r.engine != engine || r.mode != mode || r.ms <= 0 || r.bytes <= 0 {
			continue
		}
		files[r.file]++
		efforts[r.effort] = true
	}
	byEffort := map[int]*sweepPoint{}
	for _, r := range d.rows {
		if r.engine != engine || r.mode != mode || r.ms <= 0 || r.bytes <= 0 {
			continue
		}
		if files[r.file] != len(efforts) {
			continue
		}
		p := byEffort[r.effort]
		if p == nil {
			p = &sweepPoint{effort: r.effort}
			byEffort[r.effort] = p
		}
		p.seconds += r.ms / 1000
		p.mib += float64(r.bytes) / (1 << 20)
		if r.hasPSNR {
			p.psnr += r.psnr
			p.images++
		}
	}
	var out []sweepPoint
	for _, p := range byEffort {
		if p.images > 0 {
			p.psnr /= float64(p.images)
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].effort < out[j].effort })
	return out
}

const labelSize = 10.0

type label struct {
	x, y   float64
	text   string
	color  string
	effort int
	value  float64
	// anchor is the point the label belongs to, so a label pushed aside can be
	// tested for overlap without drifting away from its own point.
	anchorY float64
}

// place moves a label off the ones already in the panel, trying above the point
// first and then below it, so two engines that meet at the same setting still
// both read. It stays inside the panel: a label in the tick row below the axis
// reads as an axis label rather than as a point's.
func place(l label, taken []label, top, bottom float64) label {
	w := textWidth(l.text, labelSize)
	free := func(y float64) bool {
		if y < top || y > bottom {
			return false
		}
		for _, o := range taken {
			ow := textWidth(o.text, labelSize)
			if math.Abs(o.x-l.x) < (w+ow)/2+2 && math.Abs(o.y-y) < 11 {
				return false
			}
		}
		return true
	}
	for _, dy := range []float64{-9, 16, -22, 29, -35, 42} {
		if y := l.anchorY + dy; free(y) {
			l.y = y
			return l
		}
	}
	return l
}

// effortLabels labels a setting only where it moves the panel's value: an engine
// whose settings 3 through 8 all write the same bytes has one interesting
// setting there, the cheapest of them, and six numbers on the panel would
// suggest six choices. Points stay on the curve either way, so the settings in
// between are still visible as time spent for nothing.
func effortLabels(pts []sweepPoint, value func(sweepPoint) float64, step func(float64) float64, sx, sy func(float64) float64) []label {
	var out []label
	var last float64
	for i, p := range pts {
		v := value(p)
		if i > 0 && math.Abs(v-last) < step(last) {
			continue
		}
		last = v
		y := sy(v)
		out = append(out, label{x: sx(p.seconds), y: y - 9, text: fmt.Sprintf("%d", p.effort), anchorY: y, effort: p.effort, value: v})
	}
	return out
}

// clipOutlierTop finds where a panel's values stop being a spread and become a
// jump. It sorts them on the log axis they are drawn on, takes the largest gap
// between neighbours, and if that gap is both wide in absolute terms and several
// times wider than any other, returns the value just below it. Everything above
// is drawn on the top edge instead of setting the scale. With no such gap it
// returns the largest value and the axis is unchanged.
func clipOutlierTop(vs []float64) float64 {
	logs := append([]float64(nil), vs...)
	for i := range logs {
		logs[i] = math.Log10(logs[i])
	}
	sort.Float64s(logs)
	uniq := logs[:0]
	for i, v := range logs {
		if i == 0 || v-uniq[len(uniq)-1] > 1e-9 {
			uniq = append(uniq, v)
		}
	}
	if len(uniq) < 4 {
		return math.Pow(10, logs[len(logs)-1])
	}
	widest, widestAt, runnerUp := 0.0, -1, 0.0
	for i := 1; i < len(uniq); i++ {
		gap := uniq[i] - uniq[i-1]
		if gap > widest {
			widest, widestAt, runnerUp = gap, i, widest
		} else if gap > runnerUp {
			runnerUp = gap
		}
	}
	// A quarter decade of empty axis, three times any other gap, and not so low
	// that clipping would hide most of the data.
	if widest < 0.25 || widest < 3*runnerUp || widestAt < len(uniq)/2 {
		return math.Pow(10, uniq[len(uniq)-1])
	}
	return math.Pow(10, uniq[widestAt-1])
}

func minMax(vs []float64) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range vs {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return lo, hi
}

// axisTicks picks gridlines for a log axis, using whichever set of round
// multiples lands 4 to 9 lines in the range. Evenly spaced values are wrong
// here: the lossless panel runs 13 to 62 MiB, where a step of 10 puts one line
// under 20 and leaves the band every engine but ours sits in unlabelled, while
// 1-1.5-2-3-5-7 spaces the lines the way the axis does.
func axisTicks(lo, hi float64) []float64 {
	for _, mult := range [][]float64{{1, 2, 5}, {1, 2, 3, 5, 7}, {1, 1.5, 2, 3, 5, 7}} {
		if t := logTicks(lo, hi, mult); len(t) >= 4 && len(t) <= 9 {
			return t
		}
	}
	if t := logTicks(lo, hi, []float64{1, 1.5, 2, 3, 5, 7}); len(t) >= 4 {
		return t
	}
	return evenTicks(lo, hi)
}

// logTicks returns the given decade multiples inside a range.
func logTicks(lo, hi float64, mult []float64) []float64 {
	var out []float64
	for e := math.Floor(math.Log10(lo)); e <= math.Ceil(math.Log10(hi)); e++ {
		for _, m := range mult {
			v := m * math.Pow(10, e)
			if v >= lo && v <= hi {
				out = append(out, v)
			}
		}
	}
	sort.Float64s(out)
	return out
}

// evenTicks picks a round step that lands 4 to 8 gridlines in the range.
func evenTicks(lo, hi float64) []float64 {
	span := hi - lo
	if span <= 0 {
		return nil
	}
	step := math.Pow(10, math.Floor(math.Log10(span/4)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if span/(step*m) <= 8 {
			step *= m
			break
		}
	}
	var out []float64
	for v := math.Ceil(lo/step) * step; v <= hi; v += step {
		out = append(out, v)
	}
	return out
}

func formatMiB(v float64) string {
	if v >= 10 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func formatSeconds(v float64) string {
	switch {
	case v >= 10:
		return fmt.Sprintf("%.0f s", v)
	case v >= 1:
		return fmt.Sprintf("%.0f s", v)
	default:
		return fmt.Sprintf("%.1f s", v)
	}
}
