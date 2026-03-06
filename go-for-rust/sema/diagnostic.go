// Package sema provides the Diagnostic type for the Ore compiler.
//
// Diagnostics carry structured source location info and display in
// Rustc-style format:
//
//	error: cannot use `x` after move
//	  --> main.ore:5:3
//	   |
//	 5 |     println(x);
//	   |             ^ value used here after move
package sema

import (
	"fmt"
	"strings"
)

// Severity classifies the importance of a diagnostic.
type Severity int

const (
	SevError   Severity = iota // compilation must stop
	SevWarning                 // non-fatal issue
	SevNote                    // informational hint
)

func (s Severity) String() string {
	switch s {
	case SevError:
		return "error"
	case SevWarning:
		return "warning"
	case SevNote:
		return "note"
	}
	return "?"
}

// Diagnostic is a structured compiler message with source location.
type Diagnostic struct {
	Severity Severity
	File     string
	Line     int    // 1-based; 0 = unknown
	Col      int    // 1-based; 0 = unknown
	Message  string // primary message
	Hint     string // optional "help:" suffix
}

// Display renders a Rustc-style diagnostic string, using srcLines to show
// the actual source line. srcLines may be nil.
func (d Diagnostic) Display(srcLines []string) string {
	var sb strings.Builder

	// Header: "error: message"
	sb.WriteString(fmt.Sprintf("\x1b[1;31m%s\x1b[0m: %s\n", d.Severity, d.Message))

	// Location arrow: "  --> file:line:col"
	if d.Line > 0 {
		file := d.File
		if file == "" {
			file = "<unknown>"
		}
		sb.WriteString(fmt.Sprintf("  \x1b[34m-->\x1b[0m %s:%d:%d\n", file, d.Line, d.Col))

		// Show source line if available.
		if srcLines != nil && d.Line-1 < len(srcLines) {
			lineNum := fmt.Sprintf("%d", d.Line)
			padding := strings.Repeat(" ", len(lineNum))

			sb.WriteString(fmt.Sprintf("   \x1b[34m%s|\x1b[0m\n", padding))
			src := srcLines[d.Line-1]
			sb.WriteString(fmt.Sprintf("\x1b[34m%s |\x1b[0m %s\n", lineNum, src))

			// Caret pointer at column.
			if d.Col > 0 {
				caretOffset := strings.Repeat(" ", d.Col-1)
				sb.WriteString(fmt.Sprintf("   \x1b[34m%s|\x1b[0m %s\x1b[1;31m^\x1b[0m\n", padding, caretOffset))
			}
		}
	}

	// Optional help hint.
	if d.Hint != "" {
		sb.WriteString(fmt.Sprintf("   \x1b[32m= help:\x1b[0m %s\n", d.Hint))
	}

	return sb.String()
}

// PlainDisplay returns the diagnostic without ANSI color codes.
func (d Diagnostic) PlainDisplay() string {
	msg := fmt.Sprintf("%s: %s", d.Severity, d.Message)
	if d.Line > 0 {
		file := d.File
		if file == "" {
			file = "<unknown>"
		}
		msg += fmt.Sprintf("\n  --> %s:%d:%d", file, d.Line, d.Col)
	}
	if d.Hint != "" {
		msg += "\n  = help: " + d.Hint
	}
	return msg
}

// DiagnosticList is a slice of Diagnostic with helper methods.
type DiagnosticList []Diagnostic

// HasErrors returns true if any diagnostic has SevError severity.
func (dl DiagnosticList) HasErrors() bool {
	for _, d := range dl {
		if d.Severity == SevError {
			return true
		}
	}
	return false
}

// ErrorCount returns the number of error-level diagnostics.
func (dl DiagnosticList) ErrorCount() int {
	n := 0
	for _, d := range dl {
		if d.Severity == SevError {
			n++
		}
	}
	return n
}

// Strings converts each diagnostic to a plain string.
func (dl DiagnosticList) Strings() []string {
	out := make([]string, len(dl))
	for i, d := range dl {
		out[i] = d.PlainDisplay()
	}
	return out
}

// MakeDiag is a convenience constructor.
func MakeDiag(sev Severity, file string, line, col int, msg string) Diagnostic {
	return Diagnostic{Severity: sev, File: file, Line: line, Col: col, Message: msg}
}

// Error creates an error-severity Diagnostic.
func Error(file string, line, col int, msg string, args ...interface{}) Diagnostic {
	return MakeDiag(SevError, file, line, col, fmt.Sprintf(msg, args...))
}

// Warning creates a warning Diagnostic.
func Warning(file string, line, col int, msg string, args ...interface{}) Diagnostic {
	return MakeDiag(SevWarning, file, line, col, fmt.Sprintf(msg, args...))
}
