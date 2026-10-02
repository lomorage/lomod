package group

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/user"
)

// Group is request structure for creategroup request.
type Group struct {
	ID      int          `json:"ID,omitempty"`
	Name    string       `json:"Name,omitempty"`
	OwnerID int          `json:"OwnerID,omitempty"`
	Members []*user.User `json:"Members,omitempty"`
}

// Groups is response structure for listgroup request.
type Groups struct {
	Groups []Group
}

func listGroup(ctx context.Context, tx *sql.Tx, userid int) (*Groups, error) {
	stmt, err := tx.Prepare("select id, group_name, owner_id from (select * from groups inner join (select * from member) as m on groups.id = m.group_id where m.user_id = ?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, userid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := &Groups{Groups: []Group{}}
	for rows.Next() {
		g := Group{}
		err := rows.Scan(&g.ID, &g.Name, &g.OwnerID)
		if err != nil {
			return nil, common.ReturnCheckErrNoRows(err)
		}
		groups.Groups = append(groups.Groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, common.ReturnCheckErrNoRows(err)
	}
	return groups, err
}

// ListGroup list all groups which the user belongs to.
func ListGroup(ctx context.Context, tx *sql.Tx, userid int) (*Groups, error) {
	groups, err := listGroup(ctx, tx, userid)
	if err != nil {
		return nil, err
	}
	for i := range groups.Groups {
		groups.Groups[i].Members, err = ListGroupMembers(ctx, tx, groups.Groups[i].ID)
		if err != nil {
			return nil, common.ReturnCheckErrNoRows(err)
		}
	}
	return groups, nil
}

// err is not nil means no group is created by the owner.
func isGroupExist(ctx context.Context, tx *sql.Tx, groupName string, ownerid int) (rerr error) {
	stmt, err := tx.Prepare("select id from groups where group_name = ? and owner_id = ?")
	if err != nil {
		return err
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			if rerr != nil {
				rerr = errors.Wrapf(rerr, "%v", err)
			}
			rerr = err
		}
	}()

	var id int
	return stmt.QueryRowContext(ctx, groupName, ownerid).Scan(&id)
}

// err is not nil means no group is created by the owner.
func isGroupExistByID(ctx context.Context, tx *sql.Tx, groupID int) (rerr error) {
	stmt, err := tx.Prepare("select owner_id from groups where id = ?")
	if err != nil {
		return err
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			if rerr != nil {
				rerr = errors.Wrapf(rerr, "%v", err)
			}
			rerr = err
		}
	}()

	ownerID := 0
	err = stmt.QueryRowContext(ctx, groupID).Scan(&ownerID)
	return err
}

func isGroupOwner(ctx context.Context, tx *sql.Tx, groupID, ownerID int) (rerr error) {
	stmt, err := tx.Prepare("select id from groups where id = ? and owner_id = ?")
	if err != nil {
		return err
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			if rerr != nil {
				rerr = errors.Wrapf(rerr, "%v", err)
			}
			rerr = err
		}
	}()

	id := 0
	err = stmt.QueryRowContext(ctx, groupID, ownerID).Scan(&id)
	return err
}

// IsUserExistByGroupName checks if given user is in the group or not
// err is not nil means no user in this group.
func IsUserExistByGroupName(ctx context.Context, tx *sql.Tx, groupName string, userid int) (groupID int, rerr error) {
	stmt, err := tx.Prepare("select id from groups where group_name = ? and user_id = ?")
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			if rerr != nil {
				rerr = errors.Wrapf(rerr, "%v", err)
			}
			rerr = err
		}
	}()

	err = stmt.QueryRowContext(ctx, groupName, userid).Scan(&groupID)
	return groupID, err
}

// GetGroupID returns groupID by its groupName.
// FIXME: user at 2 different groups with same name, created by different owner???
func GetGroupID(ctx context.Context, tx *sql.Tx, groupName string, userID int) (int, error) {
	stmt, err := tx.Prepare("select id from groups where group_name = ? and user_id = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var groupID int
	if err := stmt.QueryRowContext(ctx, groupName, userID).Scan(&groupID); err != nil {
		return 0, common.ErrNotExistUser
	}

	return groupID, nil
}

// IsUserExist checks if given user is in the group or not
// err is not nil means no user in this group.
func IsUserExist(ctx context.Context, tx *sql.Tx, groupid, userid int) (exist bool, rerr error) {
	stmt, err := tx.Prepare("select group_id from member where group_id = ? and user_id = ?")
	if err != nil {
		return false, err
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			if rerr != nil {
				rerr = errors.Wrapf(rerr, "%v", err)
			}
			rerr = err
		}
	}()

	id := 0
	err = stmt.QueryRowContext(ctx, groupid, userid).Scan(&id)
	if err == nil {
		return true, err
	} else if common.IsErrNoRows(err) {
		return false, nil
	}
	return false, err
}

