// Package format provides formatting utilities used throughout the CowGnition project.
package format

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
)

// FormatMarkdownTable creates a markdown table from headers and rows.
// Returns an error if headers or rows are empty.
func FormatMarkdownTable(headersstring, rowsstring) (string, error) {
	if len(headers) == 0 {
		return "", fmt.Errorf("FormatMarkdownTable: headers are empty")
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("FormatMarkdownTable: rows are empty")
	}

	var buf strings.Builder

	// Write headers.
	buf.WriteString("| ")
	buf.WriteString(strings.Join(headers, " | "))
	buf.WriteString(" |\n")

	// Write separator.
	buf.WriteString("| ")
	for range headers {
		buf.WriteString("--- | ")
	}
	buf.WriteString("\n")

	// Write rows.
	for _, row := range rows {
		// Ensure row has the right number of columns.
		for len(row) < len(headers) {
			row = append(row, "")
		}

		buf.WriteString("| ")
		buf.WriteString(strings.Join(row, " | "))
		buf.WriteString(" |\n")
	}

	return buf.String(), nil
}

// FormatTaskPriority formats a task priority code as a human-readable string.
// RTM uses "1" for highest priority, "N" for no priority.
func FormatTaskPriority(priority string) string {
	switch priority {
	case "1":
		return "High"
	case "2":
		return "Medium"
	case "3":
		return "Low"
	case "N", "":
		return "None"
	default:
		return priority
	}
}

// FormatColumns formats text in evenly-spaced columns using tabwriter.
// Returns an error if headers or rows are empty.
func FormatColumns(headersstring, rowsstring) (string, error) {
	if len(headers) == 0 {
		return "", fmt.Errorf("FormatColumns: headers are empty")
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("FormatColumns: rows are empty")
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	// Write headers.
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Write a separator.
	sep := make(string, len(headers))
	for i := range sep {
		sep[i] = strings.Repeat("-", len(headers[i]))
	}
	fmt.Fprintln(w, strings.Join(sep, "\t"))

	// Write rows.
	for _, row := range rows {
		// Ensure row has the right number of columns.
		for len(row) < len(headers) {
			row = append(row, "")
		}
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	w.Flush()
	return buf.String(), nil
}

// ErrorMsgEnhanced:2025-03-17
