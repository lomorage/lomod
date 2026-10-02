package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/benbjohnson/litestream"
	"github.com/benbjohnson/litestream/file"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type replicateUser struct {
	add   bool
	users map[string]string // username, homedir pai
}

func (h *Handler) replicateLoop(ctx context.Context, dsn string, syncInterval time.Duration) {
	h.replicaCh = make(chan replicateUser, 1)
	h.restoreCh = make(chan string)
	h.restoreErrCh = make(chan error)

	// disable the log otherwise too many data
	//litestream.Tracef = logrus.Infof
	lsdb := litestream.NewDB(dsn)
	defer lsdb.SoftClose()

	isOpened := false
	for {
		if !isOpened {
			err := lsdb.Open()
			if err != nil {
				logrus.Warnf("open replication: %s", err)
				time.Sleep(time.Minute)
				continue
			} else {
				isOpened = true
			}
		}

		users := map[string]string{}
		err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			//return home dir if request is from localhost
			us, err := user.ListUsers(ctx, tx)
			if err != nil {
				return err
			}
			for _, u := range us.Users {
				if u.IsBotUser() {
					continue
				}
				users[u.Name] = u.HomeDir
			}
			return nil
		})
		if err != nil {
			logrus.Warnf("list user directories for replication: %s", err)
			time.Sleep(time.Minute)
			continue
		}
		if len(users) != 0 {
			h.replicaCh <- replicateUser{add: true, users: users}
		}
		break
	}

	for {
	loop:
		// sync every 5 second
		after := time.After(syncInterval)
		// loop current replica names to avoid duplicate
		var newReplicas []*litestream.Replica
		select {
		case <-ctx.Done():
			if ctx.Err() != nil {
				logrus.Warnf("replication is closed: %s", ctx.Err())
			}
			return
		case <-after:
			newReplicas = lsdb.Replicas
		case username := <-h.restoreCh:
			found := false
			for _, r := range lsdb.Replicas {
				if r.Name() != username {
					continue
				}
				logrus.Infof("use %s's backup DB for restore", r.Name())
				err := h.restoreDBFromReplica(ctx, r)
				if err != nil {
					logrus.Warnf("fail to restore DB: %s", err)
				} else {
					logrus.Info("restore complete")
				}
				h.restoreErrCh <- err
				found = true
				break
			}
			if !found {
				logrus.Infof("not found %s to backup from", username)
				h.restoreErrCh <- common.ErrInvalidUser
			}
			goto loop
		case n := <-h.replicaCh:
			newReplicas = []*litestream.Replica{}
			for _, r := range lsdb.Replicas {
				_, exist := n.users[r.Name()]
				if exist {
					if n.add {
						logrus.Infof("%s is in replica list already, skip", r.Name())
					} else {
						logrus.Infof("remove %s from replica list", r.Name())
					}
					delete(n.users, r.Name())
				} else {
					newReplicas = append(newReplicas, r)
				}
			}
			if n.add {
				for username, homedir := range n.users {
					client := file.NewReplicaClient(common.GetUserBackupDBDir(homedir))
					r := litestream.NewReplica(lsdb, username)
					r.Client = client
					newReplicas = append(newReplicas, r)
				}
			} else if len(n.users) != 0 {
				logrus.Warnf("%v is not removed from replica list", n.users)
			}
		}

		lsdb.Replicas = newReplicas
		if err := lsdb.Sync(ctx); err != nil {
			logrus.Warnf("sync main db: %s", err)
			continue
		}
		for _, r := range lsdb.Replicas {
			if err := r.Sync(ctx); err != nil {
				logrus.Warnf("sync replicate %s: %s", r.Name(), err)
				continue
			}
		}
	}
}

func (h *Handler) restoreDBFromReplica(ctx context.Context, replica *litestream.Replica) (err error) {
	// close db and move to backup file
	err = h.db.Close()
	if err != nil {
		logrus.Warnf("close main DB while restoring: %s", err)
	}
	dbFilename := replica.DB().Path()
	bn := fmt.Sprintf("%s.%s", dbFilename, time.Now().Format("01-02-06.15.04.05"))
	err = os.Rename(replica.DB().Path(), bn)
	if err != nil {
		logrus.Warnf("backup main DB to %s: %s", bn, err)
	}

	// always reopen the db
	defer func() {
		if err != nil {
			logrus.Warnf("recover original db due to restore failure: %v", err)
			copyErr := common.CopyFile(bn, replica.DB().Path())
			if copyErr != nil {
				logrus.Warnf("recover original db failure: %v", copyErr)
			}
		}
		openErr := h.openSqliteDB()
		if openErr != nil {
			if err == nil {
				err = openErr
			} else {
				logrus.Warnf("reopen original db fail due to restore failure: %v", openErr)
			}
		} else {
			h.accessLogger.DB = h.db
		}
	}()

	// Configure restore to write out to DSN path.
	opt := litestream.NewRestoreOptions()
	opt.OutputPath = replica.DB().Path()
	opt.Logger = log.New(logrus.StandardLogger().Writer(), "", log.LstdFlags|log.Lmicroseconds)

	// Determine the latest generation to restore from.
	if opt.Generation, _, err = replica.CalcRestoreTarget(ctx, opt); err != nil {
		return err
	}

	// Only restore if there is a generation available on the replica.
	// Otherwise we'll let the application create a new database.
	if opt.Generation == "" {
		logrus.Warn("no generation found, creating new database")
		return nil
	}

	logrus.Infof("restoring replica for generation %s\n", opt.Generation)
	return replica.Restore(ctx, opt)
}

func (h *Handler) restoreDBBackup(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	username := mux.Vars(r)["userName"]
	if username == "" {
		username = wl.Username
	}

	h.restoreCh <- username
	if err := <-h.restoreErrCh; err != nil {
		common.WriteError(w, err)
	}
}
