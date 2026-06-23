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

# Build the desktop binary through GoReleaser (no release/publish). This reuses
# the same pipeline as the release workflow — the `before` hooks in
# .goreleaser.yaml regenerate the bindings and build the embedded frontend, so
# this does NOT depend on desktop-frontend. --single-target builds only for the
# host platform; -o copies the binary to the usual build/ path.
desktop:
	@mkdir -p build
	goreleaser build --snapshot --clean --single-target --id wfo-desktop -o build/wfo-desktop

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
	rm -rf build dist
