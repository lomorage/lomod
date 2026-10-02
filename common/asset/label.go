package asset

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/sirupsen/logrus"
)

func ListLabels(ctx context.Context, tx *sql.Tx) ([]*types.AssetLabel, error) {
	rows, err := tx.QueryContext(ctx,
		"select id, label, label_cn, ifnull(a.c, '0')  from label as l left join (select label_id as id, count(asset_id) as c from asset_album_label group by label_id) as a using(id)")
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	labels := []*types.AssetLabel{}
	for rows.Next() {
		label := &types.AssetLabel{}
		err = rows.Scan(&label.ID, &label.Label, &label.LabelCN, &label.AssetsCount)
		if err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, nil
}

func CreateLabel(ctx context.Context, tx *sql.Tx, l *types.AssetLabel) (int64, error) {
	stmt, err := tx.Prepare("insert into label (label, label_cn, create_time) values(?, ?, ?)")
	if err != nil {
		return -1, err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, l.Label, l.LabelCN, time.Now().UTC())
	if err != nil {
		return -1, err
	}
	return result.LastInsertId()

}

func ListAssetsInLabel(ctx context.Context, tx *sql.Tx, userID, labelID int) ([]*types.AssetNameConfidence, error) {
	rows, err := tx.QueryContext(ctx,
		"select asset_id, ext_id, a.hash, aa.confidence from asset_album_label as aa inner join asset as a on aa.asset_id = a.id where user_id=? and label_id=? order by asset_id DESC",
		userID, labelID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	ids := []*types.AssetNameConfidence{}
	for rows.Next() {
		var aid, eid int
		id := &types.AssetNameConfidence{}
		err = rows.Scan(&aid, &eid, &id.Hash, &id.Confidence)
		if err != nil {
			return nil, err
		}

		id.Name, err = ext.MkAssetNameByID(aid, eid)
		if err != nil {
			logrus.Warnf("asset %d has invalid extension ID %d", aid, eid)
			continue
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

func AddAssetsInLabel(ctx context.Context, tx *sql.Tx, labelID int, assetIDs []int) error {
	statement := "insert into asset_album_label (asset_id, label_id, confidence) values "
	lid := strconv.Itoa(labelID)
	i := 0
	for _, id := range assetIDs {
		statement += "(" + strconv.Itoa(id) + "," + lid + ", 100.0)"
		if i < len(assetIDs)-1 {
			statement += ","
		}
		i++
	}
	_, err := tx.ExecContext(ctx, statement)
	return err
}

func RemoveLabelForAssets(ctx context.Context, tx *sql.Tx, labelID int, assetIDs []int) error {
	ids := ""
	for i, id := range assetIDs {
		ids += strconv.Itoa(id)
		if i != len(assetIDs)-1 {
			ids += ","
		}
	}
	statement := ""
	if labelID == -1 {
		statement = fmt.Sprintf("delete from asset_album_label where asset_id in (%s)", ids)
	} else {
		statement = fmt.Sprintf("delete from asset_album_label where label_id=%d and asset_id in (%s)",
			labelID, ids)
	}
	_, err := tx.ExecContext(ctx, statement)
	return err
}

func ListLabelsForAsset(ctx context.Context, tx *sql.Tx, assetID int) ([]*types.AssetLabelConfidence, error) {
	rows, err := tx.QueryContext(ctx,
		"select label_id, confidence from asset_album_label as a where asset_id=? order by label_id DESC",
		assetID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	labels := []*types.AssetLabelConfidence{}
	for rows.Next() {
		label := &types.AssetLabelConfidence{}
		err = rows.Scan(&label.ID, &label.Confidence)
		if err != nil {
			return nil, err
		}

		labels = append(labels, label)
	}

	return labels, rows.Err()
}