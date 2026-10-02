package asset

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// QueryAssetsNoAlbum is query condition for assets without associating with any albums
	QueryAssetsNoAlbum = "miss-album"
	// QueryMetadataName is query condition for metadata name
	QueryMetadataName = "name"
	// QueryMetadataNameMiss is query condition for metadata name
	QueryMetadataNameMiss = "miss-name"
	// QueryMetadataVersionLess is query condition for metadata version
	QueryMetadataVersionLess = "ver-less"
	// QueryMetadataSourceDevice is query condition for source device
	QueryMetadataSourceDevice = "source-device"
	// QueryMetadataCategory is query condition for category
	QueryMetadataCategory = "category"
	// QueryMetadataCategoryMiss is query condition for category
	QueryMetadataCategoryMiss = "miss-category"
	// QueryMetadataNV is to query assets by combining all condition with OR
	QueryMetadataNV = "meta-nv"
)

func mkPageStatement(offset, limit int) string {
	return " order by id DESC limit " + strconv.Itoa(limit) + " offset " + strconv.Itoa(offset)
}

// getMetadataVersion returns the version of asset's given metadata.
func getMetadataVersion(ctx context.Context, tx *sql.Tx, category, name string, sourceDeviceID, assetID int) (int, error) {
	version := 0
	err := tx.QueryRowContext(ctx, "select version from metadata_"+category+" where source_device = ? and asset_id = ? and name = ?",
		sourceDeviceID, assetID, name).Scan(&version)
	return version, err
}

// GetMetadatas return asset's metadata
func GetMetadatas(ctx context.Context, tx *sql.Tx, assetID int) ([]types.Metadata, error) {
	statements := make([]string, len(types.AllMetadataCategories))
	for category, i := range types.AllMetadataCategories {
		statements[i] = fmt.Sprintf(`
select '%s' as category, source_device, name, value, model, version, create_time, last_modified_time from metadata_%s
where asset_id = %d
`,
			category, category, assetID)
	}
	metadata := []types.Metadata{}
	rows, err := tx.QueryContext(ctx, strings.Join(statements, " union ")+" order by category, name")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		m := types.Metadata{AssetID: assetID}
		var (
			dID                int
			category, dt1, dt2 string
		)
		if err := rows.Scan(&category, &dID, &m.Name, &m.Value, &m.Model, &m.Version, &dt1, &dt2); err != nil {
			return nil, err
		}
		m.CreateTime, err = types.ParseDBTime(dt1)
		if err != nil {
			return nil, err
		}
		m.LastModifiedTime, err = types.ParseDBTime(dt2)
		if err != nil {
			return nil, err
		}
		cat, err := types.NewMetadataCategory(category)
		if err != nil {
			return nil, err
		}
		m.Category = cat
		device, err := types.NewSourceDeviceByID(dID)
		if err != nil {
			return nil, err
		}
		m.SourceDevice = device

		metadata = append(metadata, m)
	}
	return metadata, nil
}

// GetMetadatasByIDs return specified assets' metadata
func GetMetadatasByIDs(ctx context.Context, tx *sql.Tx, assetIDs []int) (map[int][]types.Metadata, error) {
	ids := []string{}
	for _, id := range assetIDs {
		ids = append(ids, strconv.Itoa(id))
	}

	statements := make([]string, len(types.AllMetadataCategories))
	for category, i := range types.AllMetadataCategories {
		statements[i] = fmt.Sprintf(`
select '%s' as category, asset_id, source_device, name, value, model, version, create_time, last_modified_time from metadata_%s
where asset_id in (%s)
`,
			category, category, strings.Join(ids, ","))
	}
	ret := map[int][]types.Metadata{}
	rows, err := tx.QueryContext(ctx, strings.Join(statements, " union ")+" order by category, name")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		m := types.Metadata{}
		var (
			aid                int
			dID                int
			category, dt1, dt2 string
		)
		if err := rows.Scan(&category, &aid, &dID, &m.Name, &m.Value, &m.Model, &m.Version, &dt1, &dt2); err != nil {
			return nil, err
		}

		m.AssetID = aid
		m.CreateTime, err = types.ParseDBTime(dt1)
		if err != nil {
			return nil, err
		}
		m.LastModifiedTime, err = types.ParseDBTime(dt2)
		if err != nil {
			return nil, err
		}
		cat, err := types.NewMetadataCategory(category)
		if err != nil {
			return nil, err
		}
		m.Category = cat
		device, err := types.NewSourceDeviceByID(dID)
		if err != nil {
			return nil, err
		}
		m.SourceDevice = device

		metadatas, ok := ret[aid]
		if !ok {
			metadatas = []types.Metadata{}
		}
		ret[aid] = append(metadatas, m)
	}
	return ret, nil
}

