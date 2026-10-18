package api_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qwerin/zahrada/internal/api"
	"github.com/qwerin/zahrada/internal/auth"
)

func TestPriceListAndQuotes(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"vat_payer": true, "quote_validity_days": 14, "quote_terms": "Platí 14 dní."})
	cl := newClient(a, "Klient")
	job := newJob(a, api.JobCreate{ClientID: cl.ID, Kind: "realization", Title: "Trávník"})

	// ceník: hromadný import (vložený text) i jednotlivě
	imp := doJSON[api.ListResponse[api.PriceItem]](a, http.StatusCreated, "POST", a.acct("/price-items/import"), api.PriceItemImport{Items: []api.PriceItemInput{
		{Kind: "work", Name: "Frézování půdy", Unit: "m²", UnitCost: 600, UnitPrice: ptr(int64(1500)), NormHours: "0,02"},
		{Kind: "material", Name: "Travní osivo", Unit: "kg", UnitCost: 18000, MarkupBps: 3000, WasteBps: 1000},
	}})
	if imp.Total != 2 || imp.Items[1].SalePrice != 23400 || imp.Items[0].NormHours != "0.02" || imp.Items[0].VatRateBps != 2100 {
		t.Fatalf("import: %+v", imp)
	}
	res, body := a.do("POST", a.acct("/price-items/import"), api.PriceItemImport{Items: []api.PriceItemInput{{Kind: "tree", Name: "x"}}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "items[0].kind")
	list := doJSON[api.ListResponse[api.PriceItem]](a, http.StatusOK, "GET", a.acct("/price-items?query=osivo"), nil)
	if list.Total != 1 {
		t.Fatalf("search: %+v", list)
	}

	// kalkulace v1 → úprava → odeslání (zakázka „nabídka odeslána“)
	q := doJSON[api.Quote](a, http.StatusCreated, "POST", path(a, "/jobs/%d/quotes", job.ID), api.QuoteCreate{})
	if q.Number != job.Number+"/v1" || q.Status != "draft" || q.Title != "Trávník" || q.Note != "Platí 14 dní." || q.ValidUntil != "2026-03-29" {
		t.Fatalf("new quote: %+v", q)
	}
	in := api.QuoteInput{Title: "Založení trávníku", DiscountBps: 300, Items: []api.QuoteLine{
		{CalcItem: api.CalcItem{Kind: "work", Section: "Příprava", Quantity: "250", UnitCost: 600, UnitPrice: ptr(int64(1500)), VatRateBps: 2100, NormHours: "0.02"}, Name: "Frézování půdy", Unit: "m²", PriceItemID: &imp.Items[0].ID},
		{CalcItem: api.CalcItem{Kind: "material", Section: "Trávník", Quantity: "7,5", WasteBps: 1000, UnitCost: 18000, MarkupBps: 3000, VatRateBps: 2100}, Name: "Travní osivo", Unit: "kg"},
	}}
	q = doJSON[api.Quote](a, http.StatusOK, "PUT", path(a, "/quotes/%d", q.ID), in)
	// 3750 + 1930,50 = 5680,50 − 3 % (170,415 → 170,42) = 5510,08
	if len(q.Items) != 2 || q.Net != 551008 || q.Result.Hours != "5" || q.Items[1].Quantity != "7.5" || q.Job.ClientName != "Klient" {
		t.Fatalf("saved: %+v", q)
	}
	res, body = a.do("PUT", path(a, "/quotes/%d", q.ID), api.QuoteInput{Title: "x", Items: []api.QuoteLine{{CalcItem: api.CalcItem{Kind: "work", Quantity: "1,2345", VatRateBps: 0}, Name: "x"}}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "items[0].quantity")

	doJSON[api.Quote](a, http.StatusOK, "POST", path(a, "/quotes/%d/status", q.ID), api.QuoteStatusChange{Status: "sent"})
	if j := doJSON[api.Job](a, http.StatusOK, "GET", path(a, "/jobs/%d", job.ID), nil); j.Status != "quoted" {
		t.Fatalf("job after sent: %s", j.Status)
	}
	res, body = a.do("PUT", path(a, "/quotes/%d", q.ID), in)
	assertCode(t, res, body, http.StatusConflict, "not_editable")

	// nová verze zkopíruje položky a starou nahradí; přijetí → zakázka objednána
	q2 := doJSON[api.Quote](a, http.StatusCreated, "POST", path(a, "/jobs/%d/quotes", job.ID), api.QuoteCreate{CopyFrom: &q.ID})
	if q2.Version != 2 || len(q2.Items) != 2 || q2.Net != q.Net || q2.Status != "draft" {
		t.Fatalf("v2: %+v", q2)
	}
	all := doJSON[[]api.QuoteSummary](a, http.StatusOK, "GET", path(a, "/jobs/%d/quotes", job.ID), nil)
	if len(all) != 2 || all[0].Version != 2 || all[1].Status != "superseded" {
		t.Fatalf("versions: %+v", all)
	}
	acc := doJSON[api.Quote](a, http.StatusOK, "POST", path(a, "/quotes/%d/status", q2.ID), api.QuoteStatusChange{Status: "accepted"})
	j := doJSON[api.Job](a, http.StatusOK, "GET", path(a, "/jobs/%d", job.ID), nil)
	if acc.Status != "accepted" || j.Status != "ordered" || j.AcceptedQuoteID == nil || *j.AcceptedQuoteID != q2.ID {
		t.Fatalf("accepted: %+v job %+v", acc.QuoteSummary, j)
	}
	res, body = a.do("DELETE", path(a, "/quotes/%d", q2.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "not_editable")

	// jiná firma nic nevidí
	b.mustDo(http.StatusNotFound, "GET", path(b, "/quotes/%d", q2.ID), nil)
	b.mustDo(http.StatusNotFound, "GET", path(b, "/jobs/%d/quotes", job.ID), nil)
	b.mustDo(http.StatusNotFound, "PUT", path(b, "/price-items/%d", imp.Items[0].ID), api.PriceItemInput{Kind: "work", Name: "x"})
}

func (c *client) uploadTo(url, filename string, data []byte) (*http.Response, []byte) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: c.session})
	rec := httptest.NewRecorder()
	c.ts.handler.ServeHTTP(rec, req)
	return rec.Result(), rec.Body.Bytes()
}

