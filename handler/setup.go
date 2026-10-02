package handler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
	qrcode "github.com/skip2/go-qrcode"
)

// setupLinkBase is the homepage page the setup QR code points to. A phone
// with LomoMobile opens the link straight in the app (Universal Links / App
// Links, declared in the homepage's /.well-known/); a phone without it lands
// on that page, which asks the user to install the app first. The parameters
// go in the fragment so the LAN address never reaches the web host.
//
// The link intentionally carries no credentials: unlike the device-to-device
// pairing QR (see PAIRING_QR_TYPE in lomo-mobile), account creation on a
// fresh instance is open to whoever can reach it (see createUser), so this
// code only needs to identify the server.
//
// Servers before this change encoded {"type":"lomorage-setup-v1",...} JSON
// instead, which a phone's camera app can't do anything with. LomoMobile
// still accepts that form, so older servers keep working.
const setupLinkBase = "https://lomorage.com/s/"

type setupQRPayload struct {
	Server     string
	UUID       string
	ServerName string
}

func (h *Handler) hasAnyUser(ctx context.Context, tx *sql.Tx) (bool, error) {
	count, err := user.Counts(ctx, tx, false)
	if err != nil {
		return false, err
	}
	return count != 0, nil
}

// setupServerAddress returns the address a phone on the same network should
// use to reach this instance, preferring the advertised LAN IP over the
// request's Host header since the latter is often "localhost".
func (h *Handler) setupServerAddress() string {
	if len(h.listenIPs) > 0 {
		return fmt.Sprintf("%s:%d", h.listenIPs[0].String(), h.conf.ListenPort)
	}
	return fmt.Sprintf("localhost:%d", h.conf.ListenPort)
}

func (h *Handler) buildSetupQRPayload() setupQRPayload {
	return setupQRPayload{
		Server:     h.setupServerAddress(),
		UUID:       h.uuid,
		ServerName: h.conf.MdnsName,
	}
}

// welcomePageHandler serves the first-run setup page carrying the setup QR
// code. It only makes sense before any account exists; if one was created
// since the page was linked to (e.g. stale bookmark), send the visitor to
// the normal login form instead.
func (h *Handler) welcomePageHandler(w http.ResponseWriter, r *http.Request) {
	h.changePreferedLanguage(r)

	var hasUser bool
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		hasUser, err = h.hasAnyUser(ctx, tx)
		return err
	})
	if err != nil {
		logrus.Warnf("welcome page: %v", err)
	}
	if hasUser {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	body, err := h.loadTemplateFile("welcome.html")
	if err != nil {
		common.WriteError(w, err)
		return
	}
	io.WriteString(w, body)
}

// setupQRContent returns the setup link encoded in the welcome page's QR code.
func (h *Handler) setupQRContent() string {
	payload := h.buildSetupQRPayload()
	params := url.Values{}
	params.Set("server", payload.Server)
	params.Set("uuid", payload.UUID)
	if payload.ServerName != "" {
		params.Set("name", payload.ServerName)
	}
	return setupLinkBase + "#" + params.Encode()
}

func (h *Handler) welcomeQRCodeHandler(w http.ResponseWriter, r *http.Request) {
	png, err := qrcode.Encode(h.setupQRContent(), qrcode.Medium, 256)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

func (h *Handler) welcomeInfoHandler(w http.ResponseWriter, r *http.Request) {
	payload := h.buildSetupQRPayload()
	common.WriteBody(w, struct {
		Server     string
		ServerName string
	}{Server: payload.Server, ServerName: payload.ServerName})
}