// GetAllMetadataByCategory return all assets' metadata
func GetAllMetadataByCategory(ctx context.Context, tx *sql.Tx, category string) (map[int][]types.Metadata, error) {
	rows, err := tx.QueryContext(ctx, "select asset_id, name, value, model, version from metadata_"+category)
	if err != nil {
		return nil, err
	}
	ret := map[int][]types.Metadata{}
	for rows.Next() {
		m := types.Metadata{Category: types.MetadataCategory(category)}
		if err := rows.Scan(&m.AssetID, &m.Name, &m.Value, &m.Model, &m.Version); err != nil {
			return nil, err
		}

		ms, ok := ret[m.AssetID]
		if !ok {
			ms = []types.Metadata{}
		}
		ret[m.AssetID] = append(ms, m)
	}
	return ret, rows.Err()
}

// GetAssetMetadataByCategory return one given asset's metadata at given category
func GetAssetMetadataByCategory(ctx context.Context, tx *sql.Tx, assetID int, category string) ([]types.Metadata, error) {
	rows, err := tx.QueryContext(ctx, "select asset_id, name, value, model, version from metadata_"+category+
		" where asset_id = ?", assetID)
	if err != nil {
		return nil, err
	}
	ret := []types.Metadata{}
	for rows.Next() {
		m := types.Metadata{}
		if err := rows.Scan(&m.AssetID, &m.Name, &m.Value, &m.Model, &m.Version); err != nil {
			return nil, err
		}

		ret = append(ret, m)
	}
	return ret, rows.Err()
}

// DeleteMetadatas deletes one asset's all metadata.
func DeleteMetadatas(ctx context.Context, tx *sql.Tx, assetID int) error {
	for category := range types.AllMetadataCategories {
		_, err := tx.ExecContext(ctx, "delete from metadata_"+category+" where asset_id = ?", assetID)
		if err != nil {
			return err
		}
	}
	return nil
}

// ListMetadataCategories return metadatas in the system already
func ListMetadataCategories(ctx context.Context, tx *sql.Tx, uid int) ([]string, error) {
	categories := []string{}
	for category := range types.AllMetadataCategories {
		count := 0
		err := tx.QueryRowContext(ctx, "select count(*) from metadata_"+category+
			" as metadata inner join asset on asset.id = metadata.asset_id where user_id = ?",
			uid).Scan(&count)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			continue
		}
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return categories, nil
}

// ListMetadataNames return unique names of metadatas for given category
func ListMetadataNames(ctx context.Context, tx *sql.Tx, uid int, category string) ([]string, error) {
	sqlStatement := "select DISTINCT name from metadata_" + category +
		" as metadata inner join asset on asset.id = metadata.asset_id where user_id = ? order by name"
	return listMetadataNameValues(ctx, tx, sqlStatement, uid)
}

// ListMetadataValues return unique values of metadatas for given category
func ListMetadataValues(ctx context.Context, tx *sql.Tx, uid int, category, name string) ([]string, error) {
	sqlStatement := "select DISTINCT value from metadata_" + category +
		" as metadata inner join asset on asset.id = metadata.asset_id where user_id = ? and name = ? order by value"
	return listMetadataNameValues(ctx, tx, sqlStatement, uid, name)
}

func listMetadataNameValues(ctx context.Context, tx *sql.Tx, sqlStatement string, args ...interface{}) ([]string, error) {
	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var n string
		err = rows.Scan(&n)
		if err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, nil
}

