package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/ext"
	_ "github.com/mattn/go-sqlite3"
)

type userv1 struct {
	userid           string
	password         string
	phone            sql.NullString
	email            sql.NullString
	nickname         sql.NullString
	homedir          string
	admin            sql.NullInt64
	status           sql.NullInt64
	createtime       string
	lastmodifiedtime string
	lastlogintime    string
}

type assetv1 struct {
	assetid    int
	userid     string
	path       string
	ext        string
	hash       string
	device     string
	createtime string
	uploadtime string
}

type tokenv1 struct {
	token      string
	userid     string
	device     string
	createtime string
	expiretime string
}

func loadTokenv1(ctx context.Context, tx *sql.Tx) ([]tokenv1, error) {
	stmt, err := tx.Prepare("select token, userid, device, createtime, expiretime from token")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	t1 := []tokenv1{}
	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		t := tokenv1{}
		err := rows.Scan(&t.token, &t.userid, &t.device, &t.createtime, &t.expiretime)
		if err != nil {
			fmt.Println("----")
			return nil, err
		}
		t1 = append(t1, t)
	}
	return t1, rows.Err()
}

func loadUserv1(ctx context.Context, tx *sql.Tx) ([]userv1, error) {
	stmt, err := tx.Prepare("select userid, password, phone, email, nickname, homedir, admin, status, createtime, lastmodifiedtime, lastlogintime from user")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	u1 := []userv1{}
	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		u := userv1{}
		err := rows.Scan(&u.userid, &u.password, &u.phone, &u.email, &u.nickname, &u.homedir, &u.admin, &u.status, &u.createtime, &u.lastmodifiedtime, &u.lastlogintime)
		if err != nil {
			return nil, err
		}
		u1 = append(u1, u)
	}
	return u1, rows.Err()
}

func loadAssetv1(ctx context.Context, tx *sql.Tx) (int, []assetv1, error) {
	stmt, err := tx.Prepare("select assetid, userid, path, ext, hash, device, createtime, uploadtime from asset")
	if err != nil {
		return 0, nil, err
	}
	defer stmt.Close()

	a1 := []assetv1{}
	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	max := 0
	for rows.Next() {
		a := assetv1{}
		err := rows.Scan(&a.assetid, &a.userid, &a.path, &a.ext, &a.hash, &a.device, &a.createtime, &a.uploadtime)
		if err != nil {
			return 0, nil, err
		}
		a1 = append(a1, a)

		if a.assetid > max {
			max = a.assetid
		}
	}
	return max, a1, rows.Err()
}

func insertUserv2(ctx context.Context, tx *sql.Tx, u []userv1) (map[string]int, error) {
	idMap := map[string]int{}
	for _, user := range u {
		stmt, err := tx.Prepare("insert into user (user_name, password, phone, email, nick_name, home_dir, admin, status, create_time, last_modified_time, last_login_time) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			return idMap, err
		}
		result, err := stmt.ExecContext(ctx, user.userid, user.password, "", "", "", user.homedir, 0, 0, user.createtime, user.lastmodifiedtime, user.lastlogintime)
		if err != nil {
			return idMap, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return idMap, err
		}
		idMap[user.userid] = int(id)
		if err := stmt.Close(); err != nil {
			return idMap, err
		}
	}
	return idMap, nil
}

func insertTokenv2(ctx context.Context, tx *sql.Tx, t []tokenv1, userMap map[string]int) (map[string]int, error) {
	idMap := map[string]int{}

	for _, token := range t {
		var id int64
		{
			stmt, err := tx.Prepare("insert into device (user_id, device_name) values (?, ?)")
			if err != nil {
				return idMap, err
			}
			result, err := stmt.ExecContext(ctx, userMap[token.userid], token.device)
			if err != nil {
				return idMap, err
			}
			id, err = result.LastInsertId()
			if err != nil {
				return idMap, err
			}
			idMap[token.userid] = int(id)
			if err := stmt.Close(); err != nil {
				return idMap, err
			}
		}
		{
			stmt, err := tx.Prepare("insert into token (token, user_id, device_id, create_time, expire_time) values (?, ?, ?, ?, ?)")
			if err != nil {
				return idMap, err
			}
			_, err = stmt.ExecContext(ctx, token.token, userMap[token.userid], id, token.createtime, token.expiretime)
			if err != nil {
				return idMap, err
			}
			if err := stmt.Close(); err != nil {
				return idMap, err
			}
		}
	}

	return idMap, nil
}

