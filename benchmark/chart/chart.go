//usr/bin/env true; cd "$(dirname "$0")"; exec go run . "$@"

// chart renders the captured benchmark results as SVG figures for results.md.
//
// The first line is a comment to Go and a command to sh, so this file runs as a
// script (./benchmark/chart/chart.go) as well as via `go run ./chart`.
//
// It reads the tables embedded in results.md (each fenced block under a "## "
// heading is one machine) so the figures and the tables cannot drift apart, and
// writes a light and a dark variant of each figure, which results.md selects
// between with GitHub's #gh-light-mode-only / #gh-dark-mode-only anchors.
//
//	go run ./chart -md results.md -out charts   (from benchmark/)
//
// Five figures, because size/quality, the effort tradeoff, encode speed, decode
// speed and memory are different questions:
//
//   - rate-distortion: output size and PSNR, both relative to libwebp, one point
//     per image. Engines that are strictly better sit up and to the left.
//   - effort sweep: every effort setting of every engine as a point in (time,
//     size), one line per engine. This is the figure that says what an effort
//     setting buys, and the one to read before the fixed-mode figures.
//   - encode time: geometric mean of each engine's ms/op, grouped by mode, one
//     panel per machine.
//   - decode time: the same, for the decode pass, which adds the x/image engine.
//   - peak memory: geometric mean of each engine's peak RSS per megapixel, laid
//     out the same way.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type row struct {
	engine, mode, file string
	effort             int
	bytes              int
	psnr               float64
	hasPSNR            bool
	ms                 float64
	mibPerMP           float64
}

type dataset struct {
	label string
	rows  []row
}

const (
	engOurs    = "ours"
	engLibwebp = "libwebp"
	engWasm    = "wasm"
	engXImage  = "x/image"
	engNative  = "nativewebp"
)

var lossyModes = []string{"lossy-fast", "lossy-slow"}
var allModes = []string{"lossless", "lossy-fast", "lossy-slow"}

// Decode rows carry the prefix so they cannot collide with the encode row for
// the same engine and image: both passes have a "lossless" mode.
const decodePrefix = "decode-"

var decodeModes = []string{decodePrefix + "lossless", decodePrefix + "lossy"}

// Sweep rows carry a prefix for the same reason, and an effort column the other
// passes do not have.
const sweepPrefix = "sweep-"

var sweepModes = []string{sweepPrefix + "lossless", sweepPrefix + "lossy"}

type theme struct {
	name                            string
	surface, plane                  string
	inkPrimary, inkSecondary, muted string
	grid, axis                      string
	series                          map[string]string
}

var themes = []theme{
	{
		name: "light", surface: "#fcfcfb", plane: "#f9f9f7",
		inkPrimary: "#0b0b0b", inkSecondary: "#52514e", muted: "#898781",
		grid: "#e1e0d9", axis: "#c3c2b7",
		series: map[string]string{engOurs: "#2a78d6", engWasm: "#1baf7a", engXImage: "#8a5cd0", engNative: "#c2456f"},
	},
	{
		name: "dark", surface: "#1a1a19", plane: "#0d0d0d",
		inkPrimary: "#ffffff", inkSecondary: "#c3c2b7", muted: "#898781",
		grid: "#2c2c2a", axis: "#383835",
		series: map[string]string{engOurs: "#3987e5", engWasm: "#199e70", engXImage: "#9d78e6", engNative: "#d95c85"},
	},
}

func main() {
	md := flag.String("md", "../results.md", "results.md to read the tables from")
	out := flag.String("out", "../charts", "directory to write the SVGs into")
	flag.Parse()

	sets, err := parseMarkdown(*md)
	if err != nil {
		fatal(err)
	}
	if len(sets) == 0 {
		fatal(fmt.Errorf("no result tables found in %s", *md))
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}

	for _, th := range themes {
		figures := map[string]string{
			"rate-distortion": rateDistortion(sets[0], th),
			"encode-time":     encodeTime(sets, th),
			"decode-time":     decodeTime(sets, th),
			"peak-memory":     peakMemory(sets, th),
			"effort-sweep":    effortSweep(sets, th),
		}
		for name, svg := range figures {
			path := filepath.Join(*out, fmt.Sprintf("%s-%s.svg", name, th.name))
			if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
				fatal(err)
			}
			fmt.Println("wrote", path)
		}
	}
}

// parseMarkdown reads every fenced block that follows a "## " heading and holds
// benchmark rows, taking the heading as the panel label. A heading with several
// blocks under it (timings and peak RSS) yields one dataset: rows carrying the
// same engine/mode/image are merged, so each figure can read the column it wants.
func parseMarkdown(path string) ([]dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var sets []dataset
	byLabel := map[string]int{}
	var heading, caption string
	var fenced, decode, sweep bool

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "## "):
			heading = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			caption = ""
		case strings.HasPrefix(line, "```"):
			if !fenced {
				// The decode table has the same column count as the peak-RSS
				// one, so the caption above it is what tells them apart.
				decode = strings.HasPrefix(caption, "Decode")
				sweep = strings.HasPrefix(caption, "Effort sweep")
			}
			// A table is described by the line above it, so the caption never
			// carries over to the next one.
			caption = ""
			fenced = !fenced
		case fenced:
			r, ok := parseRow(line, decode, sweep)
			if !ok {
				continue
			}
			i, seen := byLabel[heading]
			if !seen {
				i = len(sets)
				byLabel[heading] = i
				sets = append(sets, dataset{label: heading})
			}
			sets[i].merge(r)
		default:
			if line = strings.TrimSpace(line); line != "" {
				caption = line
			}
		}
	}
	return sets, sc.Err()
}

