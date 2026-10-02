package album

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type AlbumPhoto struct {
	Hidden          bool   `json:"hidden"`
	Favorited       bool   `json:"favorited"`
	Public          bool   `json:"public"`
	ImageURL        string `json:"image_url"`
	ExifTimestamp   string `json:"exif_timestamp"`
	ImageHash       string `json:"image_hash"`
	ThumbnailURL    string `json:"thumbnail_url"`
	ThumbnailWidth  uint   `json:"thumbnail_width"`
	ThumbnailHeight uint   `json:"thumbnail_height"`
}

type AlbumPeople struct {
	ID          int          `json:"id"`
	Title       string       `json:"title"`
	Photos      []AlbumPhoto `json:"photos"`
	CoverPhotos []AlbumPhoto `json:"cover_photos"` // hash slice
}

type AlbumPlace struct {
	ID          int          `json:"id"`
	Title       string       `json:"title"`
	Photos      []AlbumPhoto `json:"photos"`
	CoverPhotos []AlbumPhoto `json:"cover_photos"` // hash slice
}

type AlbumThing struct {
	ID          int          `json:"id"`
	Title       string       `json:"title"`
	Photos      []AlbumPhoto `json:"photos"`
	CoverPhotos []AlbumPhoto `json:"cover_photos"` // hash slice
}

