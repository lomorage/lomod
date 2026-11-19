package asset

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxOrigNameBytes caps a stored original name; common file systems allow 255 bytes.
const maxOrigNameBytes = 255

// CleanOrigName turns a client-supplied file name into one safe to store and to put in a
// download's file name: no directory part, no control characters, valid UTF-8, at most
// maxOrigNameBytes. It returns "" when nothing usable is left.
func CleanOrigName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToValidUTF8(name, "")
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if len(name) > maxOrigNameBytes {
		ext := filepath.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		stem := name[:maxOrigNameBytes-len(ext)]
		for !utf8.ValidString(stem) {
			stem = stem[:len(stem)-1]
		}
		name = stem + ext
	}
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// DownloadFilename is what a download of servedFile is called when the asset's original name
// is origName: the original stem with the served file's extension, since a download can be a
// transcoded preview (e.g. IMG_1234.HEIC served as IMG_1234.jpg). "" when origName is unknown.
func DownloadFilename(origName, servedFile string) string {
	if origName == "" {
		return ""
	}
	stem := strings.TrimSuffix(origName, filepath.Ext(origName))
	if stem == "" {
		stem = origName
	}
	return stem + strings.ToLower(filepath.Ext(servedFile))
}

// ContentDisposition is an inline Content-Disposition header value naming the file, with an
// ASCII fallback for old clients and the exact UTF-8 name in filename* (RFC 6266/5987).
func ContentDisposition(filename string) string {
	fallback := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' || r == '%' {
			return '_'
		}
		return r
	}, filename)
	return `inline; filename="` + fallback + `"; filename*=UTF-8''` + rfc5987Escape(filename)
}

// rfc5987Escape percent-encodes every byte that is not an RFC 5987 attr-char.
func rfc5987Escape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0xf])
	}
	return b.String()
}

// GetOrigName returns an asset's original name, "" if unknown.
func GetOrigName(ctx context.Context, tx *sql.Tx, assetID int) (string, error) {
	var name string
	err := tx.QueryRowContext(ctx, "select orig_name from asset where id = ?", assetID).Scan(&name)
	return name, err
}

// SetOrigNames records original names (hash -> already cleaned name) for a user's assets that
// have none yet; a name already stored is kept, so the first device to report one wins. It
// returns the name each existing asset ends up with, keyed by hash; hashes with no asset are
// left out.
func SetOrigNames(ctx context.Context, tx *sql.Tx, userID int, names map[string]string) (map[string]string, error) {
	update, err := tx.PrepareContext(ctx,
		"update asset set orig_name = ? where user_id = ? and hash = ? and orig_name = ''")
	if err != nil {
		return nil, err
	}
	defer update.Close()
	query, err := tx.PrepareContext(ctx, "select orig_name from asset where user_id = ? and hash = ?")
	if err != nil {
		return nil, err
	}
	defer query.Close()

	stored := make(map[string]string, len(names))
	for hash, name := range names {
		if name != "" {
			if _, err := update.ExecContext(ctx, name, userID, hash); err != nil {
				return nil, err
			}
		}
		var got string
		switch err := query.QueryRowContext(ctx, userID, hash).Scan(&got); {
		case err == sql.ErrNoRows:
			continue
		case err != nil:
			return nil, err
		}
		stored[hash] = got
	}
	return stored, nil
}
