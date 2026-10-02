package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/conf"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/jwt"
	. "gopkg.in/check.v1"
)

// Integration tests for the LibrePhotos-style JWT endpoints: tokens are
// signed with a per-install random key, not a key shipped in the source.

func (ts *mainSuite) jwtSecret(c *C) string {
	var secret string
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		secret, err = conf.GetConfValue(ctx, tx, common.ConfJWTSecret)
		return err
	}), IsNil)
	return secret
}

func (ts *mainSuite) obtainJWT(c *C) obtainTokenReply {
	body, err := json.Marshal(obtainTokenRequest{Username: "alice", Password: "alice123"})
	c.Assert(err, IsNil)
	res, err := ts.request("/api/auth/token/obtain/", http.MethodPost, http.StatusOK, bytes.NewReader(body), nil)
	c.Assert(err, IsNil)
	defer res.Close()
	reply := obtainTokenReply{}
	c.Assert(json.NewDecoder(res).Decode(&reply), IsNil)
	c.Assert(reply.Access, Not(Equals), "")
	c.Assert(reply.Refresh, Not(Equals), "")
	return reply
}

func (ts *mainSuite) statusWithBearer(jwtToken string) int {
	req := httptest.NewRequest(http.MethodGet, "/assets/scan/status", nil)
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	return rr.Code
}

func (ts *mainSuite) TestJWTSecretIsPerInstallAndStable(c *C) {
	secret := ts.jwtSecret(c)
	c.Assert(len(secret), Equals, 64)
	c.Assert(secret, Not(Equals), "ThisIsMySuperSecret")

	// reloading conf (lomod restart) must keep the key, or every client gets logged out
	c.Assert(ts.h.loadConf(), IsNil)
	c.Assert(ts.jwtSecret(c), Equals, secret)
}

func (ts *mainSuite) TestJWTObtainUseAndRefresh(c *C) {
	reply := ts.obtainJWT(c)
	c.Assert(ts.statusWithBearer(reply.Access), Equals, http.StatusOK)

	body, err := json.Marshal(refreshTokenRequest{Refresh: reply.Refresh})
	c.Assert(err, IsNil)
	res, err := ts.request("/api/auth/token/refresh/", http.MethodPost, http.StatusOK, bytes.NewReader(body), nil)
	c.Assert(err, IsNil)
	defer res.Close()
	refreshed := obtainTokenReply{}
	c.Assert(json.NewDecoder(res).Decode(&refreshed), IsNil)
	c.Assert(ts.statusWithBearer(refreshed.Access), Equals, http.StatusOK)
}

func (ts *mainSuite) TestJWTSignedWithOldSourceKeyRejected(c *C) {
	// a valid login token, wrapped in a JWT signed with the key that used to be
	// hardcoded in the source: only the signature differs from a real one
	claims := jwt.NewClaim()
	claims.Set("token_type", "access")
	claims.SetTime("exp", time.Now().Add(time.Hour))
	claims.Set("jti", ts.token)
	claims.Set("user_id", 1)
	claims.Set("name", "alice")
	oldKey := jwt.HmacSha256("ThisIsMySuperSecret")
	forged, err := oldKey.Encode(claims)
	c.Assert(err, IsNil)
	c.Assert(ts.statusWithBearer(forged), Equals, http.StatusUnauthorized)

	body, err := json.Marshal(refreshTokenRequest{Refresh: forged})
	c.Assert(err, IsNil)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/token/refresh/", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	c.Assert(rr.Code, Not(Equals), http.StatusOK)
}

func (ts *mainSuite) TestJWTExpiredOrTamperedRejected(c *C) {
	claims := jwt.NewClaim()
	claims.Set("token_type", "access")
	claims.SetTime("exp", time.Now().Add(-time.Minute))
	claims.Set("jti", ts.token)
	claims.Set("user_id", 1)
	key := jwt.HmacSha256(ts.jwtSecret(c))
	expired, err := key.Encode(claims)
	c.Assert(err, IsNil)
	c.Assert(ts.statusWithBearer(expired), Equals, http.StatusUnauthorized)

	// swap the payload of a real token for one carrying another session's jti
	real := strings.Split(ts.obtainJWT(c).Access, ".")
	c.Assert(real, HasLen, 3)
	claims.SetTime("exp", time.Now().Add(time.Hour))
	claims.Set("jti", "not-the-signed-token")
	other, err := key.Encode(claims)
	c.Assert(err, IsNil)
	real[1] = strings.Split(other, ".")[1]
	c.Assert(ts.statusWithBearer(strings.Join(real, ".")), Equals, http.StatusUnauthorized)
}
