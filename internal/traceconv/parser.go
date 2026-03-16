// Package traceconv provides OTLP JSON trace parsing and Mermaid Gantt rendering.
package traceconv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// OTLP JSON types

// AttributeValue represents a single OTLP attribute value.
type AttributeValue struct {
	StringValue string `json:"stringValue"`
	IntValue    string `json:"intValue"`
}

// KeyValue represents a key-value pair in OTLP resource or span attributes.
type KeyValue struct {
	Key   string         `json:"key"`
	Value AttributeValue `json:"value"`
}

// Resource represents the OTLP resource associated with a set of spans.
type Resource struct {
	Attributes []KeyValue `json:"attributes"`
}

// Status represents the status of an OTLP span.
type Status struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Span represents a single OTLP span.
type Span struct {
	TraceID           string     `json:"traceId"`
	SpanID            string     `json:"spanId"`
	ParentSpanID      string     `json:"parentSpanId"`
	Name              string     `json:"name"`
	Kind              int        `json:"kind"`
	StartTimeUnixNano string     `json:"startTimeUnixNano"`
	EndTimeUnixNano   string     `json:"endTimeUnixNano"`
	Status            Status     `json:"status"`
	Attributes        []KeyValue `json:"attributes"`
}

// ScopeSpan represents a collection of spans from a single instrumentation scope.
type ScopeSpan struct {
	Spans []Span `json:"spans"`
}

// ResourceSpan represents a collection of scope spans from a single resource.
type ResourceSpan struct {
	Resource   Resource    `json:"resource"`
	ScopeSpans []ScopeSpan `json:"scopeSpans"`
}

// TracesData is the top-level OTLP JSON structure containing resource spans.
type TracesData struct {
	ResourceSpans []ResourceSpan `json:"resourceSpans"`
}

// Parsed output types

// ServiceSpan is a flattened span with its associated service name.
type ServiceSpan struct {
	ServiceName   string
	Name          string
	TraceID       string
	SpanID        string
	ParentSpanID  string
	StartTimeNano uint64
	EndTimeNano   uint64
	StatusCode    int
}

// Trace holds the parsed trace data: a trace ID and all associated spans.
type Trace struct {
	TraceID string
	Spans   []ServiceSpan
}

func getServiceName(r Resource) string {
	for _, attr := range r.Attributes {
		if attr.Key == "service.name" {
			return attr.Value.StringValue
		}
	}
	return "unknown"
}

// Parse reads OTLP JSON Lines from r and returns a parsed Trace.
func Parse(r io.Reader) (*Trace, error) {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for large JSON lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	var allSpans []ServiceSpan
	var traceID string

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var data TracesData
		if err := json.Unmarshal(line, &data); err != nil {
			return nil, fmt.Errorf("failed to parse JSON line: %w", err)
		}

		for _, rs := range data.ResourceSpans {
			serviceName := getServiceName(rs.Resource)

			for _, ss := range rs.ScopeSpans {
				for _, span := range ss.Spans {
					startNano, err := strconv.ParseUint(span.StartTimeUnixNano, 10, 64)
					if err != nil {
						return nil, fmt.Errorf("failed to parse startTimeUnixNano %q: %w", span.StartTimeUnixNano, err)
					}
					endNano, err := strconv.ParseUint(span.EndTimeUnixNano, 10, 64)
					if err != nil {
						return nil, fmt.Errorf("failed to parse endTimeUnixNano %q: %w", span.EndTimeUnixNano, err)
					}

					if traceID == "" && span.TraceID != "" {
						traceID = span.TraceID
					}

					allSpans = append(allSpans, ServiceSpan{
						ServiceName:   serviceName,
						Name:          span.Name,
						TraceID:       span.TraceID,
						SpanID:        span.SpanID,
						ParentSpanID:  span.ParentSpanID,
						StartTimeNano: startNano,
						EndTimeNano:   endNano,
						StatusCode:    span.Status.Code,
					})
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	if len(allSpans) == 0 {
		return nil, fmt.Errorf("no spans found in input")
	}

	return &Trace{
		TraceID: traceID,
		Spans:   allSpans,
	}, nil
}
