package share

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/group"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// DeleteShare deleted one previous share
// XXX: right now, depends on DB to check if shared is belong to the user or not. Need change to detect and return meaning data back
func DeleteShare(ctx context.Context, tx *sql.Tx, shareID, senderID int) error {
	var (
		typ     types.ShareType
		groupID int
	)
	if err := tx.QueryRowContext(ctx, "select share_type, receiver_id from share where id = ? and sender_id = ?", shareID, senderID).Scan(&typ, &groupID); err != nil {
		return errors.Wrapf(err, "while find share ID: %d", shareID)
	}
	if typ == types.ShareToUser {
		if err := unlinkShareForUser(ctx, tx, shareID, senderID); err != nil {
			return err
		}
	} else {
		if err := unlinkShareForGroup(ctx, tx, shareID, senderID, groupID); err != nil {
			return err
		}
	}

	stmt, err := tx.Prepare("delete from share where id = ? and sender_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, shareID, senderID)
	return err
}

func unlinkShareForUser(ctx context.Context, tx *sql.Tx, shareID, senderID int) error {
	var (
		homeDir string
		extID   int
		admin   int
	)
	if err := tx.QueryRowContext(ctx, "select user.home_dir, user.admin, asset.ext_id from share inner join (select id, ext_id from asset) as asset on share.asset_id = asset.id inner join (select id, home_dir, admin from user) as user on share.receiver_id=user.id where share.id = ? and share.sender_id = ?", shareID, senderID).Scan(&homeDir, &admin, &extID); err != nil {
		return errors.Wrapf(err, "while select user info for share %d", shareID)
	}

	if !user.IsBotUser(admin) {
		return nil
	}
	return unlinkShare(homeDir, shareID, extID)
}

func unlinkShare(homeDir string, shareID, extID int) error {
	extStr, err := ext.GetExtString(extID)
	if err != nil {
		return err
	}
	file := filepath.Join(common.GetUserPhotoSharedDir(homeDir), fmt.Sprintf("%d.%s", shareID, extStr))
	return cmd.DeleteFile(file)
}

func unlinkShareForGroup(ctx context.Context, tx *sql.Tx, shareID, senderID, groupID int) error {
	users, err := group.ListGroupMembersEnabledSMB(ctx, tx, groupID)
	if err != nil {
		return err
	}
	extID := 0
	if err := tx.QueryRowContext(ctx, "select asset.ext_id from share inner join (select id, ext_id from asset) as asset on share.asset_id = asset.id where share.id = ? and share.sender_id = ?", shareID, senderID).Scan(&extID); err != nil {
		return errors.Wrapf(err, "while select share %d by group", shareID)
	}

	for _, u := range users {
		if err := unlinkShare(u.HomeDir, shareID, extID); err != nil {
			return err
		}
	}
	return nil
}

func getShareType(ctx context.Context, tx *sql.Tx, shareID uint64) (types.ShareType, int, error) {
	stmt, err := tx.Prepare("select share_type, receiver_id from share where id = ?")
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()

	var (
		typ        types.ShareType
		receiverID int
	)
	err = stmt.QueryRowContext(ctx, shareID).Scan(&typ, &receiverID)
	return typ, receiverID, err
}

func isAssetShared(ctx context.Context, tx *sql.Tx, senderID, receiverID, assetID int, shareType types.ShareType) (uint64, error) {
	sqlStatement := "select id from share where sender_id = ? and receiver_id = ? and asset_id = ? and share_type = ?"
	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var shareID uint64
	err = stmt.QueryRowContext(ctx, senderID, receiverID, assetID, shareType).Scan(&shareID)
	return shareID, err
}

func updateShareAsset(ctx context.Context, tx *sql.Tx, shareID uint64) error {
	stmt, err := tx.Prepare("update share set last_modified_time = ? where id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, time.Now().UTC(), shareID)

	return err
}

func shareAsset(ctx context.Context, tx *sql.Tx, senderID, receiverID, assetID int, shareType types.ShareType, expireTime time.Time) (uint64, error) {
	stmt, err := tx.Prepare("insert into share(share_type, sender_id, receiver_id, asset_id, read_flag, create_time, expire_time, last_modified_time) values (?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, shareType, senderID, receiverID, assetID, 0,
		time.Now().UTC(), expireTime.UTC(), time.Now().UTC())
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	return uint64(id), err
}

// Share add share asset into table
func Share(ctx context.Context, tx *sql.Tx, senderID, assetID int, receiver *user.User,
	shareType types.ShareType, expireTime time.Time, shareFiles *[]string) (uint64, error) {
	shareID, err := isAssetShared(ctx, tx, senderID, receiver.ID, assetID, shareType)
	if err == nil {
		// shared before, update last modified time
		return shareID, updateShareAsset(ctx, tx, shareID)
	}
	if !common.IsErrNoRows(err) {
		return 0, err
	}

	shareID, err = shareAsset(ctx, tx, senderID, receiver.ID, assetID, shareType, expireTime)
	if err != nil {
		return 0, err
	}

	if shareType == types.ShareToGroup {
		return shareID, linkShareForGroup(ctx, tx, senderID, assetID, receiver.ID, shareID, shareFiles)
	}
	if !receiver.IsBotUser() || receiver.IsChromecast() {
		return shareID, nil
	}
	file, err := linkShare(ctx, tx, senderID, assetID, shareID, receiver.HomeDir)
	if err != nil {
		return 0, err
	}

	// append and return, otherwise, chown here
	if shareFiles != nil {
		*shareFiles = append(*shareFiles, file)
		return shareID, nil
	}
	return shareID, cmd.Chown(receiver.Name+":"+common.GetLomoGroupName(), file)
}

func linkShare(ctx context.Context, tx *sql.Tx, senderID, assetID int, shareID uint64,
	receiverHomeDir string) (string, error) {
	master, _, err := asset.GetAssetMasterPreviewPath(ctx, tx, senderID, assetID, 0, 0, nil, nil, 0)
	if err != nil {
		return "", err
	}
	file := filepath.Join(common.GetUserPhotoSharedDir(receiverHomeDir), fmt.Sprintf("%d%s", shareID, filepath.Ext(master)))
	return file, cmd.Link(master, file)
}

func linkShareForGroup(ctx context.Context, tx *sql.Tx, senderID, assetID, receiverID int,
	shareID uint64, sharedFiles *[]string) error {
	users, err := group.ListGroupMembersEnabledSMB(ctx, tx, receiverID)
	if err != nil {
		return err
	}
	for _, u := range users {
		if !u.IsBotUser() || u.IsChromecast() {
			continue
		}
		f, err := linkShare(ctx, tx, senderID, assetID, shareID, u.HomeDir)
		if err != nil {
			return err
		}
		if sharedFiles != nil {
			*sharedFiles = append(*sharedFiles, f)
			continue
		}
		// only chown after all are done
		// TODO: add rollback and delete share id
		if err := cmd.Chown(u.Name+":"+common.GetLomoGroupName(), f); err != nil {
			logrus.Errorf("chown %s: %v", u.Name, err)
		}
	}

	return nil
}

// ReceiveHistoryByUser returns the list of receive history
func ReceiveHistoryByUser(ctx context.Context, tx *sql.Tx, senderID, receiverID int, includeme bool) (*types.Records, error) {
	sqlStatement := `
select id, share_type, sender_id, receiver_id, asset_id, ext_id, read_flag, last_modified_time from
  (select * from share inner join (select id, ext_id from asset) as a on share.asset_id = a.id
	    where (share_type = ? or share_type = ?) and sender_id = ? and receiver_id = ? and 
			  not exists (select NULL from assets_hide ah 
				            where ah.share_id = share.id and ah.receiver_id = share.receiver_id)
	)`
	args := []interface{}{types.ShareToUser, types.CastToUser, senderID, receiverID}

	if includeme {
		sqlStatement = sqlStatement + " union " + sqlStatement
		args = append(args, types.ShareToUser, types.CastToUser, receiverID, senderID)
	}

	sqlStatement = sqlStatement + " order by last_modified_time DESC"

	return recordHistory(ctx, tx, sqlStatement, args)
}

// ReceiveHistoryByGroup returns the list of receive history
func ReceiveHistoryByGroup(ctx context.Context, tx *sql.Tx, groupID, receiverID int, includeme bool) (*types.Records, error) {
	// check if the user is in the group or not
	// Note that in this case, senderID is receiver name
	exist, err := group.IsUserExist(ctx, tx, groupID, receiverID)
	if err != nil {
		return nil, err
	}
	if !exist {
		return nil, common.ErrNotInGroup
	}

	sqlStatement := `
select id, share_type, sender_id, receiver_id, asset_id, ext_id, read_flag, last_modified_time from 
  (select * from share as s inner join (select id, ext_id from asset) as a on s.asset_id = a.id) 
where share_type = ? and not exists 
  (select NULL from assets_hide ah where ah.share_id = id and ah.group_id = ?)`
	args := []interface{}{types.ShareToGroup, groupID}

	if includeme {
		sqlStatement = sqlStatement + " and receiver_id = ?"
		args = append(args, groupID)
	} else {
		sqlStatement = sqlStatement + " and sender_id != ? and receiver_id = ?"
		args = append(args, receiverID, groupID)
	}
	sqlStatement = sqlStatement + " order by last_modified_time DESC"

	records, err := recordHistory(ctx, tx, sqlStatement, args)
	if err != nil {
		return nil, err
	}

	for _, r := range records.Records {
		r.ReceiverID = groupID
	}

	return records, nil
}

func recordHistory(ctx context.Context, tx *sql.Tx, sqlStatement string, args []interface{}) (*types.Records, error) {
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

	records := &types.Records{Records: []*types.Record{}}
	for rows.Next() {
		var (
			r     types.Record
			t     string
			aid   int
			extID int
		)
		//var t time.Time
		err = rows.Scan(&r.ID, &r.Type, &r.SenderID, &r.ReceiverID, &aid, &extID, &r.ReadFlag, &t)
		if err != nil {
			if common.IsErrNoRows(err) {
				return records, nil
			}
			return nil, err
		}

		r.AssetID, err = ext.MkAssetNameByID(aid, extID)
		if err != nil {
			return nil, err
		}
		r.ShareTime, err = types.ParseDBTime(t)
		if err != nil {
			return nil, err
		}
		records.Records = append(records.Records, &r)
	}

	return records, rows.Err()
}

func list(tx *sql.Tx, sqlStatement string, args []interface{}) ([]int, error) {
	l := []int{}
	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r int
		err = rows.Scan(&r)
		if err != nil {
			if common.IsErrNoRows(err) {
				return l, nil
			}
			return nil, err
		}
		l = append(l, r)
	}

	return l, rows.Err()
}

func listAsset(tx *sql.Tx, sqlStatement string, args []interface{}) (*types.Records, error) {
	l := &types.Records{Records: []*types.Record{}}
	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			r     types.Record
			t     string
			aid   int
			extID int
		)

		err := rows.Scan(&r.ID, &r.Type, &r.SenderID, &r.ReceiverID, &aid, &extID, &t, &r.ReadFlag)
		if err != nil {
			if common.IsErrNoRows(err) {
				return l, nil
			}
			return nil, err
		}
		r.AssetID, err = ext.MkAssetNameByID(aid, extID)
		if err != nil {
			return nil, err
		}
		r.ShareTime, err = types.ParseDBTime(t)
		if err != nil {
			return nil, err
		}
		l.Records = append(l.Records, &r)
	}

	return l, rows.Err()
}

