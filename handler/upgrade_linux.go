package handler

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/exec"
	"syscall"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
)

func (h *Handler) upgrade(w http.ResponseWriter, r *http.Request) {
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		count, err := user.Counts(ctx, tx, false)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		return w.(*logger.ResponseLogger).Err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	logrus.Info("update apt repo information")
	_, err := cmd.RunWithSudo("apt", "update", "-y")
	if err != nil {
		common.WriteError(w, err)
		return
	}

	logrus.Info("update lomod-backend package")
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		common.WriteError(w, err)
		return
	}
	if err := syscall.Exec(sudo, []string{sudo, "apt-get", "install", "-y", "--only-upgrade", "lomo-backend"},
		os.Environ()); err != nil {
		common.WriteError(w, err)
	}
}
