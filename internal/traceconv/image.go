package traceconv

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	mermaid "github.com/bvolpato/mermaid-go-renderer"
)

// baseDate is the reference date used to map millisecond offsets to calendar
// dates. The renderer maps 1 ms of trace time to 1 day from this date.
var baseDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// datePattern matches YYYY-MM-DD date strings in the SVG output so they can
// be replaced with the corresponding millisecond offset.
var datePattern = regexp.MustCompile(`>\d{4}-\d{2}-\d{2}<`)

// viewBoxPattern matches the viewBox attribute in the SVG root element.
var viewBoxPattern = regexp.MustCompile(`viewBox="([0-9.-]+)\s+([0-9.-]+)\s+([0-9.-]+)\s+([0-9.-]+)"`)

// tickGroupPattern matches entire tick <g> groups including their children.
var tickGroupPattern = regexp.MustCompile(`<g class="tick"[^>]*>.*?</g>`)

// gridTranslatePattern extracts the y-offset from the grid group's transform.
var gridTranslatePattern = regexp.MustCompile(`<g class="grid" transform="translate\([0-9.]+,\s*([0-9.]+)\)"`)

// tickTextYPattern matches the y attribute on tick text elements.
var tickTextYPattern = regexp.MustCompile(`(<g class="tick"[^>]*>.*?<text[^>]*)\by="[0-9.]+"`)

// tickLineY2Pattern matches the y2 attribute on tick line elements.
var tickLineY2Pattern = regexp.MustCompile(`(<g class="tick"[^>]*>.*?<line[^>]*)\by2="[0-9.-]+"`)

// RenderSVG renders the trace as an SVG image, writing to w.
func RenderSVG(w io.Writer, t *Trace) error {
	var buf bytes.Buffer
	if err := Render(&buf, t); err != nil {
		return err
	}

	svg, err := mermaid.Render(buf.String())
	if err != nil {
		return fmt.Errorf("rendering mermaid diagram: %w", err)
	}

	svg = replaceDateLabels(svg)
	svg = decodeMermaidEntities(svg)
	svg = thinTicks(svg)
	svg = fixGrid(svg)
	svg = fixViewBox(svg)

	_, err = io.WriteString(w, svg)
	return err
}

// mermaidEntityPattern matches Mermaid entity references like #58; (colon)
// and #59; (semicolon). These are not decoded by the mmdg renderer.
var mermaidEntityPattern = regexp.MustCompile(`#(\d+);`)

// decodeMermaidEntities replaces Mermaid entity references (#<charcode>;)
// in the SVG with their corresponding characters. The mmdg renderer passes
// these through literally rather than decoding them.
func decodeMermaidEntities(svg string) string {
	return mermaidEntityPattern.ReplaceAllStringFunc(svg, func(match string) string {
		codeStr := match[1 : len(match)-1]
		code, err := strconv.Atoi(codeStr)
		if err != nil || code < 32 || code > 126 {
			return match
		}
		return string(rune(code))
	})
}

// replaceDateLabels replaces all YYYY-MM-DD axis labels in the SVG with
// their corresponding millisecond offset from the base date.
func replaceDateLabels(svg string) string {
	return datePattern.ReplaceAllStringFunc(svg, func(match string) string {
		dateStr := match[1 : len(match)-1]
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return match
		}
		days := int(parsed.Sub(baseDate).Hours() / 24)
		return ">" + strconv.Itoa(days) + "ms<"
	})
}

// thinTicks reduces the number of tick marks in the SVG to at most maxTicks.
// mmdg generates one tick per 2 days (= 2 ms) which creates hundreds of
// overlapping vertical lines. We keep only a handful of evenly spaced ticks.
func thinTicks(svg string) string {
	const maxTicks = 8

	ticks := tickGroupPattern.FindAllStringIndex(svg, -1)
	if len(ticks) <= maxTicks {
		return svg
	}

	keep := make(map[int]bool)
	for i := 0; i < maxTicks; i++ {
		idx := int(math.Round(float64(i) * float64(len(ticks)-1) / float64(maxTicks-1)))
		keep[idx] = true
	}

	var result bytes.Buffer
	result.Grow(len(svg))
	lastEnd := 0
	for i, loc := range ticks {
		if keep[i] {
			continue
		}
		result.WriteString(svg[lastEnd:loc[0]])
		lastEnd = loc[1]
	}
	result.WriteString(svg[lastEnd:])

	return result.String()
}