// GetAllReceiveHistory returns which group and users has sent message to the user
func GetAllReceiveHistory(ctx context.Context, tx *sql.Tx, receiverID int) (*types.ReceiveRecords, error) {
	records := &types.ReceiveRecords{}

	var err error
	records.Users, err = list(tx, "select distinct sender_id from share where (share_type = 0 or share_type=2) and receiver_id = ?", []interface{}{receiverID})
	if err != nil && !common.IsErrNoRows(err) {
		return nil, err
	}

	records.Groups, err = list(tx, "select distinct id from (select id from groups inner join (select sender_id, receiver_id from share where share_type = 1 and sender_id != ?) as s on groups.id = s.receiver_id inner join (select group_id from member where user_id = ?) as m on m.group_id = s.receiver_id)", []interface{}{receiverID, receiverID})
	if err != nil && !common.IsErrNoRows(err) {
		return nil, err
	}

	return records, nil
}

// GetAllReceiveHistoryByAssets returns which group and users has sent message to the user
func GetAllReceiveHistoryByAssets(ctx context.Context, tx *sql.Tx, receiverID int) (*types.Records, error) {
	records, err := listAsset(tx, `
select id, share_type, sender_id, receiver_id, asset_id, ext_id, last_modified_time, read_flag
from (select * from share inner join (select id as aid, ext_id from asset) as a on a.aid = share.asset_id
      where (share_type = 0 or share_type=2) and receiver_id = ? and not exists (select NULL from assets_hide ah 
			    where ah.share_id = share.id and ah.receiver_id = share.receiver_id)
		 )
order by last_modified_time DESC`, []interface{}{receiverID})
	if err != nil && !common.IsErrNoRows(err) {
		return nil, err
	}

	recordsGroups, err := listAsset(tx, `
select id, share_type, sender_id, receiver_id, asset_id, ext_id, last_modified_time, read_flag
from (select * from share inner join (select id as aid, ext_id from asset) as a on a.aid = share.asset_id inner join
          (select group_id, user_id from member where user_id = ?) as m on m.group_id = share.receiver_id
		 where share_type = 1 and sender_id != ? and not exists (select NULL from assets_hide ah
		     where ah.share_id = share.id and ah.group_id = share.receiver_id))
order by last_modified_time DESC`, []interface{}{receiverID, receiverID})
	if err != nil && !common.IsErrNoRows(err) {
		return nil, err
	}

	records.Records = append(records.Records, recordsGroups.Records...)
	sort.Sort(types.ByRecordsShareTime(records.Records))
	return records, nil
}

