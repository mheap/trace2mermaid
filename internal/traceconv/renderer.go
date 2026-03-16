package traceconv

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"
)

// mermaidWriter wraps an io.Writer and captures the first error encountered.
type mermaidWriter struct {
	w   io.Writer
	err error
}

func (mw *mermaidWriter) printf(format string, args ...any) {
	if mw.err != nil {
		return
	}
	_, mw.err = fmt.Fprintf(mw.w, format, args...)
}

// Render takes a parsed Trace and writes a Mermaid Gantt diagram to the writer.
func Render(w io.Writer, t *Trace) error {
	if len(t.Spans) == 0 {
		return fmt.Errorf("no spans to render")
	}

	// Group spans by service, preserving order of first appearance
	type serviceGroup struct {
		name  string
		spans []ServiceSpan
	}
	serviceOrder := []string{}
	serviceMap := map[string]*serviceGroup{}

	// Sort all spans by start time first so services appear in order of their earliest span
	sorted := make([]ServiceSpan, len(t.Spans))
	copy(sorted, t.Spans)
	slices.SortStableFunc(sorted, func(a, b ServiceSpan) int {
		return cmp.Compare(a.StartTimeNano, b.StartTimeNano)
	})

	for _, s := range sorted {
		if _, exists := serviceMap[s.ServiceName]; !exists {
			serviceOrder = append(serviceOrder, s.ServiceName)
			serviceMap[s.ServiceName] = &serviceGroup{name: s.ServiceName}
		}
		serviceMap[s.ServiceName].spans = append(serviceMap[s.ServiceName].spans, s)
	}

	// sorted[0] has the earliest start time since we sorted by StartTimeNano above.
	minStart := sorted[0].StartTimeNano

	mw := &mermaidWriter{w: w}

	// Write header.
	// mmdr only renders proportional bars with dateFormat YYYY-MM-DD and Nd
	// durations. We map 1 millisecond of trace time to 1 day in Mermaid.
	mw.printf("gantt\n")
	mw.printf("    title Trace %s\n", t.TraceID)
	mw.printf("    dateFormat YYYY-MM-DD\n")

	baseDate := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	// Write each service section
	for _, svcName := range serviceOrder {
		group := serviceMap[svcName]
		mw.printf("\n")
		mw.printf("    section %s\n", svcName)

		for _, span := range group.spans {
			offsetMs := nanoToMs(span.StartTimeNano - minStart)
			durationMs := nanoToMs(span.EndTimeNano - span.StartTimeNano)

			// Ensure at least 1 day duration for visibility (zero-duration spans are invisible)
			if durationMs == 0 {
				durationMs = 1
			}

			startDate := baseDate.AddDate(0, 0, int(offsetMs))
			startStr := startDate.Format("2006-01-02")

			name := escapeMermaid(span.Name)
			tag := spanTag(span)

			if tag != "" {
				mw.printf("    %-45s :%s, %s, %dd\n", name, tag, startStr, durationMs)
			} else {
				mw.printf("    %-45s :%s, %dd\n", name, startStr, durationMs)
			}
		}
	}

	return mw.err
}

// nanoToMs converts nanoseconds to milliseconds, rounding to nearest.
func nanoToMs(nano uint64) int64 {
	return int64(math.Round(float64(nano) / 1_000_000.0))
}

// spanTag returns the Mermaid styling tag for a span.
func spanTag(s ServiceSpan) string {
	if s.ParentSpanID == "" {
		return "crit"
	}
	if s.StatusCode == 2 {
		return "done"
	}
	return ""
}

// escapeMermaid escapes characters that are special in Mermaid syntax.
func escapeMermaid(name string) string {
	delimiters := map[rune]bool{
		';': true,
		'#': true,
		'(': true,
		')': true,
		'[': true,
		']': true,
		'{': true,
		'}': true,
		'|': true,
		'>': true,
		'<': true,
		'"': true,
		':': true,
	}

	var ret strings.Builder
	for _, c := range name {
		if !delimiters[c] {
			ret.WriteString(string(c))
			continue
		}

		fmt.Fprintf(&ret, "#%d;", c)
	}

	return ret.String()
}
