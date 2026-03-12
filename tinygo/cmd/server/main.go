package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CompileRequest is the JSON body sent from the browser
type CompileRequest struct {
	Code    string `json:"code"`
	Command string `json:"command"` // "build", "lex", "parse", "emit"
}

// CompileResponse is sent back to the browser
type CompileResponse struct {
	Success     bool   `json:"success"`
	Output      string `json:"output"`
	BinarySize  string `json:"binarySize"`
	CompileTime string `json:"compileTime"`
	Command     string `json:"command"`
	Stage       string `json:"stage"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/compile", withCORS(handleCompile))
	mux.HandleFunc("/health", withCORS(handleHealth))

	port := "8080"
	fmt.Printf("\n🔧 TinyGo Compiler Server\n")
	fmt.Printf("   Listening on http://localhost:%s\n\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleCompile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CompileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "invalid request body: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Code) == "" {
		sendError(w, "no code provided")
		return
	}

	// Basic size guard
	if len(req.Code) > 64*1024 {
		sendError(w, "code too large (max 64 KB)")
		return
	}

	// Default to build
	if req.Command == "" {
		req.Command = "build"
	}

	// Write code to a temp file
	tmpDir, err := os.MkdirTemp("", "tinygo_*")
	if err != nil {
		sendError(w, "failed to create temp dir: "+err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(srcFile, []byte(req.Code), 0644); err != nil {
		sendError(w, "failed to write source: "+err.Error())
		return
	}

	// Resolve tinygo binary — look next to this server binary first, then PATH
	exe, _ := os.Executable()
	tinygoPath := filepath.Join(filepath.Dir(exe), "tinygo")
	if _, err := os.Stat(tinygoPath); os.IsNotExist(err) {
		// try project root (two levels up from cmd/server)
		tinygoPath = filepath.Join(filepath.Dir(exe), "..", "..", "tinygo")
	}
	if _, err := os.Stat(tinygoPath); os.IsNotExist(err) {
		tinygoPath = "tinygo" // fall back to PATH
	}

	var args []string
	outputBin := ""
	isRun := req.Command == "run"

	switch req.Command {
	case "lex":
		args = []string{"lex", srcFile}
	case "parse":
		args = []string{"parse", srcFile}
	case "emit":
		args = []string{"emit", srcFile}
	case "run", "build":
		outputBin = filepath.Join(tmpDir, "out")
		args = []string{"build", "-o", outputBin, srcFile}
	}

	// 10-second timeout per compile
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, tinygoPath, args...)
	cmd.Dir = tmpDir

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	elapsed := time.Since(start)

	// Compile-time stderr (warnings etc.)
	compileOut := strings.TrimSpace(outBuf.String())
	compileErr := strings.TrimSpace(errBuf.String())

	success := runErr == nil

	// Get binary size
	binarySize := ""
	if success && outputBin != "" {
		if info, err := os.Stat(outputBin); err == nil {
			bytes := info.Size()
			switch {
			case bytes < 1024:
				binarySize = fmt.Sprintf("%d B", bytes)
			case bytes < 1024*1024:
				binarySize = fmt.Sprintf("%.1f KB", float64(bytes)/1024)
			default:
				binarySize = fmt.Sprintf("%.2f MB", float64(bytes)/1024/1024)
			}
		}
	}

	var combined string

	if !success {
		// Compilation failed — show errors
		combined = compileErr
		if combined == "" {
			combined = compileOut
		}
	} else if isRun {
		// Run the compiled binary and capture output
		runCtx, runCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer runCancel()
		runCmd := exec.CommandContext(runCtx, outputBin)
		runCmd.Dir = tmpDir
		var runOut, runErrBuf strings.Builder
		runCmd.Stdout = &runOut
		runCmd.Stderr = &runErrBuf
		runErr2 := runCmd.Run()
		prog := strings.TrimSpace(runOut.String())
		progErr := strings.TrimSpace(runErrBuf.String())
		if prog != "" {
			combined = prog
		}
		if progErr != "" {
			if combined != "" {
				combined += "\n" + progErr
			} else {
				combined = progErr
			}
		}
		if runErr2 != nil && combined == "" {
			combined = fmt.Sprintf("runtime error: %v", runErr2)
		}
		if combined == "" {
			combined = "(program produced no output)"
		}
		// Prepend any compile warnings
		if compileErr != "" {
			combined = "[compiler warnings]\n" + compileErr + "\n\n[program output]\n" + combined
		}
	} else {
		// lex/parse/emit write their output to stdout (compileOut).
		// build produces no stdout — show "Build successful" in that case.
		if compileOut != "" {
			combined = compileOut
			if compileErr != "" {
				combined += "\n\n[warnings]\n" + compileErr
			}
		} else {
			combined = "✓ Build successful"
			if compileErr != "" {
				combined += "\n\n[warnings]\n" + compileErr
			}
		}
	}

	resp := CompileResponse{
		Success:     success,
		Output:      combined,
		BinarySize:  binarySize,
		CompileTime: fmt.Sprintf("%.3fs", elapsed.Seconds()),
		Command:     req.Command,
		Stage:       stageLabel(req.Command),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func stageLabel(cmd string) string {
	switch cmd {
	case "lex":
		return "Lexer"
	case "parse":
		return "Parser"
	case "emit":
		return "CodeGen"
	case "run":
		return "Run"
	default:
		return "Full Build"
	}
}

func sendError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CompileResponse{
		Success: false,
		Output:  "error: " + msg,
	})
}

func withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		h(w, r)
	}
}


