package dbx

// DummyLogger do nothing for db
type DummyLogger struct {
}

// LogCallback is log callback function for db trace
func (dl *DummyLogger) LogCallback(runTimeText, dbErrText, msg string, isTxn bool) {
}
