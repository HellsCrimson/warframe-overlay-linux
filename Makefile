GO ?= go

.PHONY: all build test vet fmt clean desktop desktop-frontend run-desktop

all: build

# --- Wails3 desktop app (cmd/wfo-desktop) ---------------------------------
# The frontend (Svelte + Vite) is built and embedded into the Go binary. The
# JS bindings are generated from the Go service. Requires node/npm, webkit2gtk
# and the wails3 CLI.
DESKTOP_DIR := cmd/wfo-desktop

desktop-frontend:
	cd $(DESKTOP_DIR) && wails3 generate bindings -d ./frontend/bindings
	cd $(DESKTOP_DIR)/frontend && npm install && npm run build

desktop: desktop-frontend
	$(GO) build -o build/wfo-desktop ./$(DESKTOP_DIR)

# Run the desktop app. Pass INV to load a saved inventory (dev, no game needed):
#   make run-desktop INV=dump/inventory.json
# The relic overlay still watches EE.log; add flags after the binary as needed
# (e.g. -dump dump/ -v -no-overlay).
run-desktop: desktop
	WFO_INVENTORY_FILE='$(INV)' ./build/wfo-desktop


build:
	$(GO) build ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w ./cmd ./internal

clean:
	rm -rf build