// fixGrid repositions x-axis tick text labels below the chart content and
// extends tick lines to span the full chart height.
//
// mmdg places the grid group at a y position in the middle of the chart. The
// tick text has a small y offset which puts labels behind section background
// rects, and tick lines only cover a fraction of the chart height.
func fixGrid(svg string) string {
	gridMatch := gridTranslatePattern.FindStringSubmatch(svg)
	if len(gridMatch) < 2 {
		return svg
	}
	gridY, _ := strconv.ParseFloat(gridMatch[1], 64)

	contentBottom := findMaxYExtent(svg)
	contentTop := findMinYExtent(svg)

	// Move tick text labels below all chart content.
	const labelPadding = 5
	newTickTextY := contentBottom - gridY + labelPadding
	svg = tickTextYPattern.ReplaceAllStringFunc(svg, func(match string) string {
		sub := tickTextYPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return sub[1] + fmt.Sprintf(`y="%.1f"`, newTickTextY)
	})

	// Extend tick lines from content top to content bottom (relative to grid).
	newY2 := fmt.Sprintf(`y2="%.1f"`, contentTop-gridY)
	svg = tickLineY2Pattern.ReplaceAllStringFunc(svg, func(match string) string {
		sub := tickLineY2Pattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return sub[1] + newY2
	})

	return svg
}

// fixViewBox works around a mmdg bug where the Gantt chart viewBox height is
// too small, causing content to be clipped. It accounts for both direct
// element positions and elements inside the grid group (which uses a
// translate transform).
func fixViewBox(svg string) string {
	vbMatch := viewBoxPattern.FindStringSubmatch(svg)
	if len(vbMatch) < 5 {
		return svg
	}

	vbH, _ := strconv.ParseFloat(vbMatch[4], 64)

	maxExtent := findMaxYExtent(svg)

	// Account for tick text inside the grid group, which is positioned
	// relative to the grid's translate-y transform.
	gridMatch := gridTranslatePattern.FindStringSubmatch(svg)
	if len(gridMatch) >= 2 {
		gridY, _ := strconv.ParseFloat(gridMatch[1], 64)
		for _, tick := range tickGroupPattern.FindAllString(svg, -1) {
			for _, m := range textYExtent.FindAllStringSubmatch(tick, -1) {
				y, _ := strconv.ParseFloat(m[1], 64)
				const tickTextHeight = 20 // accounts for dy="1em" + font size
				if absBottom := gridY + y + tickTextHeight; absBottom > maxExtent {
					maxExtent = absBottom
				}
			}
		}
	}

	const padding = 10
	neededH := maxExtent + padding
	if neededH <= vbH {
		return svg
	}

	oldVB := fmt.Sprintf(`viewBox="%s %s %s %s"`, vbMatch[1], vbMatch[2], vbMatch[3], vbMatch[4])
	newVB := fmt.Sprintf(`viewBox="%s %s %s %.1f"`, vbMatch[1], vbMatch[2], vbMatch[3], neededH)
	return strings.Replace(svg, oldVB, newVB, 1)
}

// rectYExtent matches rect elements with y and height attributes to compute
// the bottom edge of section/task rectangles.
var rectYExtent = regexp.MustCompile(`<rect[^>]*\by="([0-9.]+)"[^>]*\bheight="([0-9.]+)"`)

// textYExtent matches text elements with a y attribute (excluding tick text
// inside the grid group, which is positioned relative to its parent transform).
var textYExtent = regexp.MustCompile(`<text[^>]*\by="([0-9.]+)"`)

// findMinYExtent finds the topmost y position of section rects in the chart.
func findMinYExtent(svg string) float64 {
	minY := math.MaxFloat64
	for _, m := range rectYExtent.FindAllStringSubmatch(svg, -1) {
		y, _ := strconv.ParseFloat(m[1], 64)
		if y < minY {
			minY = y
		}
	}
	if minY == math.MaxFloat64 {
		return 0
	}
	return minY
}

// findMaxYExtent determines the maximum vertical extent of visible content
// by scanning rect bottom edges (y+height) and text y positions.
func findMaxYExtent(svg string) float64 {
	var maxY float64

	// Check bottom edges of all rects.
	for _, m := range rectYExtent.FindAllStringSubmatch(svg, -1) {
		y, _ := strconv.ParseFloat(m[1], 64)
		h, _ := strconv.ParseFloat(m[2], 64)
		if bottom := y + h; bottom > maxY {
			maxY = bottom
		}
	}

	// Check text positions (add a small amount for line height).
	const textLineHeight = 16
	for _, m := range textYExtent.FindAllStringSubmatch(svg, -1) {
		y, _ := strconv.ParseFloat(m[1], 64)
		if bottom := y + textLineHeight; bottom > maxY {
			maxY = bottom
		}
	}

	return maxY
}
