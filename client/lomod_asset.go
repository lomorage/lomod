package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/pkg/errors"
)

// IsAssetExist checks if asset is exist by its SHA
func (ld *Lomod) IsAssetExist(hash string) (bool, *PartialContent, error) {
	resp, err := ld.requestReply(http.MethodHead, "/asset/"+hash, nil)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil, nil
	} else if resp.StatusCode == http.StatusPartialContent {
		size, hash, err := common.GetHeaderIfMatch(resp.Header)
		return false, &PartialContent{Size: size, Hash: hash}, err
	} else if resp.StatusCode == http.StatusNotFound {
		return false, nil, nil
	} else if resp.StatusCode >= http.StatusInternalServerError {
		return false, nil, makeStatusError(resp.Body)
	} else {
		return false, nil, errors.Errorf("unsupported error code: %d", resp.StatusCode)
	}
}

// UploadAsset uploads asset
func (ld *Lomod) UploadAsset(f *os.File, pc *PartialContent, hash string, t time.Time, isCreateTime bool,
	metadatas []types.Metadata) (*types.Asset, error) {
	offset := 0
	method := http.MethodPost
	u := fmt.Sprintf("%s/asset/%s?%s=%s", ld.host, hash, common.QueryKeyExt,
		strings.TrimPrefix(filepath.Ext(f.Name()), "."))
	if !t.IsZero() {
		if isCreateTime {
			u += "&" + common.QueryKeyCreateTime + "=" + t.Format(common.TimeFormatLomod)
		} else {
			u += "&" + common.QueryKeyModifiedtime + "=" + t.Format(common.TimeFormatLomod)
		}
	}
	if len(metadatas) > 1 {
		metas := []string{}
		for _, meta := range metadatas {
			metas = append(metas, "meta="+meta.Name+","+url.QueryEscape(meta.Value))
		}
		u += "&" + strings.Join(metas, "&")
	}

	headers := getHeaders(ld.login.Token)
	if pc != nil {
		offset = pc.Size
		common.SetHeaderIfMatch(headers, int64(pc.Size), pc.Hash)
	}
	if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, err
	}
	resp, err := requestReply(method, u, headers, f)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusConflict:
		return nil, errors.Errorf("%s is exist", f.Name())
	default:
		return nil, makeStatusError(resp.Body)
	}

	a := &types.Asset{}
	return a, json.NewDecoder(resp.Body).Decode(a)
}

// DownloadAsset downloads asset
func (ld *Lomod) DownloadAsset(id string, isPreview bool, q map[string]string) (io.ReadCloser, error) {
	u := "/asset/"
	if isPreview {
		u += "preview/"
	}
	if len(id) == 40 {
		if q == nil {
			q = map[string]string{common.QueryKeyBahash: "1"}
		} else {
			q[common.QueryKeyBahash] = "1"
		}
	}
	resp, err := ld.get(u+id, q)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, common.ErrAssetNotExistForUser
	}
	return nil, makeStatusError(resp.Body)
}

// GetCategoryAll returns all categories
func (ld *Lomod) GetCategoryAll() (types.Years, error) {
	u := "/assets/merkletree"
	years := types.Years{}
	return years, ld.getReplyJSON(u, &years)
}

// GetCategoryYear returns categories in one year
func (ld *Lomod) GetCategoryYear(year int) (types.Year, error) {
	u := fmt.Sprintf("/assets/merkletree/%d", year)
	y := types.Year{}
	return y, ld.getReplyJSON(u, &y)
}

// GetCategoryMonth returns categories in one month
func (ld *Lomod) GetCategoryMonth(year, month int) (types.Month, error) {
	u := fmt.Sprintf("/assets/merkletree/%d/%d", year, month)
	m := types.Month{}
	return m, ld.getReplyJSON(u, &m)
}

// GetCategoryDay returns categories in one day
func (ld *Lomod) GetCategoryDay(year, month, day int) (types.Day, error) {
	u := fmt.Sprintf("/assets/merkletree/%d/%d/%d", year, month, day)
	d := types.Day{}
	return d, ld.getReplyJSON(u, &d)
}

// DeleteAsset deletes assets by type
func (ld *Lomod) DeleteAsset(id string) error {
	u := "/asset/" + id
	var q map[string]string
	if len(id) == 40 {
		q = map[string]string{common.QueryKeyBahash: "1"}
	}
	return ld.request(http.MethodDelete, u, q, nil)
}

// DeleteAssets deletes assets
func (ld *Lomod) DeleteAssets(ids []string) (*types.DeleteAssetItems, error) {
	u := "/asset"
	req := types.DeleteAssetItems{}
	req.List = make([]types.DeleteAssetItem, len(ids))
	for i, id := range ids {
		item := types.DeleteAssetItem{ID: id, Type: types.Index}
		if len(id) == 40 {
			item.Type = types.Hash
		}
		req.List[i] = item
	}
	resp, err := ld.requestReply(http.MethodDelete, u, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, makeStatusError(resp.Body)
	}
	defer resp.Body.Close()
	reply := &types.DeleteAssetItems{}
	return reply, json.NewDecoder(resp.Body).Decode(reply)
}

// ScanAndImportDir scan given directory and import into backend
func (ld *Lomod) ScanAndImportDir(dir string, move, scanVideo, useExifTime bool) error {
	u := "/assets/scan"
	q := map[string]string{
		common.QueryKeyScanAndImport: "1",
		common.QueryKeyPath:          url.QueryEscape(dir),
	}
	if move {
		q[common.QueryKeyMove] = "1"
	}
	if scanVideo {
		q[common.QueryKeyScanVideo] = "1"
	}
	if useExifTime {
		q[common.QueryKeyUseExifTime] = "1"
	}
	return ld.request(http.MethodPost, u, q, nil)
}

// GetAssetInfo returns asset overview by its hash
func (ld *Lomod) GetAssetInfo(hash string) (*types.Asset, error) {
	u := fmt.Sprintf("/asset/%s?%s=1", hash, common.QueryKeyInfo)
	a := &types.Asset{}
	return a, ld.getReplyJSON(u, &a)
}

// UpdateAssetCreateTime update asset create time
func (ld *Lomod) UpdateAssetCreateTime(hash string, y, m, d int) error {
	return ld.request(http.MethodPut, fmt.Sprintf("/asset/%s/%d/%d/%d", hash, y, m, d), nil, nil)
}
