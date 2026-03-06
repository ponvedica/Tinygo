// Package sema provides the top-level semantic analysis orchestrator.
//
// Analyze runs all three semantic passes in order:
//  1. Ownership analysis  (move semantics + implicit drops)
//  2. Borrow checker      (aliasing invariants + mutability)
//  3. Lifetime analysis   (reference constraint graph + dangling refs)
//
// Each pass produces []Diagnostic. Analyze collects them all.
// Passes run sequentially; if ownership fails critically, borrow/lifetime
// may produce spurious warnings but still run (for maximum error coverage).
package sema

import (
	"fmt"
	"strings"

	"goforust/ast"
	"goforust/borrow"
	"goforust/lifetime"
	"goforust/ownership"
)

// Result holds all diagnostics from the full semantic analysis.
type Result struct {
	Diagnostics DiagnosticList
	SrcLines    []string // source split into lines (for Display)
	File        string   // source file path
}

// HasErrors returns true if any error-level diagnostic was produced.
func (r Result) HasErrors() bool { return r.Diagnostics.HasErrors() }

// PrintAll prints all diagnostics to a strings.Builder with color.
func (r Result) PrintAll(color bool) string {
	var sb strings.Builder
	for _, d := range r.Diagnostics {
		if color {
			sb.WriteString(d.Display(r.SrcLines))
		} else {
			sb.WriteString(d.PlainDisplay())
			sb.WriteByte('\n')
		}
	}
	if r.Diagnostics.HasErrors() {
		sb.WriteString(fmt.Sprintf("\naborting due to %d error(s)\n",
			r.Diagnostics.ErrorCount()))
	}
	return sb.String()
}

// Analyze runs all three semantic passes and returns a Result.
// src is the raw source bytes (used for source-context display in errors).
// filePath is the name of the source file (for error messages).
func Analyze(file *ast.File, src []byte, filePath string) Result {
	srcLines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")

	var allDiags DiagnosticList

	// ── Pass 1: Ownership ──────────────────────────────────────────────────
	allDiags = append(allDiags, RunOwnership(file, filePath)...)

	// ── Pass 2: Borrow ────────────────────────────────────────────────────
	allDiags = append(allDiags, RunBorrow(file, filePath)...)

	// ── Pass 3: Lifetime ──────────────────────────────────────────────────
	allDiags = append(allDiags, RunLifetime(file, filePath)...)

	return Result{
		Diagnostics: allDiags,
		SrcLines:    srcLines,
		File:        filePath,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Individual pass runners — each wraps a checker and converts its errors
// to Diagnostics.
// ─────────────────────────────────────────────────────────────────────────────

// RunOwnership runs the ownership checker and returns Diagnostics.
func RunOwnership(file *ast.File, filePath string) DiagnosticList {
	oc := ownership.New()
	oc.Check(file)
	var out DiagnosticList
	for _, d := range oc.Diagnostics() {
		out = append(out, Diagnostic{
			Severity: SevError,
			File:     filePath,
			Line:     d.Line,
			Col:      d.Col,
			Message:  d.Message,
			Hint:     d.Hint,
		})
	}
	return out
}

// RunBorrow runs the borrow checker and returns Diagnostics.
func RunBorrow(file *ast.File, filePath string) DiagnosticList {
	bc := borrow.New()
	bc.Check(file)
	var out DiagnosticList
	for _, e := range bc.Errors() {
		out = append(out, Diagnostic{
			Severity: SevError,
			File:     filePath,
			Message:  e,
		})
	}
	return out
}

// RunLifetime runs the lifetime analyzer and returns Diagnostics.
func RunLifetime(file *ast.File, filePath string) DiagnosticList {
	la := lifetime.New()
	la.Analyze(file)
	var out DiagnosticList
	for _, e := range la.Errors() {
		out = append(out, Diagnostic{
			Severity: SevError,
			File:     filePath,
			Message:  e,
		})
	}
	return out
}