// merge folds a row into the dataset, filling in the columns of an existing row
// for the same measurement rather than adding a duplicate.
func (d *dataset) merge(r row) {
	for i := range d.rows {
		e := &d.rows[i]
		if e.engine != r.engine || e.mode != r.mode || e.file != r.file || e.effort != r.effort {
			continue
		}
		if r.bytes > 0 {
			e.bytes = r.bytes
		}
		if r.ms > 0 {
			e.ms = r.ms
		}
		if r.mibPerMP > 0 {
			e.mibPerMP = r.mibPerMP
		}
		if r.hasPSNR {
			e.psnr, e.hasPSNR = r.psnr, true
		}
		return
	}
	d.rows = append(d.rows, r)
}

// parseRow accepts both the whitespace-aligned tables in results.md and the raw
// comma-separated output of webpbench/rustbench, for the timing table (9 columns)
// and the peak-RSS table (8). A decode row is 8 columns too, but ends in ms/op
// rather than MiB per megapixel, so the caller has to say which it is.
func parseRow(line string, decode, sweep bool) (row, bool) {
	fields := strings.Fields(strings.ReplaceAll(line, ",", " "))
	if len(fields) == 0 || fields[0] == "file" || fields[0] == "engine" {
		return row{}, false
	}
	if sweep {
		return parseSweepRow(fields)
	}
	if len(fields) != 9 && len(fields) != 8 {
		return row{}, false
	}
	// results.md orders columns file,mode,engine; the tools emit engine,mode,file.
	file, mode, engine := fields[0], fields[1], fields[2]
	if strings.HasPrefix(engine, "loss") {
		file, mode, engine = fields[2], fields[1], fields[0]
	}
	if decode {
		mode = decodePrefix + mode
	}
	r := row{engine: engine, mode: mode, file: file}
	if decode {
		ms, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			return row{}, false
		}
		r.ms = ms
		return r, true
	}
	if len(fields) == 8 {
		mibPerMP, err := strconv.ParseFloat(fields[7], 64)
		if err != nil {
			return row{}, false
		}
		r.mibPerMP = mibPerMP
		return r, true
	}
	bytes, err := strconv.Atoi(fields[5])
	if err != nil {
		return row{}, false
	}
	ms, err := strconv.ParseFloat(fields[8], 64)
	if err != nil {
		return row{}, false
	}
	r.bytes, r.ms = bytes, ms
	if psnr, err := strconv.ParseFloat(fields[6], 64); err == nil {
		r.psnr, r.hasPSNR = psnr, true
	}
	return r, true
}

// parseSweepRow reads a sweep line, which carries an effort column the other
// passes do not: file,mode,engine,effort,width,height,bytes,psnr_db,iters,ms_per_op
// in results.md, and engine,mode,effort,file,... straight out of webpbench.
func parseSweepRow(fields []string) (row, bool) {
	if len(fields) != 10 {
		return row{}, false
	}
	// results.md orders columns file,mode,engine,effort; webpbench emits
	// engine,mode,effort,file. Mode sits at index 1 either way, so which layout
	// this is comes down to whether index 2 is the effort number.
	file, mode, engine, effortStr := fields[0], fields[1], fields[2], fields[3]
	if _, err := strconv.Atoi(fields[2]); err == nil {
		file, mode, engine, effortStr = fields[3], fields[1], fields[0], fields[2]
	}
	effort, err := strconv.Atoi(effortStr)
	if err != nil {
		return row{}, false
	}
	bytes, err := strconv.Atoi(fields[6])
	if err != nil {
		return row{}, false
	}
	ms, err := strconv.ParseFloat(fields[9], 64)
	if err != nil {
		return row{}, false
	}
	r := row{engine: engine, mode: sweepPrefix + mode, file: file, effort: effort, bytes: bytes, ms: ms}
	if psnr, err := strconv.ParseFloat(fields[7], 64); err == nil {
		r.psnr, r.hasPSNR = psnr, true
	}
	return r, true
}

func (d dataset) find(engine, mode, file string) (row, bool) {
	for _, r := range d.rows {
		if r.engine == engine && r.mode == mode && r.file == file {
			return r, true
		}
	}
	return row{}, false
}

func (d dataset) files() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range d.rows {
		if !seen[r.file] {
			seen[r.file] = true
			out = append(out, r.file)
		}
	}
	sort.Strings(out)
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "chart:", err)
	os.Exit(1)
}

// shortName trims the pexels-<subject>-<ids> filenames down to the subject.
func shortName(file string) string {
	name := strings.TrimSuffix(file, filepath.Ext(file))
	name = strings.TrimPrefix(name, "pexels-")
	parts := strings.Split(name, "-")
	var words []string
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err == nil {
			break
		}
		words = append(words, p)
	}
	if len(words) == 0 {
		return name
	}
	return strings.Join(words, "-")
}