func TestJobPhotosFromContractor(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	cl := newClient(a, "Klient")
	petr := newContractor(a, "Petr", 30000)
	mine := newJob(a, api.JobCreate{ClientID: cl.ID, Kind: "realization", Title: "Trávník", Status: "ordered", ContractorIDs: []uint{petr.ID}})
	other := newJob(a, api.JobCreate{ClientID: cl.ID, Kind: "one_off", Title: "Cizí", Status: "ordered"})
	d := contractorLogin(t, ts, a, petr.ID, "petr@example.cz")

	res, body := d.uploadTo(path(d, "/me/jobs/%d/photos", mine.ID), "pred.jpg", encodeImage("jpeg", 20, 10))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %s", res.StatusCode, body)
	}
	photo := decodeJSON[api.Attachment](t, body)
	res, body = d.uploadTo(path(d, "/me/jobs/%d/photos", mine.ID), "smlouva.pdf", []byte("%PDF-1.4\n"))
	assertError(t, res, body, http.StatusUnsupportedMediaType, "only photos")
	res, body = d.uploadTo(path(d, "/me/jobs/%d/photos", other.ID), "x.jpg", encodeImage("jpeg", 2, 2))
	assertError(t, res, body, http.StatusNotFound, "")

	mineList := doJSON[[]api.Attachment](d, http.StatusOK, "GET", path(d, "/me/jobs/%d/photos", mine.ID), nil)
	staff := doJSON[[]api.Attachment](a, http.StatusOK, "GET", path(a, "/jobs/%d/photos", mine.ID), nil)
	if len(mineList) != 1 || len(staff) != 1 || staff[0].UploadedBy != "Vlastní petr@example.cz" && staff[0].UploadedBy == "" {
		t.Fatalf("photos: %+v / %+v", mineList, staff)
	}
	res, _ = d.do("GET", path(d, "/me/photos/%d", photo.ID), nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("view photo: %d %v", res.StatusCode, res.Header)
	}
	// staff photo of another job is not reachable through /me
	staffPhoto := uploadOK(a, "job", other.ID, "jine.png", encodeImage("png", 3, 3))
	d.mustDo(http.StatusNotFound, "GET", path(d, "/me/photos/%d", staffPhoto.ID), nil)
	d.mustDo(http.StatusForbidden, "GET", path(d, "/attachments/%d/download", photo.ID), nil)
}
