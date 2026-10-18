package api_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"testing"
)

// hugePNG is a PNG header declaring w×h pixels (IHDR only): tiny on disk.
func hugePNG(w, h uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	_ = binary.Write(&b, binary.BigEndian, uint32(len(ihdr)))
	crc := crc32.NewIEEE()
	crc.Write([]byte("IHDR"))
	crc.Write(ihdr)
	b.WriteString("IHDR")
	b.Write(ihdr)
	_ = binary.Write(&b, binary.BigEndian, crc.Sum32())
	b.Write(bytes.Repeat([]byte{0}, 64))
	return b.Bytes()
}

// The logo is limited in size and dimensions (decompression bombs).
func TestAccountImageLimits(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")

	bomb := uploadOK(a, "account", 0, "bomb.png", hugePNG(8000, 8000))
	res, body := a.do("PATCH", a.acct(""), map[string]any{"logo_attachment_id": bomb.ID})
	assertError(t, res, body, http.StatusUnprocessableEntity, "4000×4000 px")

	big := uploadOK(a, "account", 0, "big.png", append(encodeImage("png", 4, 4), make([]byte, 3<<20)...))
	res, body = a.do("PATCH", a.acct(""), map[string]any{"logo_attachment_id": big.ID})
	assertError(t, res, body, http.StatusUnprocessableEntity, "at most 2 MB")

	ok := uploadOK(a, "account", 0, "logo.jpg", encodeImage("jpeg", 40, 20))
	res, body = a.do("PATCH", a.acct(""), map[string]any{"logo_attachment_id": ok.ID})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("logo: %d %s", res.StatusCode, body)
	}
}
