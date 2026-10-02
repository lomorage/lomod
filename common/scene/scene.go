package scene

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/scene/labels"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Summary is overall summary for one scene
type Summary struct {
	ID         int
	Label      string
	AssetCount int
}

// AssociateAssetWithSceneLabel add given asset into list of scene labels
func AssociateAssetWithSceneLabel(ctx context.Context, tx *sql.Tx, assetID int, label string, prob float32) error {
	rule, ok := labels.Find(label)
	if !ok {
		logrus.Debugf("unable to find label for asset %d, %s", assetID, label)
		return nil
	}
	if rule.Label == "" {
		return nil
	}
	if rule.Threshold > prob {
		logrus.Debugf("asset %d %s's probability %f is lower than %v", assetID, label, prob, rule)
		return nil
	}
	id, ok := labels.LabelMaps[rule.Label]
	if !ok {
		logrus.Debugf("unable to find label map for asset %d, %s, %v", assetID, label, rule)
		return nil
	}

	_, err := tx.ExecContext(ctx,
		"insert OR IGNORE into asset_album_label (asset_id, label_id, confidence, create_time) values (?,?,?,?)",
		assetID, id, prob, time.Now().UTC())
	return err
}

// DeAssociateAsset remove given asset from list of scene labels
func DeAssociateAsset(ctx context.Context, tx *sql.Tx, assetID int) error {
	_, err := tx.ExecContext(ctx, "delete from asset_album_label where asset_id=?", assetID)
	return err
}

// ListAssetNames returns the list of asset names in given scene
func ListAssetNames(ctx context.Context, tx *sql.Tx, userID, labelID, page, limit int) ([]string, error) {
	stmt, err := tx.Prepare(`
select a.id, a.ext_id from asset_album_label as aasl
  inner join label as sl on sl.id=aasl.label_id
  inner join asset as a on a.id=aasl.asset_id
  inner join user as u on u.id=a.user_id
  where u.id=? and sl.id=? order by asset_id DESC limit ? offset ?
`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, userID, labelID, limit, page*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		id := 0
		eid := 0
		err = rows.Scan(&id, &eid)
		if err != nil {
			return nil, err
		}

		e, err := ext.GetExtString(eid)
		if err != nil {
			logrus.Warnf("asst %d has invalid extension: %d", id, eid)
			continue
		}
		names = append(names, strconv.Itoa(id)+"."+e)
	}

	return names, rows.Err()
}

// GetScene returns thing detail
func GetScene(ctx context.Context, tx *sql.Tx, labelID int) (label string, err error) {
	// read from labels
	//err = tx.QueryRowContext(ctx, "select label from label where id=?", labelID).Scan(&label)
	label, ok := labels.LabelIDMaps[labelID]
	if !ok {
		err = errors.Wrapf(common.ErrNotFound, "label ID: %d", labelID)
	}
	return
}

// GetScenesByUser returns things which the photos are classified
func GetScenesByUser(ctx context.Context, tx *sql.Tx, userID int) ([]Summary, error) {
	statement := `
select sl.id, sl.label, count(aasl.asset_id) from asset_album_label as aasl
  inner join label as sl on sl.id=aasl.label_id
  inner join asset as a on a.id=aasl.asset_id
  inner join user as u on u.id=a.user_id
  where u.id=? group by sl.label order by sl.id
`
	rows, err := tx.QueryContext(ctx, statement, userID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	things := []Summary{}
	for rows.Next() {
		thing := Summary{}
		err = rows.Scan(&thing.ID, &thing.Label, &thing.AssetCount)
		if err != nil {
			return nil, err
		}
		things = append(things, thing)
	}
	return things, rows.Err()
}

// GetAssetsInScene returns assets in one scene
func GetAssetsInScene(ctx context.Context, tx *sql.Tx, userID, labelID, count int) ([]string, error) {
	statement := `
select aasl.asset_id, a.ext_id from asset_album_label as aasl
  inner join asset as a on a.id=aasl.asset_id
  inner join user as u on u.id=a.user_id
  where u.id=? and aasl.label_id=? and a.ext_id in (?,?,?)`
	if count != 0 {
		statement = statement + " order by aasl.asset_id DESC limit " + strconv.Itoa(count)
	}
	rows, err := tx.QueryContext(ctx, statement, userID, labelID, ext.JPG, ext.JPEG, ext.WebP)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var id, eid int
		err = rows.Scan(&id, &eid)
		if err != nil {
			return nil, err
		}
		name, err := ext.MkAssetNameByID(id, eid)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