// AlbumLibre is request structure for createalbum request
type AlbumLibre struct {
	ID               int    `json:"id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Author           string `json:"author"`
	CoverPhoto       string `json:"cover_photos"`
	SharedTo         string `json:"shared_to"`
	PhotoCount       string `json:"photo_count"`
	CreateTime       string `json:"create_time"`
	LastModifiedTime string `json:"last_modified_time"`
}

// Album is request structure for createalbum request
type Album struct {
	ID               int
	Title            string
	Description      string
	Author           string
	CreateTime       string
	LastModifiedTime string
	CoverImage       string
	// AssetsCount is only filled in by ListAlbums; other responses omit it.
	AssetsCount int `json:",omitempty"`
}

// Albums is response structure for listalbum request
type Albums struct {
	Albums []Album
}

// ListAlbums list all albums which the user has
func ListAlbums(ctx context.Context, tx *sql.Tx, userid int) (*Albums, error) {
	// One grouped join rather than a count per album: asset_album is only
	// indexed on (asset_id, album_id), so each per-album count is a table scan.
	stmt, err := tx.Prepare(`select album.id, title, description, author, album.create_time, last_modified_time, cover_image, count(aa.asset_id)
		from album left join asset_album as aa on aa.album_id = album.id
		where user_id = ? group by album.id order by title`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, userid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	albums := &Albums{Albums: []Album{}}
	for rows.Next() {
		ct := ""
		ldt := ""
		a := Album{}
		err := rows.Scan(&a.ID, &a.Title, &a.Description, &a.Author, &ct, &ldt, &a.CoverImage, &a.AssetsCount)
		if err != nil {
			return nil, common.ReturnCheckErrNoRows(err)
		}
		a.CreateTime, err = common.ParseAndFormatTime(ct)
		if err != nil {
			return nil, err
		}
		a.LastModifiedTime, err = common.ParseAndFormatTime(ldt)
		if err != nil {
			return nil, err
		}
		albums.Albums = append(albums.Albums, a)
	}
	if err := rows.Err(); err != nil {
		return nil, common.ReturnCheckErrNoRows(err)
	}

	return albums, err
}

// GetTotalAssets returns total assets in one album by its ID
func GetTotalAssets(ctx context.Context, tx *sql.Tx, albumid int) (int, error) {
	stmt, err := tx.Prepare("select count(asset_id) from asset_album where album_id = ?")
	if err != nil {
		return -1, err
	}
	defer stmt.Close()
	var count int
	return count, stmt.QueryRowContext(ctx, albumid).Scan(&count)
}

// GetCoverPhoto returns the first assets in one album by its ID
func GetCoverPhoto(ctx context.Context, tx *sql.Tx, albumid int) (int, error) {
	stmt, err := tx.Prepare("select asset_id from asset_album where album_id = ? order by asset_id desc")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var id int
	return id, stmt.QueryRowContext(ctx, albumid).Scan(&id)
}

// IsUserAlbumExist checks if given album belongs to the user or not
func IsUserAlbumExist(ctx context.Context, tx *sql.Tx, userid int, albumTitle string) (id int, rerr error) {
	stmt, rerr := tx.Prepare("select id from album where title = ? and user_id = ?")
	if rerr != nil {
		return
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			if rerr != nil {
				rerr = errors.Wrapf(rerr, "%v", err)
			}
			rerr = err
		}
	}()

	rerr = stmt.QueryRowContext(ctx, albumTitle, userid).Scan(&id)
	return
}

// CreateAlbum creates the album
func CreateAlbum(ctx context.Context, tx *sql.Tx, userid int, album Album) (int64, error) {
	stmt, err := tx.Prepare("insert into album (user_id, title, description, author, create_time, last_modified_time, cover_image) values(?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return -1, err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, userid, album.Title, album.Description, album.Author,
		time.Now().UTC(), time.Now().UTC(), album.CoverImage)
	if err != nil {
		return -1, err
	}
	return result.LastInsertId()
}

// GetAlbum gets one album
func GetAlbum(ctx context.Context, tx *sql.Tx, albumid int) (*Album, error) {
	stmt, err := tx.Prepare("select title, description, author, create_time, cover_image from album where id = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	dt := ""
	album := &Album{ID: albumid}
	err = stmt.QueryRowContext(ctx, albumid).Scan(&album.Title, &album.Description, &album.Author, &dt, &album.CoverImage)
	if err != nil {
		return nil, err
	}
	album.CreateTime, err = common.ParseAndFormatTime(dt)
	return album, err
}

// GetAlbumByTitle gets one album by its title
func GetAlbumByTitle(ctx context.Context, tx *sql.Tx, title string) (*Album, error) {
	stmt, err := tx.Prepare("select id, description, author, create_time, cover_image from album where title = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	dt := ""
	album := &Album{Title: title}
	err = stmt.QueryRowContext(ctx, title).Scan(&album.ID, &album.Description, &album.Author, &dt, &album.CoverImage)
	if err != nil {
		if common.IsErrNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	album.CreateTime, err = common.ParseAndFormatTime(dt)
	return album, err
}

// DeleteAlbum deletes the album
func DeleteAlbum(ctx context.Context, tx *sql.Tx, userid, albumid int) error {
	stmt, err := tx.Prepare("delete from album where id = ? and user_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, albumid, userid)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	} else if count != 1 {
		logrus.Errorf("delete album affect %d rows", count)
		return common.ErrBadRequest
	}
	return nil
}

// UpdateAlbum update the album
func UpdateAlbum(ctx context.Context, tx *sql.Tx, userid int, album Album) error {
	// if author is lomod, it is system created, so can not be updated
	author := ""
	err := tx.QueryRowContext(ctx, "select author from album where user_id = ? and id = ?", userid, album.ID).Scan(&author)
	if err != nil {
		return err
	}
	var args []interface{}
	statement := ""
	if author == common.AlbumAuthorInternal {
		statement = "update album set title = ?, description = ?, last_modified_time = ? where user_id = ? and id = ?"
		args = []interface{}{album.Title, album.Description, time.Now().UTC(), userid, album.ID}
	} else {
		statement = "update album set title = ?, description = ?, author = ?, last_modified_time = ?, cover_image = ? where user_id = ? and id = ?"
		args = []interface{}{album.Title, album.Description, album.Author, time.Now().UTC(), album.CoverImage, userid, album.ID}
	}
	stmt, err := tx.Prepare(statement)
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	} else if count != 1 {
		return errors.Errorf("update album affect %d rows", count)
	}
	return nil
}

// ValidateAlbumUser validates if the user owns the album
func ValidateAlbumUser(ctx context.Context, tx *sql.Tx, userid, albumid int) error {
	stmt, err := tx.Prepare("select id from album where id = ? and user_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	var id int
	return stmt.QueryRowContext(ctx, albumid, userid).Scan(&id)
}

// ListAssetIDs returns the list of asset ids in given album
func ListAssetIDs(ctx context.Context, tx *sql.Tx, albumid, page, limit int) ([]string, error) {
	stmt, err := tx.Prepare("select asset_id, ext_id from asset_album as aa inner join asset as a on aa.asset_id = a.id where album_id = ? order by asset_id DESC limit ? offset ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, albumid, limit, page*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id, eid int
		err = rows.Scan(&id, &eid)
		if err != nil {
			return nil, err
		}

		name, err := ext.MkAssetNameByID(id, eid)
		if err != nil {
			logrus.Warnf("asset %d has invalid extension ID %d", id, eid)
			continue
		}
		ids = append(ids, name)
	}

	return ids, rows.Err()
}

// ListAssetIDsWithDate returns the list of asset ids including dates in given album
func ListAssetIDsWithDate(ctx context.Context, tx *sql.Tx, albumid int) ([]string, error) {
	stmt, err := tx.Prepare("select asset_id, ext_id, year, month, day from asset_album as aa inner join asset as a on aa.asset_id = a.id where album_id = ? order by year, month, day, asset_id DESC")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, albumid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id, eid, y, m, d int
		err = rows.Scan(&id, &eid, &y, &m, &d)
		if err != nil {
			return nil, err
		}
		name, err := ext.MkAssetNameByID(id, eid)
		if err != nil {
			logrus.Warnf("asset %d has invalid extension ID %d", id, eid)
			continue
		}
		ids = append(ids, ext.NormalizeAssetNameString(y, m, d, name))
	}

	return ids, rows.Err()
}

// ListAssetNames returns the list of asset names in given album
func ListAssetNames(ctx context.Context, tx *sql.Tx, albumid, page, limit int) ([]string, error) {
	stmt, err := tx.Prepare("select asset_id, a.ext_id from asset_album as aa inner join asset as a on aa.asset_id = a.id where album_id = ? order by asset_id DESC limit ? offset ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, albumid, limit, page*limit)
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

// ListAssets returns the list of assets in given album
func ListAssets(ctx context.Context, tx *sql.Tx, albumid, page, limit int) ([]types.AssetName, error) {
	stmt, err := tx.Prepare("select asset_id, ext_id, a.hash from asset_album as aa inner join asset as a on aa.asset_id = a.id where album_id = ? order by asset_id DESC limit ? offset ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, albumid, limit, page*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []types.AssetName{}
	for rows.Next() {
		var aid, eid int
		id := types.AssetName{}
		err = rows.Scan(&aid, &eid, &id.Hash)
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

func GetTargetAlbumIdToMerge(ctx context.Context, tx *sql.Tx, albumIDs []int) (int, error) {
	maxCount := 0
	targetId := -1
	for _, id := range albumIDs {
		count, err := GetTotalAssets(ctx, tx, id)
		if err != nil {
			return -1, err
		}
		if maxCount < count {
			maxCount = count
			targetId = id
		}
	}

	if targetId == -1 {
		return -1, errors.Errorf("no target albums found")
	}

	return targetId, nil
}

// Merge albums with new title
func MergeAlbum(ctx context.Context, tx *sql.Tx, title string, userid int, albumIDs []int) error {
	if len(albumIDs) <= 1 {
		return errors.Errorf("need to merge at least two albums")
	}

	// find album_id with largest asset count to merge into
	targetAlbumID, err := GetTargetAlbumIdToMerge(ctx, tx, albumIDs)

	// update other `album_id`s in asset_album table with the merge target album id
	// delete other `album_id`s and update title
	for _, id := range albumIDs {
		if id != targetAlbumID {
			err := RemoveSameAssetsInAlbums(ctx, tx, id, targetAlbumID)
			if err != nil {
				logrus.Errorf("MergeAlbum remove same assets in albums error, album id:%d -> %d, got %v", id, targetAlbumID, err)
				return err
			}

			err = UpdateAlbumIDInAssetAlbum(ctx, tx, id, targetAlbumID)
			if err != nil {
				logrus.Errorf("MergeAlbum update asset_album error, album id:%d -> %d, got %v", id, targetAlbumID, err)
				return err
			}

			err = DeleteAlbum(ctx, tx, userid, id)
			if err != nil {
				logrus.Errorf("MergeAlbum delete album error, album id:%d, got %v", id, err)
				return err
			}
		}
	}

	album, err := GetAlbum(ctx, tx, targetAlbumID)
	if err != nil {
		logrus.Errorf("MergeAlbum get album error, album id:%d, got %v", targetAlbumID, err)
		return err
	}

	album.Title = title
	err = UpdateAlbum(ctx, tx, userid, *album)
	if err != nil {
		logrus.Errorf("MergeAlbum update album error, album id:%d, got %v", targetAlbumID, err)
		return err
	}

	return nil
}

// DeleteAssets delete assets from album by their ID. AlbumID is -1 means delete from all albums
func DeleteAssets(ctx context.Context, tx *sql.Tx, albumID int, assetIDs []int) error {
	ids := ""
	for i, id := range assetIDs {
		ids += strconv.Itoa(id)
		if i != len(assetIDs)-1 {
			ids += ","
		}
	}
	statement := ""
	if albumID == -1 {
		statement = fmt.Sprintf("delete from asset_album where asset_id in (%s)", ids)
	} else {
		statement = fmt.Sprintf("delete from asset_album where album_id=%d and asset_id in (%s)",
			albumID, ids)
	}
	_, err := tx.ExecContext(ctx, statement)
	return err
}

// AddAssets adds assets into one album
func AddAssets(ctx context.Context, tx *sql.Tx, albumID int, assetIDs map[int]string) error {
	statement := "insert into asset_album (asset_id, album_id, orig_filename, create_time) values "
	times := []interface{}{}
	aid := strconv.Itoa(albumID)
	i := 0
	for id, name := range assetIDs {
		times = append(times, time.Now().UTC())
		statement += "(" + strconv.Itoa(id) + "," + aid + ",'" + name + "', ?)"
		if i < len(assetIDs)-1 {
			statement += ","
		}
		i++
	}
	stmt, err := tx.Prepare(statement)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, times...)
	return err
}

// IsAssetInAlbum checks if given asset in given album
func IsAssetInAlbum(ctx context.Context, tx *sql.Tx, albumID, assetID int) error {
	stmt, err := tx.Prepare("select album_id from asset_album where album_id = ? and asset_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	var id int
	return stmt.QueryRowContext(ctx, albumID, assetID).Scan(&id)
}

// ListAlbumsByAssetID returns the list of albums having given asset ID
func ListAlbumsByAssetID(ctx context.Context, tx *sql.Tx, assetID string) ([]Album, error) {
	statement := ""
	if len(assetID) == 40 {
		statement = "select album.id, title, description, author from album inner join asset_album as aa on album.id=aa.album_id inner join asset on aa.asset_id=asset.id where asset.hash = ? order by album.id DESC"
	} else {
		statement = "select id, title, description, author from album inner join asset_album as aa on album.id=aa.album_id where aa.asset_id = ? order by id DESC"
	}
	stmt, err := tx.Prepare(statement)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	as := []Album{}
	for rows.Next() {
		a := Album{}
		err = rows.Scan(&a.ID, &a.Title, &a.Description, &a.Author)
		if err != nil {
			return nil, err
		}

		as = append(as, a)
	}

	return as, rows.Err()
}

// RemoveSameAssetsInAlbums could have same assets in both albums, need to avoid constrain conflicts when updating album id
func RemoveSameAssetsInAlbums(ctx context.Context, tx *sql.Tx, oldAlbumID int, newAlbumID int) error {
	statement := "delete from asset_album where (asset_id, album_id) in "
	statement += "(select asset_id, album_id from asset_album where album_id=? or album_id=? group by asset_id having count(*)>1);"
	args := []interface{}{newAlbumID, oldAlbumID}
	stmt, err := tx.Prepare(statement)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, args...)
	if err != nil {
		return err
	}
	return nil
}

// UpdateAlbumIDInAssetAlbum update the album id in table asset_album
func UpdateAlbumIDInAssetAlbum(ctx context.Context, tx *sql.Tx, oldAlbumID int, newAlbumID int) error {
	statement := "update asset_album set album_id = ? where album_id = ?"
	args := []interface{}{newAlbumID, oldAlbumID}
	stmt, err := tx.Prepare(statement)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, args...)
	if err != nil {
		return err
	}
	return nil
}