func createGroup(ctx context.Context, tx *sql.Tx, groupName string, ownerID int) (int64, error) {
	stmt, err := tx.Prepare("insert into groups(group_name, owner_id, create_time, last_modified_time) values(?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, groupName, ownerID, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// CreateGroup creates the group.
func CreateGroup(ctx context.Context, tx *sql.Tx, groupName string, ownerID int, memberIDs []*user.User) (*Group, error) {
	if groupName == "" {
		return nil, common.ErrEmptyGroupName
	}

	// if group is exist by the owner, return group exist error
	if err := isGroupExist(ctx, tx, groupName, ownerID); err == nil {
		return nil, common.ErrGroupExist
	}

	// create group firstly, then add member one by one
	groupID, err := createGroup(ctx, tx, groupName, ownerID)
	if err != nil {
		return nil, err
	}

	// add members
	foundOwner := false
	g := &Group{ID: int(groupID), Name: groupName, OwnerID: ownerID, Members: []*user.User{}}
	for _, m := range memberIDs {
		member := &user.User{ID: m.ID}
		if m.ID == ownerID {
			foundOwner = true
		}

		member.Name, err = user.IsExistByID(ctx, tx, m.ID)
		if err != nil {
			return nil, err
		}

		if err := addMember(ctx, tx, int(groupID), m.ID); err != nil {
			return nil, err
		}
		g.Members = append(g.Members, member)
	}
	if !foundOwner {
		if err := addMember(ctx, tx, int(groupID), ownerID); err != nil {
			return nil, err
		}
		name, err := user.IsExistByID(ctx, tx, ownerID)
		if err != nil {
			return nil, err
		}
		g.Members = append(g.Members, &user.User{ID: ownerID, Name: name})
	}

	return g, nil
}

// ListGroupMembers returns the list of member in given group.
func ListGroupMembers(ctx context.Context, tx *sql.Tx, groupID int) ([]*user.User, error) {
	if err := isGroupExistByID(ctx, tx, groupID); err != nil {
		return nil, err
	}

	stmt, err := tx.Prepare("select uid, user_name from (select id as uid, user_name from user as u inner join (select group_id, user_id from member) as m on u.id = m.user_id inner join (select id as gid from groups) as g on g.gid = m.group_id where g.gid = ?) order by uid")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []*user.User{}
	for rows.Next() {
		m := &user.User{}
		err = rows.Scan(&m.ID, &m.Name)
		if err != nil {
			return nil, err
		}

		members = append(members, m)
	}

	return members, rows.Err()
}

// ListGroupMembersEnabledSMB returns the list of member in given group who enabled samba.
func ListGroupMembersEnabledSMB(ctx context.Context, tx *sql.Tx, groupID int) ([]*user.User, error) {
	if err := isGroupExistByID(ctx, tx, groupID); err != nil {
		return nil, errors.Wrapf(err, "while checking group exist: %d", groupID)
	}

	stmt, err := tx.Prepare("select u.user_name, u.home_dir, u.admin from (select id, user_name, home_dir, admin from user) as u inner join (select group_id, user_id from member) as m on u.id = m.user_id inner join (select id from groups) as g on g.id = m.group_id where g.id = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, groupID)
	if err != nil {
		return nil, errors.Wrapf(err, "while select group members for group %d", groupID)
	}
	defer rows.Close()

	members := []*user.User{}
	for rows.Next() {
		admin := 0
		m := &user.User{}
		err = rows.Scan(&m.Name, &m.HomeDir, &admin)
		if err != nil {
			return nil, err
		}
		m.SetAdminFlag(admin)

		if m.IsBotUser() && !m.IsChromecast() {
			members = append(members, m)
		}
	}

	return members, rows.Err()
}

// DeleteMember delete one member from group by its owner.
func DeleteMember(ctx context.Context, tx *sql.Tx, groupID, ownerID, memberID int) error {
	if _, err := user.IsExistByID(ctx, tx, ownerID); err != nil {
		return err
	}

	if _, err := user.IsExistByID(ctx, tx, memberID); err != nil {
		return err
	}

	if err := isGroupOwner(ctx, tx, groupID, ownerID); err != nil {
		return err
	}

	stmt, err := tx.Prepare("delete from member where group_id = ? and user_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, groupID, memberID)
	return err
}

// AddMember adds one member into one group.
func AddMember(ctx context.Context, tx *sql.Tx, groupID, ownerID, memberID int) error {
	if _, err := user.IsExistByID(ctx, tx, ownerID); err != nil {
		return err
	}

	if err := isGroupOwner(ctx, tx, groupID, ownerID); err != nil {
		return err
	}

	return addMember(ctx, tx, groupID, memberID)
}

func addMember(ctx context.Context, tx *sql.Tx, groupID, memberID int) error {
	exist, err := IsUserExist(ctx, tx, groupID, memberID)
	if err != nil {
		return err
	}
	if exist {
		return common.ErrDuplicate
	}
	stmt, err := tx.Prepare("insert into member(group_id, user_id, create_time) values(?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, groupID, memberID, time.Now().UTC())
	return err
}
