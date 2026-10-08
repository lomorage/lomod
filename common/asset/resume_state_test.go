package asset

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/types"
)

func randomBytes(t *testing.T, n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func sha1Hex(b []byte) string {
	return fmt.Sprintf("%x", sha1.Sum(b))
}

// uploadPiece sends one request's worth of data the way the handler does (POST at offset 0,
// PATCH with If-Match size/sha1 after that).
func uploadPiece(t *testing.T, home, finalSHA string, piece []byte, currSize int64, currSHA string) (string, error) {
	t.Helper()
	lsa := &types.LastSavedAsset{FinalSHA: finalSHA, CurrSize: currSize, CurrSHA: currSHA}
	return SaveAsset(home, lsa, ioutil.NopCloser(bytes.NewReader(piece)), sha1.New(), 0755, 0644)
}

// head reports the partial upload the way HEAD does.
func head(t *testing.T, home, finalSHA string) *types.LastSavedAsset {
	t.Helper()
	lsa, err := GetPartialUploadContent(home, finalSHA)
	if err != nil {
		t.Fatalf("partial content: %v", err)
	}
	return lsa
}

func stateExists(home, finalSHA string) bool {
	_, err := os.Stat(resumeStateFilename(getCachedFilename(home, finalSHA)))
	return err == nil
}

func TestUploadInPiecesUsesSavedState(t *testing.T) {
	home := t.TempDir()
	data := randomBytes(t, 3*1024*1024+123)
	final := sha1Hex(data)
	pieces := [][]byte{data[:1<<20], data[1<<20 : 2<<20], data[2<<20:]}

	_, err := uploadPiece(t, home, final, pieces[0], 0, "")
	if err != common.ErrAssetDiffHash {
		t.Fatalf("first piece: got %v, want ErrAssetDiffHash (kept as partial)", err)
	}
	if !stateExists(home, final) {
		t.Fatal("no resume state after the first piece")
	}

	var offset int64
	for i, piece := range pieces[1:] {
		st := head(t, home, final)
		offset += int64(len(pieces[i]))
		if st.CurrSize != offset || st.CurrSHA != sha1Hex(data[:offset]) {
			t.Fatalf("HEAD after piece %d: size %d sha %s, want %d %s", i, st.CurrSize, st.CurrSHA, offset, sha1Hex(data[:offset]))
		}
		name, err := uploadPiece(t, home, final, piece, st.CurrSize, st.CurrSHA)
		last := i == len(pieces)-2
		if last {
			if err != nil {
				t.Fatalf("last piece: %v", err)
			}
			got, _ := ioutil.ReadFile(name)
			if !bytes.Equal(got, data) {
				t.Fatal("assembled file differs from the original")
			}
		} else if err != common.ErrAssetDiffHash {
			t.Fatalf("piece %d: got %v, want ErrAssetDiffHash", i+1, err)
		}
	}
	if stateExists(home, final) {
		t.Fatal("resume state left behind after the upload completed")
	}
}

func corruptByte(t *testing.T, name string, at int64, keepModTime bool) {
	t.Helper()
	fi, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 1)
	if _, err := f.ReadAt(b, at); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte{b[0] ^ 0xff}, at); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if keepModTime {
		if err := os.Chtimes(name, fi.ModTime(), fi.ModTime()); err != nil {
			t.Fatal(err)
		}
	}
}

// With a valid state, HEAD must not re-read what was received. A byte changed behind lomod's
// back with the modification time put back shows it: HEAD still reports the state's SHA-1.
func TestHeadUsesStateWithoutRereading(t *testing.T) {
	home := t.TempDir()
	data := randomBytes(t, 2*1024*1024)
	final := sha1Hex(data)
	if _, err := uploadPiece(t, home, final, data[:1<<20], 0, ""); err != common.ErrAssetDiffHash {
		t.Fatal(err)
	}
	corruptByte(t, getCachedFilename(home, final), 10, true)

	if st := head(t, home, final); st.CurrSHA != sha1Hex(data[:1<<20]) {
		t.Fatalf("HEAD re-read the file (sha %s) instead of using the saved state", st.CurrSHA)
	}
}

// A temp file modified after the state was saved (new modification time) is hashed again, so
// the change is reported and the upload can't complete with the wrong content.
func TestModifiedPartialFileIsRehashed(t *testing.T) {
	home := t.TempDir()
	data := randomBytes(t, 2*1024*1024)
	final := sha1Hex(data)
	if _, err := uploadPiece(t, home, final, data[:1<<20], 0, ""); err != common.ErrAssetDiffHash {
		t.Fatal(err)
	}
	cached := getCachedFilename(home, final)
	fi, _ := os.Stat(cached)
	// make sure the clock visibly moves on file systems with coarse timestamps
	later := fi.ModTime().Add(3 * time.Second)
	corruptByte(t, cached, 10, false)
	if err := os.Chtimes(cached, later, later); err != nil {
		t.Fatal(err)
	}

	st := head(t, home, final)
	if st.CurrSHA == sha1Hex(data[:1<<20]) {
		t.Fatal("HEAD trusted a stale state for a modified file")
	}
	if _, err := uploadPiece(t, home, final, data[1<<20:], st.CurrSize, st.CurrSHA); err != common.ErrAssetDiffHash {
		t.Fatalf("completing a corrupted file: got %v, want ErrAssetDiffHash", err)
	}
}

