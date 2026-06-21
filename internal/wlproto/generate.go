// Package wlproto holds Wayland protocol bindings generated from the vendored
// XML in third_party/protocols/ by go-wayland-scanner.
//
// Install the scanner once:
//
//	go install github.com/rajveermalviya/go-wayland/cmd/go-wayland-scanner@latest
//
// Then regenerate with `go generate ./...`. Note: the vendored
// ext-image-capture-source-v1.xml has had its ext_foreign_toplevel_* manager
// interface removed, because it references a foreign-toplevel handle type we do
// not vendor and do not need (we only capture whole outputs).
package wlproto

//go:generate go-wayland-scanner -i ../../third_party/protocols/wlr-screencopy-unstable-v1.xml -o screencopy/screencopy.go -pkg screencopy
//go:generate go-wayland-scanner -i ../../third_party/protocols/ext-image-capture-source-v1.xml -o extcapture/source.go -pkg extcapture
//go:generate go-wayland-scanner -i ../../third_party/protocols/ext-image-copy-capture-v1.xml -o extcapture/copy.go -pkg extcapture
//go:generate go-wayland-scanner -i ../../third_party/protocols/color-management-v1.xml -o colormgmt/colormgmt.go -pkg colormgmt
//go:generate go-wayland-scanner -i ../../third_party/protocols/wlr-layer-shell-unstable-v1.xml -o layershell/layershell.go -pkg layershell
