package handler

/*
func (h *Handler) initCert(rootCACert, rootCAKey string) error {
	if rootCACert == "" || rootCAKey == "" {
		if err := h.cert.GenerateCert(true); err != nil {
			return err
		}
		certBuffer := &bytes.Buffer{}
		if err := h.cert.WriteCert(certBuffer); err != nil {
			return err
		}
		keyBuffer := &bytes.Buffer{}
		if err := h.cert.WriteKey(keyBuffer); err != nil {
			return err
		}
		return dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			if err := conf.SetConfValue(ctx, tx, common.ConfRootCACert, certBuffer.String()); err != nil {
				return err
			}
			return conf.SetConfValue(ctx, tx, common.ConfRootCAKey, keyBuffer.String())
		})
	}
	var err error
	h.cert, err = cert.ParseCert([]byte(rootCACert), nil)
	if err != nil {
		return err
	}

	return h.cert.ParseKey([]byte(rootCAKey))
}

func (h *Handler) loadCert() error {
	var rootCACert, rootCAKey string
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		err := h.loadUsername(ctx, tx)
		if err != nil {
			return err
		}
		rootCACert, err = conf.GetConfValue(ctx, tx, common.ConfRootCACert)
		if err != nil {
			return err
		}
		rootCAKey, err = conf.GetConfValue(ctx, tx, common.ConfRootCAKey)
		return err
	}); err != nil {
		if !common.IsErrNoRows(err) {
			return err
		}
		return h.initCert("", "")
	}
	return h.initCert(rootCACert, rootCAKey)
}
*/
