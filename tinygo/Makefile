# TinyGo — Demo Makefile
# Usage:
#   make          → build everything
#   make demo     → build + open the demo in browser
#   make server   → just run the server
#   make clean    → remove binaries

GO      = go
GOFLAGS = GOTMPDIR=$(PWD)/.tmp GOCACHE=$(PWD)/.cache/go-build GOPATH=$(PWD)/.cache
GORUN   = $(GOFLAGS) $(GO)

.PHONY: all demo server clean setup

all: setup tinygo server
	@echo ""
	@echo "✓ Built: ./tinygo  (compiler)"
	@echo "✓ Built: ./server  (demo server)"
	@echo ""
	@echo "  Run:  make demo"

setup:
	@mkdir -p .tmp .cache/go-build

tinygo: setup
	@echo "→ Building tinygo compiler…"
	@$(GORUN) build -o tinygo .
	@echo "  ✓ tinygo"

server: setup
	@echo "→ Building demo server…"
	@$(GORUN) build -o server ./cmd/server/
	@echo "  ✓ server"

demo: all
	@echo ""
	@echo "🔧 Starting TinyGo Compiler Server on http://localhost:8080"
	@echo "   Opening demo.html in your browser…"
	@echo "   Press Ctrl+C to stop"
	@echo ""
	@open demo.html &
	@./server

server-only:
	@echo "🔧 Starting server on http://localhost:8080 (press Ctrl+C to stop)"
	@./server

clean:
	@rm -f tinygo server
	@rm -rf .tmp .cache
	@echo "✓ Cleaned"