// PATCH, too, resumes from the state without re-reading.
func TestPatchUsesStateWithoutRereading(t *testing.T) {
	home := t.TempDir()
	data := randomBytes(t, 2*1024*1024)
	final := sha1Hex(data)
	if _, err := uploadPiece(t, home, final, data[:1<<20], 0, ""); err != common.ErrAssetDiffHash {
		t.Fatal(err)
	}
	st := head(t, home, final)
	corruptByte(t, getCachedFilename(home, final), 10, true)

	// a re-read would see the changed byte and reject If-Match; the state accepts it, and the
	// assembled hash comes from the state -- the trust described in resume_state.go
	if _, err := uploadPiece(t, home, final, data[1<<20:], st.CurrSize, st.CurrSHA); err != nil {
		t.Fatalf("PATCH re-read the partial file: %v", err)
	}
}

type failingReader struct {
	r   io.Reader
	n   int
	err error
}

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, f.err
	}
	if len(p) > f.n {
		p = p[:f.n]
	}
	n, err := f.r.Read(p)
	f.n -= n
	return n, err
}

func TestInterruptedUploadKeepsState(t *testing.T) {
	home := t.TempDir()
	data := randomBytes(t, 1500*1024)
	final := sha1Hex(data)
	cut := 700*1024 + 17

	lsa := &types.LastSavedAsset{FinalSHA: final}
	body := ioutil.NopCloser(&failingReader{r: bytes.NewReader(data), n: cut, err: errors.New("connection reset")})
	if _, err := SaveAsset(home, lsa, body, sha1.New(), 0755, 0644); err == nil {
		t.Fatal("interrupted upload reported success")
	}
	if !stateExists(home, final) {
		t.Fatal("no resume state after an interrupted upload")
	}
	st := head(t, home, final)
	if st.CurrSize != int64(cut) || st.CurrSHA != sha1Hex(data[:cut]) {
		t.Fatalf("HEAD: %d %s, want %d %s", st.CurrSize, st.CurrSHA, cut, sha1Hex(data[:cut]))
	}
	if _, err := uploadPiece(t, home, final, data[cut:], st.CurrSize, st.CurrSHA); err != nil {
		t.Fatalf("resume: %v", err)
	}
}

func TestUnusableStateFallsBackToHashingTheFile(t *testing.T) {
	for name, spoil := range map[string]func(t *testing.T, stateFile string){
		"missing": func(t *testing.T, stateFile string) {
			if err := os.Remove(stateFile); err != nil {
				t.Fatal(err)
			}
		},
		"corrupt": func(t *testing.T, stateFile string) {
			if err := ioutil.WriteFile(stateFile, []byte("{not json"), 0600); err != nil {
				t.Fatal(err)
			}
		},
		"other size": func(t *testing.T, stateFile string) {
			if err := ioutil.WriteFile(stateFile, []byte(`{"size":5,"sha1":""}`), 0600); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			data := randomBytes(t, 2*1024*1024)
			final := sha1Hex(data)
			if _, err := uploadPiece(t, home, final, data[:1<<20], 0, ""); err != common.ErrAssetDiffHash {
				t.Fatal(err)
			}
			spoil(t, resumeStateFilename(getCachedFilename(home, final)))

			st := head(t, home, final)
			if st.CurrSize != 1<<20 || st.CurrSHA != sha1Hex(data[:1<<20]) {
				t.Fatalf("HEAD: %d %s", st.CurrSize, st.CurrSHA)
			}
			if _, err := uploadPiece(t, home, final, data[1<<20:], st.CurrSize, st.CurrSHA); err != nil {
				t.Fatalf("resume: %v", err)
			}
		})
	}
}

func TestPatchWithWrongSHAIsStillRejected(t *testing.T) {
	home := t.TempDir()
	data := randomBytes(t, 2*1024*1024)
	final := sha1Hex(data)
	if _, err := uploadPiece(t, home, final, data[:1<<20], 0, ""); err != common.ErrAssetDiffHash {
		t.Fatal(err)
	}
	_, err := uploadPiece(t, home, final, data[1<<20:], 1<<20, sha1Hex([]byte("something else")))
	if err == nil || err == common.ErrAssetDiffHash {
		t.Fatalf("PATCH with a wrong If-Match sha1: got %v, want a mismatch error", err)
	}
}

func TestNewUploadDiscardsOldState(t *testing.T) {
	home := t.TempDir()
	data := randomBytes(t, 2*1024*1024)
	final := sha1Hex(data)
	if _, err := uploadPiece(t, home, final, data[:1<<20], 0, ""); err != common.ErrAssetDiffHash {
		t.Fatal(err)
	}
	// the client starts over with a full POST
	if _, err := uploadPiece(t, home, final, data, 0, ""); err != nil {
		t.Fatalf("full upload: %v", err)
	}
	if stateExists(home, final) {
		t.Fatal("stale resume state left after a full upload")
	}
	entries, _ := ioutil.ReadDir(filepath.Dir(getCachedFilename(home, final)))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temp state file left: %s", e.Name())
		}
	}
}
