package lomocloud

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"

	"github.com/gorilla/mux"
)

// Config is the configuration parameters for the handler.
type Config struct {
	ListenPort  int
	DbFilename  string
	LogDir      string
	DomainName  string
	DeadTimeout time.Duration
}

// Handler is structure to handle http request.
type Handler struct {
	gCtx          context.Context
	conf          *Config
	db            *sql.DB
	dbtrace       *dbx.DBTrace
	df            string // database filename
	accessLogger  *logger.AccessLogger
	accessFile    *logger.RotateFileHook
	domainName    string
	logdir        string
	logFile       *logger.RotateFileHook
	tokenDuration time.Duration
}

// NewHandler construct handler object.
func NewHandler(ctx context.Context, c *Config) (*Handler, error) {
	h := &Handler{
		gCtx:       ctx,
		conf:       c,
		logdir:     c.LogDir,
		df:         c.DbFilename,
		domainName: c.DomainName,
	}

	if err := h.openSqliteDB(); err != nil {
		return nil, err
	}

	if err := h.initLog(); err != nil {
		return nil, err
	}

	return h, nil
}

// Close closes handler.
func (h *Handler) Close() error {
	return nil
}

// CreateRouter associates handler with URL endpoints.
func (h *Handler) CreateRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/", h.readme).Methods(http.MethodGet)

	r.HandleFunc("/login", h.login).Methods(http.MethodPost)

	r.HandleFunc("/account", h.createAccount).Methods(http.MethodPost)
	r.HandleFunc("/account/ip_map", h.getAccountIPMap).Methods(http.MethodGet)
	r.HandleFunc("/account/ip_map", h.createAccountIPMap).Methods(http.MethodPost)
	r.HandleFunc("/account/portmap/{in}/{out}", h.createAccountPortMap).Methods(http.MethodPost)
	r.HandleFunc("/account/portmap/{in}/{out}", h.updateAccountPortMap).Methods(http.MethodPut)

	r.Use(h.accessLogger.Middleware)
	return r
}

// CreateRouterHTTP associates handler with 80 port for mobile client facing.
func (h *Handler) CreateRouterHTTP() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/", h.redirect).Methods(http.MethodGet)
	r.Use(h.accessLogger.Middleware)

	return r
}

func (h *Handler) readme(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`
<!DOCTYPE html>
<html>
  <head>
	  <title>https://lomorage.com/</title>
		<link rel="canonical" href="https://lomorage.com/"/>
		<meta name="robots" content="noindex">
		<meta charset="utf-8" />
		<meta http-equiv="refresh" content="0; url=https://lomorage.com/" />
	</head>
</html>
`))
}

// CORS is the header response to an OPTIONS request.
func (h *Handler) CORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", "PUT,GET,POST,HEAD,PATCH")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
}

func (h *Handler) isMaintenance(p string) bool {
	return false
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request) {
}