// InsertOrUpdateMetadata inserts or update metadata if previous version exist
func InsertOrUpdateMetadata(ctx context.Context, tx *sql.Tx, meta types.Metadata) error {
	did := meta.SourceDevice.ID()
	version, err := getMetadataVersion(ctx, tx, meta.Category.String(), meta.Name, did, meta.AssetID)
	if err != nil {
		if !common.IsErrNoRows(err) {
			return err
		}
		return insertMetadata(ctx, tx, did, meta)
	}
	if version <= meta.Version {
		return nil
	}
	return updateMetadata(ctx, tx, did, meta)
}

func insertMetadata(ctx context.Context, tx *sql.Tx, did int, meta types.Metadata) error {
	stmt, err := tx.Prepare("insert into metadata_" + meta.Category.String() +
		" (source_device, asset_id, name, value, model, version, create_time, last_modified_time) values(?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, did, meta.AssetID, meta.Name, meta.Value,
		meta.Model, meta.Version, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		return err
	}
	if c, err := result.RowsAffected(); err != nil {
		return err
	} else if c == 0 {
		return errors.Errorf("insert empty metadata for %d - (%s, %s, %s)", meta.AssetID,
			meta.Category, meta.Name, meta.Value)
	} else if c != 1 {
		logrus.Warnf("actual insert %d metadata than %v", c, meta)
	}
	return nil
}

func updateMetadata(ctx context.Context, tx *sql.Tx, did int, meta types.Metadata) error {
	stmt, err := tx.Prepare("update metadata_" + meta.Category.String() +
		" set value = ?, model = ?, version = ?, last_modified_time = ? where source_device = ? and asset_id = ? and name = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, meta.Value, meta.Model, meta.Version, time.Now().UTC(),
		did, meta.AssetID, meta.Name)
	if err != nil {
		return err
	}
	if c, err := result.RowsAffected(); err != nil {
		return err
	} else if c == 0 {
		return errors.Errorf("update empty metadata for %d - (%s, %s, %s)", meta.AssetID, meta.Category,
			meta.Name, meta.Value)
	} else if c != 1 {
		logrus.Warnf("actual update %d metadata than %v", c, meta)
	}
	return nil
}

func getTotalCount(ctx context.Context, tx *sql.Tx, sqlStatement string, args ...interface{}) (int, error) {
	count := 0
	err := tx.QueryRowContext(ctx, sqlStatement, args...).Scan(&count)
	return count, err
}

// searchAssets runs countStatement and searchStatement, which take the same args.
func searchAssets(ctx context.Context, tx *sql.Tx, retHash bool, countStatement, searchStatement string,
	args ...interface{}) (interface{}, int, error) {
	count, err := getTotalCount(ctx, tx, countStatement, args...)
	if err != nil {
		return nil, 0, err
	}
	stmt, err := tx.Prepare(searchStatement)
	if err != nil {
		return nil, 0, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, args...)
	if err != nil {
		return nil, 0, err
	}

	defer rows.Close()

	assetNames := []types.AssetName{}
	assetIDs := []int{}
	var id, eid int
	for rows.Next() {
		if retHash {
			n := types.AssetName{}
			err = rows.Scan(&id, &eid, &n.Hash)
			if err != nil {
				return nil, 0, err
			}
			n.Name, err = ext.MkAssetNameByID(id, eid)
			if err != nil {
				logrus.Warnf("asset %d has invalid extension ID %d", id, eid)
				continue
			}
			assetNames = append(assetNames, n)
		} else {
			err = rows.Scan(&id)
			if err != nil {
				return nil, 0, err
			}
			assetIDs = append(assetIDs, id)
		}
	}
	if retHash {
		return assetNames, count, nil
	}
	return assetIDs, count, nil
}

