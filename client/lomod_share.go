package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/pkg/errors"
)

// Share shares one asset to user or group
func (ld *Lomod) Share(uid, aid int, isUser bool) (*types.Record, error) {
	b := "/send"
	if isUser {
		b += "/user"
	} else {
		b += "/group"
	}
	resp, err := ld.requestReply(http.MethodPost, fmt.Sprintf("%s/%d/%d", b, uid, aid), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Wrapf(makeStatusError(resp.Body), "status code: %d", resp.StatusCode)
	}
	rec := &types.Record{}
	return rec, json.NewDecoder(resp.Body).Decode(rec)
}

// ReceiveSharedAsset download received assets
func (ld *Lomod) ReceiveSharedAsset(sid uint64, isPreview bool, q map[string]string) (io.ReadCloser, error) {
	b := "/receive"
	if isPreview {
		b += "/preview/"
	} else {
		b += "/asset/"
	}
	resp, err := ld.get(b+strconv.FormatUint(sid, 10), q)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}
	defer resp.Body.Close()
	return nil, makeStatusError(resp.Body)
}
