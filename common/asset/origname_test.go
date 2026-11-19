package asset

import (
	"context"
	"database/sql"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	_ "github.com/mattn/go-sqlite3"
)

func TestCleanOrigName(t *testing.T) {
	cases := map[string]string{
		"IMG_1234.HEIC":              "IMG_1234.HEIC",
		"  vacances été.jpg  ":       "vacances été.jpg",
		"DCIM/Camera/IMG_1.jpg":      "IMG_1.jpg",
		`C:\Users\me\Pictures\a.png`: "a.png",
		"../../etc/passwd":           "passwd",
		"..":                         "",
		"dir/":                       "",
		"bad\x00name\n.jpg":          "badname.jpg",
		"in\xffvalid.jpg":            "invalid.jpg",
		"":                           "",
	}
	for in, want := range cases {
		if got := CleanOrigName(in); got != want {
			t.Errorf("CleanOrigName(%q) = %q, want %q", in, got, want)
		}
	}

	long := strings.Repeat("é", 200) + ".jpeg"
	got := CleanOrigName(long)
	if len(got) > maxOrigNameBytes || !utf8.ValidString(got) || !strings.HasSuffix(got, ".jpeg") {
		t.Errorf("long name cleaned to %d bytes %q", len(got), got)
	}
}

func TestDownloadFilename(t *testing.T) {
	cases := []struct{ orig, served, want string }{
		{"IMG_1234.HEIC", "/m/2024/01/01/20240101000012.heic", "IMG_1234.heic"},
		{"IMG_1234.HEIC", "/p/2024/01/01/12_0.jpg", "IMG_1234.jpg"},
		{"IMG_1234.HEIC", "/m/2024/01/01/20240101000012.zip", "IMG_1234.zip"},
		{"noext", "/m/x.mp4", "noext.mp4"},
		{".hidden", "/m/x.jpg", ".hidden.jpg"},
		{"", "/m/x.jpg", ""},
	}
	for _, c := range cases {
		if got := DownloadFilename(c.orig, c.served); got != c.want {
			t.Errorf("DownloadFilename(%q, %q) = %q, want %q", c.orig, c.served, got, c.want)
		}
	}
}

func TestContentDisposition(t *testing.T) {
	got := ContentDisposition(`été "1"; (a).jpg`)
	want := `inline; filename="_t_ _1_; (a).jpg"; filename*=UTF-8''%C3%A9t%C3%A9%20%221%22%3B%20%28a%29.jpg`
	if got != want {
		t.Errorf("ContentDisposition = %s\nwant              %s", got, want)
	}
}

func TestSetOrigNames(t *testing.T) {
	f, err := ioutil.TempFile("", "origname-db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())
	db, err := sql.Open("sqlite3", f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`create table asset(id integer primary key, user_id integer, hash text,
		orig_name TEXT NOT NULL default '');
		insert into asset(user_id, hash) values (1, 'aaa'), (1, 'bbb'), (2, 'ccc');
		update asset set orig_name = 'first.jpg' where hash = 'bbb';`); err != nil {
		t.Fatal(err)
	}

	var stored map[string]string
	err = dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		stored, err = SetOrigNames(ctx, tx, 1, map[string]string{
			"aaa": "IMG_1.jpg", // recorded
			"bbb": "other.jpg", // keeps the name it already has
			"ccc": "x.jpg",     // another user's asset: untouched and not reported
			"ddd": "y.jpg",     // no such asset
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"aaa": "IMG_1.jpg", "bbb": "first.jpg"}
	if len(stored) != len(want) || stored["aaa"] != want["aaa"] || stored["bbb"] != want["bbb"] {
		t.Errorf("stored = %v, want %v", stored, want)
	}
	var ccc string
	if err := db.QueryRow("select orig_name from asset where hash = 'ccc'").Scan(&ccc); err != nil || ccc != "" {
		t.Errorf("other user's asset got name %q (%v)", ccc, err)
	}
}