// GetAssetsByMetadata returns assets based query condition and page number
func GetAssetsByMetadata(ctx context.Context, tx *sql.Tx, uid int, queries map[string][]string,
	page, limit int) (interface{}, int, error) {
	//sqlStatement := " from metadata inner join asset on metadata.asset_id = asset.id where user_id=" + strconv.Itoa(uid)
	// Every value from queries goes in as a ? parameter: nvArgs and condArgs
	// hold them in the order their placeholders appear in nvOr and conditions.
	sqlStatement := ""
	categories := []string{}
	nvOr := []string{}
	nvArgs := []interface{}{}
	conditions := []string{}
	condArgs := []interface{}{}
	searchMiss := false
	for k, v := range queries {
		switch k {
		case QueryMetadataNV:
			for _, q := range v {
				var err error
				q, err = url.QueryUnescape(q)
				if err != nil {
					return nil, 0, err
				}
				parts := strings.SplitN(q, common.QueryDelimiterKV, 2)
				nv := []string{}
				if parts[0] != "" {
					nv = append(nv, "name = ?")
					nvArgs = append(nvArgs, parts[0])
				}
				if len(parts) > 1 {
					nv = append(nv, "value like ?")
					nvArgs = append(nvArgs, "%"+parts[1]+"%")
				}
				if len(nv) == 0 {
					continue
				}
				nvOr = append(nvOr, "("+strings.Join(nv, " and ")+")")
			}
		case QueryMetadataCategory:
			if len(v) >= 1 {
				_, ok := types.AllMetadataCategories[v[0]]
				if !ok {
					return nil, 0, common.ErrNotImplementedFormat
				}
				categories = append(categories, v[0])
			}
		case QueryMetadataCategoryMiss:
			if len(v) >= 1 {
				_, ok := types.AllMetadataCategories[v[0]]
				if !ok {
					return nil, 0, common.ErrNotImplementedFormat
				}
				categories = append(categories, v[0])
				searchMiss = true
			}
		case QueryMetadataName:
			if len(v) >= 1 {
				conditions = append(conditions, "name = ?")
				condArgs = append(condArgs, v[0])
			}
		case QueryMetadataNameMiss:
			if len(v) >= 1 {
				conditions = append(conditions, "name = ?")
				condArgs = append(condArgs, v[0])
				searchMiss = true
			}
		case QueryMetadataVersionLess:
			if len(v) >= 1 {
				version, err := strconv.Atoi(v[0])
				if err != nil {
					return nil, 0, common.ErrBadRequest
				}
				conditions = append(conditions, "version < ?")
				condArgs = append(condArgs, version)
			}
		case QueryMetadataSourceDevice:
			if len(v) == 1 {
				sd, err := types.NewSourceDevice(v[0])
				if err != nil {
					return nil, 0, err
				}
				conditions = append(conditions, "source_device="+strconv.Itoa(sd.ID()))
			}
		default:
			continue
		}
	}
	if len(nvOr) > 0 {
		sqlStatement = " (" + strings.Join(nvOr, " or ") + ")"
	}
	if len(conditions) == 0 && len(categories) == 0 {
		return nil, 0, errors.New("no query conditions are provided")
	}
	if len(conditions) != 0 {
		if sqlStatement != "" {
			sqlStatement += " and"
		}
		sqlStatement += " " + strings.Join(conditions, " and ")
	}
	fragmentArgs := append(nvArgs, condArgs...)
	// categories are checked against types.AllMetadataCategories above, so
	// they're safe to use as table names
	if len(categories) == 0 {
		categories = types.AllMetadataCategoryList
	}
	statements := make([]string, len(categories))
	args := []interface{}{}
	for i, c := range categories {
		var s string
		if !searchMiss {
			s = fmt.Sprintf("select distinct asset_id from metadata_%s as metadata inner join asset on metadata.asset_id = asset.id where user_id=%d", c, uid)
			if sqlStatement != "" {
				s += " and " + sqlStatement
			}
		} else {
			s = "select distinct asset_id from metadata_" + c
			if sqlStatement != "" {
				s += " where " + sqlStatement
			}
		}
		statements[i] = s
		if sqlStatement != "" {
			args = append(args, fragmentArgs...)
		}
	}
	if !searchMiss {
		return searchAssets(ctx, tx, false,
			"select count(distinct asset_id) from ("+strings.Join(statements, " union ")+")",
			"select distinct asset_id as id from ("+strings.Join(statements, " union ")+")"+
				mkPageStatement(page*limit, limit), args...)
	}
	sqlStatement = "select distinct asset_id from (" + strings.Join(statements, " union ") + ")"
	sqlStatement = " from asset where user_id=" + strconv.Itoa(uid) +
		" and id not in (" + sqlStatement + ")"
	return searchAssets(ctx, tx, false,
		"select count(distinct id) "+sqlStatement,
		"select distinct id "+sqlStatement+mkPageStatement(page*limit, limit), args...)
}