// LocateAssetPath resolves the given share's underlying asset's master file path, preview
// directory, preview filename prefix, asset ID, and extension ID via DB lookups only -- no
// filesystem or transcode work. Callers should follow up with ResolveAssetPreview outside
// their transaction; see LocateAsset for the combined, transaction-spanning convenience
// version, which holds tx for the duration of any preview generation and so isn't safe for
// hot paths where that can be slow (see asset.GenerateAssetPreview).
func LocateAssetPath(ctx context.Context, tx *sql.Tx, shareID uint64, receiverID int,
	folderPerm os.FileMode) (masterFile, previewPath, assetPreviewPrefix string, assetID, extID int, err error) {
	stmt, err := tx.Prepare("select sender_id, receiver_id, asset_id, share_type from share where id = ?")
	if err != nil {
		return "", "", "", 0, 0, err
	}
	defer stmt.Close()

	var sID, rID, aID int
	var shareType types.ShareType

	if err := stmt.QueryRowContext(ctx, shareID).Scan(&sID, &rID, &aID, &shareType); err != nil {
		return "", "", "", 0, 0, err
	}

	logrus.Infof("find senderid: %d, receiverid: %d, assetid: %d for share %d", sID, rID, aID, shareID)

	// return error if it is not sent to me, or the group I'm belonging
	switch shareType {
	case types.CastToUser:
	case types.ShareToUser:
		if rID != receiverID && sID != receiverID {
			return "", "", "", 0, 0, common.ErrAssetNotSharedToUser
		}
	case types.ShareToGroup:
		exist, err := group.IsUserExist(ctx, tx, rID, receiverID)
		if err != nil {
			return "", "", "", 0, 0, err
		} else if !exist {
			return "", "", "", 0, 0, common.ErrNotInGroup
		}
	default:
		return "", "", "", 0, 0, fmt.Errorf("not implemented share type for locate asset: %d", shareType)
	}

	masterFile, previewPath, assetPreviewPrefix, extID, err = asset.GetAssetPath(ctx, tx, sID, aID, folderPerm)
	if err != nil {
		if common.IsErrNoRows(err) {
			err = common.ErrNotExistAsset
		}
		return "", "", "", 0, 0, err
	}
	return masterFile, previewPath, assetPreviewPrefix, aID, extID, nil
}

