package db

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/model"
)

func TestOpenMigrate(t *testing.T) {
	d, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}

	var fk int
	d.Raw("PRAGMA foreign_keys").Scan(&fk)
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}

	u := model.User{Email: "a@b.cz", Name: "A", PasswordHash: "x"}
	if err := d.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	dup := model.User{Email: "a@b.cz", Name: "B", PasswordHash: "x"}
	if err := d.Create(&dup).Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate email: got %v, want ErrDuplicatedKey", err)
	}

	job := model.Job{AccountID: 1, Number: "Z2026-001", ClientID: 1, Kind: model.JobRealization, Title: "Trávník", Status: model.JobInquiry}
	if err := d.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	same := model.Job{AccountID: 1, Number: "Z2026-001", ClientID: 1, Kind: model.JobRealization, Title: "x", Status: model.JobInquiry}
	if err := d.Create(&same).Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate job number: got %v, want ErrDuplicatedKey", err)
	}
	var got model.Job
	if err := d.First(&got, job.ID).Error; err != nil || got.SearchText == "" {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
}

func TestOpenUnknownDriver(t *testing.T) {
	if _, err := Open("mysql", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoggerQuietAndWithoutValues(t *testing.T) {
	var buf bytes.Buffer
	lg, err := newLogger(options{logLevel: "info", slow: time.Second, out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	d, err := gorm.Open(sqlite.Open(sqliteDSN(":memory:")), &gorm.Config{Logger: lg})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	var u model.User
	err = d.Where("email = ?", "tajny@example.cz").First(&u).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "tajny@example.cz") {
		t.Errorf("query values must not be logged:\n%s", out)
	}
	if strings.Contains(out, "record not found") {
		t.Errorf("record not found must not be logged:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("log must be without colors")
	}

	buf.Reset()
	quiet, _ := newLogger(options{logLevel: "error", slow: time.Second, out: &buf})
	d = d.Session(&gorm.Session{Logger: quiet})
	d.Where("email = ?", "x").First(&u)
	if buf.Len() != 0 {
		t.Errorf("default level error must stay quiet for normal queries: %s", buf.String())
	}
	if _, err := newLogger(options{logLevel: "loud"}); err == nil {
		t.Error("unknown level must fail")
	}
}
