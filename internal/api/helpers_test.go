package api_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"github.com/qwerin/zahrada/internal/api"
	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/storage"
)

func encodeImage(format string, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		img.Set(x, x*h/w, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&buf, img, nil)
	} else {
		err = png.Encode(&buf, img)
	}
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (c *client) upload(fields map[string]string, filename string, data []byte) (*http.Response, []byte) {
	c.ts.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if filename != "" {
		fw, _ := mw.CreateFormFile("file", filename)
		_, _ = fw.Write(data)
	}
	_ = mw.Close()
	req := httptest.NewRequest("POST", c.acct("/attachments"), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: c.session})
	rec := httptest.NewRecorder()
	c.ts.handler.ServeHTTP(rec, req)
	return rec.Result(), rec.Body.Bytes()
}

func uploadOK(c *client, ownerType string, ownerID uint, filename string, data []byte) api.Attachment {
	c.ts.t.Helper()
	res, body := c.upload(map[string]string{"owner_type": ownerType, "owner_id": strconv.Itoa(int(ownerID))}, filename, data)
	if res.StatusCode != http.StatusCreated {
		c.ts.t.Fatalf("upload %s: %d %s", filename, res.StatusCode, body)
	}
	return decodeJSON[api.Attachment](c.ts.t, body)
}

func newClient(c *client, name string) api.ClientDetail {
	c.ts.t.Helper()
	return doJSON[api.ClientDetail](c, http.StatusCreated, "POST", c.acct("/clients"), api.ClientCreate{Name: name, City: "Říčany"})
}

func newSite(c *client, clientID uint, name string) api.Site {
	c.ts.t.Helper()
	return doJSON[api.Site](c, http.StatusCreated, "POST", c.acct("/sites"),
		api.SiteCreate{ClientID: clientID, Name: name, Street: "Lipová 5", City: "Říčany", AccessNote: "klíč pod rohožkou"})
}

func newContractor(c *client, name string, rate int64) api.Contractor {
	c.ts.t.Helper()
	return doJSON[api.Contractor](c, http.StatusCreated, "POST", c.acct("/contractors"),
		api.ContractorCreate{Name: name, Email: "dodavatel@example.cz", HourlyRate: &rate})
}

func newJob(c *client, body api.JobCreate) api.Job {
	c.ts.t.Helper()
	return doJSON[api.Job](c, http.StatusCreated, "POST", c.acct("/jobs"), body)
}

func path(c *client, format string, args ...any) string { return c.acct(fmt.Sprintf(format, args...)) }

func ptr[T any](v T) *T { return &v }

func runJob(ts *testServer, name string) error {
	ts.t.Helper()
	deps := api.Deps{Now: func() time.Time { return ts.now }, Mailer: ts.mail, Storage: storage.NewLocal(ts.dataDir), Secrets: ts.secrets}
	for _, j := range api.Jobs(ts.db, ts.cfg, deps) {
		if j.Name == name {
			return j.Run(context.Background(), ts.now)
		}
	}
	ts.t.Fatalf("no job %q", name)
	return nil
}

func mustRunJob(ts *testServer, name string) {
	ts.t.Helper()
	if err := runJob(ts, name); err != nil {
		ts.t.Fatalf("job %s: %v", name, err)
	}
}
