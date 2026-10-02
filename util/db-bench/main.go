package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	_ "github.com/mattn/go-sqlite3"
)

const dbFile = "../../v2/assets.db"

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func main() {
	if len(os.Args) != 5 {
		log.Fatalf("wrong number of arguments. Should be db-bench <# users> <# groups> <# extensions> <# assets>")
	}

	numUsers, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatalf(err.Error())
	}
	numGroups, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatalf(err.Error())
	}
	numExts, err := strconv.Atoi(os.Args[3])
	if err != nil {
		log.Fatalf(err.Error())
	}
	numAssets, err := strconv.Atoi(os.Args[4])
	if err != nil {
		log.Fatalf(err.Error())
	}
	fmt.Printf("start creating %d users, %d groups, %d extensions, %d assets\n", numUsers, numGroups, numExts, numAssets)

	process(numUsers, numExts, numAssets)
}

func createUsers(ctx context.Context, tx *sql.Tx, numUsers int) ([]int, error) {
	fmt.Println("start creating users")
	userids := []int{}
	for i := 0; i < numUsers; i++ {
		username := fmt.Sprintf("user%d", i)
		stmt, err := tx.Prepare("insert into user(user_name, password, create_time, last_modified_time) values(?, ?, ?, ?)")
		if err != nil {
			return nil, err
		}
		defer stmt.Close()

		result, err := stmt.ExecContext(ctx, username, username, time.Now(), time.Now())
		if err != nil {
			return nil, err
		}
		userid, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}
		userids = append(userids, int(userid))
	}
	return userids, nil
}

func createDevices(ctx context.Context, tx *sql.Tx, userids []int) ([]int, error) {
	fmt.Println("start creating devices")
	deviceids := []int{}
	for i, u := range userids {
		devicename := fmt.Sprintf("device%d", i)
		stmt, err := tx.Prepare("insert into device(user_id, device_name) values(?, ?)")
		if err != nil {
			return nil, err
		}
		defer stmt.Close()

		result, err := stmt.ExecContext(ctx, u, devicename)
		if err != nil {
			return nil, err
		}
		deviceid, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}
		deviceids = append(deviceids, int(deviceid))
	}

	return deviceids, nil
}

func createTokens(ctx context.Context, tx *sql.Tx, userids, deviceids []int) error {
	fmt.Println("start creating tokens")
	for i := range userids {
		token := fmt.Sprintf("token%d", i)
		stmt, err := tx.Prepare("insert into token(token, user_id, device_id, create_time, expire_time) values(?, ?, ?, ?, ?)")
		if err != nil {
			return err
		}
		defer stmt.Close()

		_, err = stmt.ExecContext(ctx, token, userids[i], deviceids[i], time.Now(), time.Now().Add(time.Hour))
		if err != nil {
			return err
		}
	}
	return nil
}

func process(numUsers, numExts, numAssets int) {
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer tx.Rollback()

	ctx, cancel := context.WithTimeout(context.Background(), common.DBTimeOut)
	defer cancel()

	userids, err := createUsers(ctx, tx, numUsers)
	if err != nil {
		log.Fatalf(err.Error())
	}

	deviceids, err := createDevices(ctx, tx, userids)
	if err != nil {
		log.Fatalf(err.Error())
	}

	if err := createTokens(ctx, tx, userids, deviceids); err != nil {
		log.Fatalf(err.Error())
	}

	fmt.Println("start creating exts")
	extids := []int{}
	for i := 0; i < numExts; i++ {
		ext := fmt.Sprintf("ext%d", i)
		stmt, err := tx.Prepare("insert into ext(ext) values(?)")
		if err != nil {
			log.Fatalf(err.Error())
		}
		defer stmt.Close()

		result, err := stmt.ExecContext(ctx, ext)
		if err != nil {
			log.Fatalf(err.Error())
		}
		extid, err := result.LastInsertId()
		if err != nil {
			log.Fatalf(err.Error())
		}
		extids = append(extids, int(extid))
	}

	fmt.Println("start creating assets")
	//useridxs := []int{}
	for i := 0; i < numAssets; i++ {
		useridx := rand.Intn(numUsers)
		//useridxs = append(useridxs, useridx)
		userid := userids[useridx]
		deviceid := deviceids[useridx]

		extid := extids[rand.Intn(numExts)]
		y := 2000 + rand.Intn(18)
		m := rand.Intn(12) + 1
		d := rand.Intn(25) + 1

		c := ""
		for j := 0; j <= i; j++ {
			c = fmt.Sprintf("%s%d", c, j)
		}

		stmt, err := tx.Prepare("insert into asset(user_id, year, month, day, ext_id, hash, device_id, create_time, upload_time) values(?, ?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			log.Fatalf(err.Error())
		}
		defer stmt.Close()

		_, err = stmt.ExecContext(ctx, userid, y, m, d, extid, randStringRunes(32), deviceid, time.Now(), time.Now())
		if err != nil {
			log.Fatalf(err.Error())
		}
	}

	// start benchmark
	fmt.Println("get /category")
	fmt.Println("get /category/y")
	fmt.Println("get /category/y/m")
	fmt.Println("get /category/y/m/d")

	fmt.Println("user share to each other")
	fmt.Println("user share to each other")
	fmt.Println("user receives share")
}
