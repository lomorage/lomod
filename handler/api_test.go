package handler

import (
	"net/http"
)

func (ts *mainSuite) restore(username string) error {
	reply, err := ts.request("/system/restore/db/"+username+"?token="+ts.token, http.MethodPost, 200, nil, nil)
	if err != nil {
		return err
	}
	return reply.Close()
}
