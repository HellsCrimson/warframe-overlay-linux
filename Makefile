# The Gio companion app (cmd/wfo-app) links Gio, which compiles a Vulkan backend
# by default and needs the Vulkan dev headers (vulkan-headers). To build without
# them, pass -tags novulkan to fall back to OpenGL ES. We default to that so the
# whole module builds out of the box; install vulkan-headers and drop the tag to
# use the (faster) Vulkan backend.
GOTAGS ?= novulkan
GO ?= go

.PHONY: all build test vet fmt app run-app clean

all: build

build:
	$(GO) build -tags '$(GOTAGS)' ./...

test:
	$(GO) test -tags '$(GOTAGS)' ./...

vet:
	$(GO) vet -tags '$(GOTAGS)' ./...

fmt:
	gofmt -w ./cmd ./internal

# Build the binaries into ./bin.
app:
	$(GO) build -tags '$(GOTAGS)' -o bin/wfo ./cmd/wfo
	$(GO) build -tags '$(GOTAGS)' -o bin/wfo-app ./cmd/wfo-app

# Run the companion app against a saved inventory (dev, no game needed):
#   make run-app INV=dump/inventory.json
run-app:
	$(GO) run -tags '$(GOTAGS)' ./cmd/wfo-app -inventory-file '$(INV)'

clean:
	rm -rf bin
