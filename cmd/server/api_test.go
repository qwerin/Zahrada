package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qwerin/zahrada/internal/config"
)

func do(t *testing.T, h http.Handler, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result(), rec.Body.Bytes()
}

func TestHealth(t *testing.T) {
	h := newHandler(config.Config{})
	res, body := do(t, h, "GET", "/api/health", "")
	if res.StatusCode != 200 || !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("%d %s", res.StatusCode, body)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing security headers")
	}
	res, _ = do(t, h, "GET", "/api/nope", "")
	if res.StatusCode != 404 {
		t.Errorf("unknown api route: %d", res.StatusCode)
	}
}

func TestCalcPreview(t *testing.T) {
	h := newHandler(config.Config{})
	in := `{"vat_payer":true,"discount_bps":300,"items":[
	  {"kind":"work","section":"Příprava terénu","quantity":"250","unit_cost":600,"unit_price":1500,"vat_rate_bps":2100,"norm_hours":"0.02"},
	  {"kind":"material","section":"Trávník","quantity":"7,5","waste_bps":1000,"unit_cost":18000,"markup_bps":3000,"vat_rate_bps":2100},
	  {"kind":"material","section":"Trávník","quantity":"12.5","unit_cost":95000,"unit_price":129000,"discount_bps":500,"vat_rate_bps":1200},
	  {"kind":"transport","quantity":"2","unit_cost":120000,"vat_rate_bps":2100}]}`
	res, body := do(t, h, "POST", "/api/calc/preview", in)
	if res.StatusCode != 200 {
		t.Fatalf("%d %s", res.StatusCode, body)
	}
	var out previewOut
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Gross != 2_612_637 || out.Hours != "5" || out.Items[1].EffectiveQuantity != "8.25" || len(out.VAT) != 2 {
		t.Errorf("unexpected: %+v", out)
	}

	res, body = do(t, h, "POST", "/api/calc/preview", `{"items":[{"kind":"material","quantity":"1","vat_rate_bps":2100},{"kind":"material","quantity":"1,2345","vat_rate_bps":2100}]}`)
	if res.StatusCode != 422 || !strings.Contains(string(body), `"field":"items[1].quantity"`) {
		t.Errorf("validation: %d %s", res.StatusCode, body)
	}
	res, body = do(t, h, "POST", "/api/calc/preview", `{"items":[{"kind":"tree","quantity":"1","vat_rate_bps":2100}]}`)
	if res.StatusCode != 422 || !strings.Contains(string(body), `"field":"items[0].kind"`) {
		t.Errorf("kind: %d %s", res.StatusCode, body)
	}
	res, _ = do(t, h, "POST", "/api/calc/preview", `{"items":[], "hack":1}`)
	if res.StatusCode != 400 {
		t.Errorf("unknown field: %d", res.StatusCode)
	}
}

func TestSPAFallback(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>app"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "assets", "app-abc.js"), []byte("js"), 0o644)
	h := newHandler(config.Config{StaticDir: dir})

	res, body := do(t, h, "GET", "/zakazky/12", "")
	if res.StatusCode != 200 || !strings.Contains(string(body), "app") {
		t.Errorf("fallback: %d %s", res.StatusCode, body)
	}
	res, _ = do(t, h, "GET", "/assets/app-abc.js", "")
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %d %v", res.StatusCode, res.Header)
	}
	res, body = do(t, h, "GET", "/../../etc/passwd", "")
	if strings.Contains(string(body), "root:") {
		t.Error("path traversal")
	}
}
