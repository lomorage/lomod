package asset

import (
	"crypto/sha1"
	"encoding"
	"encoding/json"
	"fmt"
	"hash"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// A partial upload's SHA-1 state is kept next to its temp file, so resuming it (PATCH) or
// reporting it (HEAD) doesn't have to re-read everything received so far. On a Raspberry Pi
// that re-read runs at ~30 MB/s: for a multi-GB video it outlasted the client's HEAD timeout
// and Cloudflare's 100 s origin timeout, so large uploads sent in pieces could never finish.
// The state is only a shortcut: when it is missing, or the temp file's size or modification
// time differs from when it was saved (something else touched the file), callers fall back to
// hashing the file. Trusting it is the same as trusting the bytes a single request wrote and
// fsynced, which lomod never re-reads either.

const resumeStateSuffix = ".sha1state"

type resumeState struct {
	// Size is how many bytes of the temp file the state covers.
	Size int64 `json:"size"`
	// ModTime is the temp file's modification time (UnixNano) when the state was saved.
	ModTime int64 `json:"mtime"`
	// SHA1 is the marshaled crypto/sha1 digest after those bytes.
	SHA1 []byte `json:"sha1"`
}

func resumeStateFilename(cachedFilename string) string {
	return cachedFilename + resumeStateSuffix
}

// saveResumeState records h as the SHA-1 of cachedFilename, which must be size bytes long.
func saveResumeState(cachedFilename string, size int64, h hash.Hash) error {
	fi, err := os.Stat(cachedFilename)
	if err != nil {
		return err
	}
	if fi.Size() != size {
		return errors.Errorf("%s is %d bytes, not %d", cachedFilename, fi.Size(), size)
	}
	m, ok := h.(encoding.BinaryMarshaler)
	if !ok {
		return errors.Errorf("%T can't be saved", h)
	}
	state, err := m.MarshalBinary()
	if err != nil {
		return err
	}
	data, err := json.Marshal(resumeState{Size: size, ModTime: fi.ModTime().UnixNano(), SHA1: state})
	if err != nil {
		return err
	}
	// write then rename, so a reader never sees a half-written state
	name := resumeStateFilename(cachedFilename)
	tmp := name + ".tmp"
	if err := ioutil.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}

// loadResumeState returns the SHA-1 of cachedFilename from its saved state, or nil if there is
// no state for the file as it is now: size bytes long and unmodified since the state was saved.
func loadResumeState(cachedFilename string, size int64) hash.Hash {
	fi, err := os.Stat(cachedFilename)
	if err != nil || fi.Size() != size {
		return nil
	}
	data, err := ioutil.ReadFile(resumeStateFilename(cachedFilename))
	if err != nil {
		if !os.IsNotExist(err) {
			logrus.Warnf("read resume state of %s: %v", cachedFilename, err)
		}
		return nil
	}
	var st resumeState
	if err := json.Unmarshal(data, &st); err != nil || st.Size != size || st.ModTime != fi.ModTime().UnixNano() {
		return nil
	}
	h := sha1.New()
	if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary(st.SHA1); err != nil {
		logrus.Warnf("bad resume state of %s: %v", cachedFilename, err)
		return nil
	}
	return h
}

func removeResumeState(cachedFilename string) {
	if err := os.Remove(resumeStateFilename(cachedFilename)); err != nil && !os.IsNotExist(err) {
		logrus.Warnf("remove resume state of %s: %v", cachedFilename, err)
	}
}

// restoreHash sets h to the state in src (both crypto/sha1 digests).
func restoreHash(h, src hash.Hash) error {
	state, err := src.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return err
	}
	u, ok := h.(encoding.BinaryUnmarshaler)
	if !ok {
		return fmt.Errorf("%T can't be restored", h)
	}
	return u.UnmarshalBinary(state)
}
