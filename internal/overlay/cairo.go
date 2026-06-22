package overlay

/*
#cgo pkg-config: cairo pangocairo
#include <stdlib.h>
#include <string.h>
#include <cairo/cairo.h>
#include <pango/pangocairo.h>

// wfo_draw renders the overlay into a premultiplied ARGB32 buffer (matching
// wl_shm ARGB8888). The buffer is first cleared to fully transparent; each label
// is drawn as a rounded, nearly-opaque background with centered Pango-markup
// text (small dim item name above a large bold price). Best picks get a gold
// border and gold price.
static void wfo_draw(unsigned char *data, int w, int h, int stride,
                     int n, const int *cx, const int *top,
                     const char **markups, const int *best) {
    cairo_surface_t *surface = cairo_image_surface_create_for_data(
        data, CAIRO_FORMAT_ARGB32, w, h, stride);
    cairo_t *cr = cairo_create(surface);

    // Clear to transparent.
    cairo_set_operator(cr, CAIRO_OPERATOR_CLEAR);
    cairo_paint(cr);
    cairo_set_operator(cr, CAIRO_OPERATOR_OVER);

    PangoLayout *layout = pango_cairo_create_layout(cr);

    for (int i = 0; i < n; i++) {
        pango_layout_set_alignment(layout, PANGO_ALIGN_CENTER);
        pango_layout_set_markup(layout, markups[i], -1);
        int tw, th;
        pango_layout_get_pixel_size(layout, &tw, &th);

        double padx = 14.0, pady = 9.0;
        double bw = tw + 2 * padx;
        double bh = th + 2 * pady;
        double bx = cx[i] - bw / 2.0;
        double by = (double)top[i];

        // Rounded-rect background (nearly opaque for quick legibility).
        double r = 9.0;
        cairo_new_sub_path(cr);
        cairo_arc(cr, bx + bw - r, by + r, r, -G_PI_2, 0);
        cairo_arc(cr, bx + bw - r, by + bh - r, r, 0, G_PI_2);
        cairo_arc(cr, bx + r, by + bh - r, r, G_PI_2, G_PI);
        cairo_arc(cr, bx + r, by + r, r, G_PI, 3 * G_PI_2);
        cairo_close_path(cr);

        cairo_set_source_rgba(cr, 0.04, 0.04, 0.06, 0.94);
        cairo_fill_preserve(cr);
        if (best[i]) {
            cairo_set_source_rgba(cr, 1.0, 0.84, 0.0, 0.98); // gold
            cairo_set_line_width(cr, 3.0);
        } else {
            cairo_set_source_rgba(cr, 0.45, 0.45, 0.5, 0.7);
            cairo_set_line_width(cr, 1.2);
        }
        cairo_stroke(cr);

        cairo_move_to(cr, bx + padx, by + pady);
        pango_cairo_show_layout(cr, layout);
    }

    g_object_unref(layout);
    cairo_destroy(cr);
    cairo_surface_destroy(surface);
}
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

// Label is one rendered price tag: the item name and its price string, the
// horizontal center and top edge (in surface pixels) at which to draw it,
// whether it is the best pick, and optional inventory ownership info.
type Label struct {
	Name    string
	Price   string
	CenterX int
	Top     int
	Best    bool

	// OwnedKnown is true when inventory data is available; Owned is how many of
	// this part the player already has (0 => a part they don't own yet).
	OwnedKnown bool
	Owned      int
}

// markup builds the Pango-markup string for a label: a small dim item name
// above a large bold price (gold for the best pick) for fast scanning, with an
// optional ownership line (bright "NEW" when unowned, dim "owned ×N" otherwise).
func (l Label) markup() string {
	priceColor := "#ffffff"
	if l.Best {
		priceColor = "#ffd633"
	}
	m := fmt.Sprintf(
		`<span size="11500" foreground="#c8c8d0">%s</span>`+"\n"+
			`<span size="19000" weight="bold" foreground="%s">%s</span>`,
		escapeMarkup(l.Name), priceColor, escapeMarkup(l.Price))
	if l.OwnedKnown {
		if l.Owned == 0 {
			m += "\n" + `<span size="10500" weight="bold" foreground="#5fe38f">✦ NEW</span>`
		} else {
			m += "\n" + fmt.Sprintf(`<span size="10500" foreground="#9a9aa5">owned ×%d</span>`, l.Owned)
		}
	}
	return m
}

func escapeMarkup(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// draw renders labels into a premultiplied ARGB32 buffer of the given geometry.
func draw(buf []byte, width, height, stride int, labels []Label) {
	n := len(labels)
	if n == 0 {
		// Still clear the buffer to transparent.
		for i := range buf {
			buf[i] = 0
		}
		return
	}

	cx := make([]C.int, n)
	top := make([]C.int, n)
	best := make([]C.int, n)
	cmarkups := make([]*C.char, n)
	for i, l := range labels {
		cx[i] = C.int(l.CenterX)
		top[i] = C.int(l.Top)
		if l.Best {
			best[i] = 1
		}
		cmarkups[i] = C.CString(l.markup())
	}
	defer func() {
		for _, p := range cmarkups {
			C.free(unsafe.Pointer(p))
		}
	}()

	C.wfo_draw(
		(*C.uchar)(unsafe.Pointer(&buf[0])),
		C.int(width), C.int(height), C.int(stride),
		C.int(n),
		&cx[0], &top[0],
		(**C.char)(unsafe.Pointer(&cmarkups[0])),
		&best[0],
	)
}