// ResolveAssetPreview generates (or finds cached) the preview file for an asset already
// resolved via LocateAssetPath. It touches only the filesystem and, on a cache miss, an
// external transcoder -- no database access -- so it's safe to call outside of a DB
// transaction. previewRunner == nil returns the master file path unchanged, matching
// LocateAsset's long-standing "no codec requested" behavior.
func ResolveAssetPreview(ctx context.Context, masterFile, previewPath, assetPreviewPrefix string,
	assetID, extID, width, height int, previewRunner types.PreviewRunner, folderPerm os.FileMode) (string, error) {
	var previewCodec *int
	if previewRunner != nil {
		icodec := ext.JPG
		previewCodec = &icodec
	}
	masterfile, previewfile, err := asset.GenerateAssetPreview(ctx, masterFile, previewPath, assetPreviewPrefix,
		assetID, extID, uint(width), uint(height), previewCodec, previewRunner, folderPerm)
	if previewRunner != nil {
		return previewfile, err
	}
	return masterfile, err
}

// LocateAsset find the asset path. Holds tx for the duration of any preview generation -- see
// LocateAssetPath/ResolveAssetPreview for hot paths where that transcode can be slow and
// holding a DB transaction across it would starve unrelated requests.
func LocateAsset(ctx context.Context, tx *sql.Tx, shareID uint64, receiverID, width, height int,
	previewRunner types.PreviewRunner, folderPerm os.FileMode) (string, error) {
	masterFile, previewPath, assetPreviewPrefix, assetID, extID, err :=
		LocateAssetPath(ctx, tx, shareID, receiverID, folderPerm)
	if err != nil {
		return "", err
	}
	return ResolveAssetPreview(ctx, masterFile, previewPath, assetPreviewPrefix, assetID, extID,
		width, height, previewRunner, folderPerm)
}

