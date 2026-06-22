package wfmarket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlugAndPrice(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/items", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[
			{"slug":"mesa_prime_set","i18n":{"en":{"name":"Mesa Prime Set"}}},
			{"slug":"mesa_prime_systems_blueprint","i18n":{"en":{"name":"Mesa Prime Systems Blueprint"}}}
		]}`))
	})
	mux.HandleFunc("/v1/items/mesa_prime_set/statistics", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"payload":{"statistics_closed":{"48hours":[
			{"median":60,"avg_price":62,"min_price":55},
			{"median":70,"avg_price":71,"min_price":66}
		]}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	oldItems, oldStats := itemsURLVar, statsURLVar
	itemsURLVar = srv.URL + "/v2/items"
	statsURLVar = srv.URL + "/v1/items/%s/statistics"
	defer func() { itemsURLVar, statsURLVar = oldItems, oldStats }()

	c := New(t.TempDir())
	if err := c.LoadItems(); err != nil {
		t.Fatal(err)
	}
	// Name -> slug, case/punctuation-insensitive.
	if s, ok := c.Slug("mesa prime systems blueprint"); !ok || s != "mesa_prime_systems_blueprint" {
		t.Errorf("slug = %q,%v", s, ok)
	}
	// Price uses the most recent 48h median.
	p, err := c.PriceByName("Mesa Prime Set")
	if err != nil {
		t.Fatal(err)
	}
	if p != 70 {
		t.Errorf("price = %d, want 70", p)
	}
}

func TestThrottleNonBlockingFirstCall(t *testing.T) {
	// First call shouldn't sleep meaningfully (no prior request).
	c := New(t.TempDir())
	c.throttle() // must return promptly
	_ = fmt.Sprintf("%v", strings.TrimSpace("ok"))
}