func preCreateAssets(ctx context.Context, tx *sql.Tx, maxassetID int) error {
	for i := 0; i < maxassetID; i++ {
		stmt, err := tx.Prepare("insert into asset (user_id, year, month, day, ext_id, hash, device_id, create_time, upload_time) values (?, ?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			return err
		}
		_, err = stmt.ExecContext(ctx, "", "", "", "", "", "", "", "", "")
		if err != nil {
			return err
		}
		if err := stmt.Close(); err != nil {
			return err
		}
	}
	return nil
}

func insertAssetv2(ctx context.Context, tx *sql.Tx, max int, a []assetv1, userMap, deviceMap map[string]int) error {
	exist := make([]bool, max)

	for _, asset := range a {
		exist[asset.assetid-1] = true

		stmt, err := tx.Prepare("update asset set user_id = ?, year = ?, month = ?, day = ?, ext_id = ?, hash = ?, device_id = ?, create_time = ?, upload_time = ? where id = ?")
		if err != nil {
			return err
		}
		parts := strings.Split(asset.path, "/")
		y, err := strconv.Atoi(parts[0])
		if err != nil {
			return err
		}
		m, err := strconv.Atoi(parts[1])
		if err != nil {
			return err
		}
		d, err := strconv.Atoi(parts[2])
		if err != nil {
			return err
		}
		extID, err := ext.GetExtID(asset.ext)
		if err != nil {
			return err
		}

		t, err := time.Parse("2006-01-02T15:04:05Z", asset.createtime)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, userMap[asset.userid], y, m, d, extID, asset.hash, deviceMap[asset.userid], t.Format("2006-01-02 15:04:05.000-07:00"), asset.uploadtime, asset.assetid)
		if err != nil {
			return err
		}
		if err := stmt.Close(); err != nil {
			return err
		}
	}

	// remove non-exist ones
	for i := 0; i < max; i++ {
		if exist[i] {
			continue
		}
		fmt.Printf("Delete %d\n", i+1)
		stmt, err := tx.Prepare("delete from asset where id = ?")
		if err != nil {
			return err
		}
		_, err = stmt.ExecContext(ctx, i+1)
		if err != nil {
			return err
		}
		if err := stmt.Close(); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("%s <v1 db file> <v2 db file>", os.Args[0])
	}

	dbv1, err := sql.Open("sqlite3", os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer dbv1.Close()

	dbv2, err := sql.Open("sqlite3", os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer dbv2.Close()

	tx1, err := dbv1.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx1.Rollback()

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	t1, err := loadTokenv1(ctx, tx1)
	if err != nil {
		log.Fatal(err)
	}
	u1, err := loadUserv1(ctx, tx1)
	if err != nil {
		log.Fatal(err)
	}
	max, a1, err := loadAssetv1(ctx, tx1)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("max asset id is %d\n", max)

	//fmt.Println(t1)
	//fmt.Println(u1)
	//fmt.Println(a1)

	// insert user
	tx2, err := dbv2.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx2.Rollback()

	userMap, err := insertUserv2(ctx, tx2, u1)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(userMap)

	deviceMap, err := insertTokenv2(ctx, tx2, t1, userMap)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(deviceMap)

	if err := preCreateAssets(ctx, tx2, max); err != nil {
		log.Fatal(err)
	}

	if err := insertAssetv2(ctx, tx2, max, a1, userMap, deviceMap); err != nil {
		log.Fatal(err)
	}

	if err := tx2.Commit(); err != nil {
		log.Fatal(err)
	}
}