// GetShareIDs returns all share for one user's asset
func GetShareIDs(ctx context.Context, tx *sql.Tx, senderID, aid int) ([]int, error) {
	q := "select id from share where sender_id = ? and asset_id = ?"

	shareIDs := []int{}
	stmt, err := tx.Prepare(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(senderID, aid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		err := rows.Scan(&id)
		if err != nil {
			if common.IsErrNoRows(err) {
				return shareIDs, nil
			}
			return nil, err
		}
		shareIDs = append(shareIDs, id)
	}

	return shareIDs, nil
}

// HideReceivedShare hides one asset shared to user
func HideReceivedShare(ctx context.Context, tx *sql.Tx, receiverID int, shareID uint64) error {
	typ, rid, err := getShareType(ctx, tx, shareID)
	if err != nil {
		if common.IsErrNoRows(err) {
			return common.ErrAssetNotSharedToUser
		}
		return err
	}
	var groupID int
	if typ == types.ShareToGroup {
		groupID = rid
		exist, err := group.IsUserExist(ctx, tx, rid, receiverID)
		if err != nil {
			return err
		} else if !exist {
			return common.ErrNotInGroup
		}
	} else if rid != receiverID {
		return common.ErrAssetNotSharedToUser
	}

	stmt, err := tx.Prepare("insert into assets_hide(share_id, receiver_id, group_id, create_time) values (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, shareID, receiverID, groupID, time.Now().UTC())
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		logrus.Errorf("insert %d hide asset, but expect only 1 asset", count)
		return errors.New("insert hide asset failure")
	}
	return nil
}
