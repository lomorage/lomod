package common

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

var (
	regexNoTable     = regexp.MustCompile("no such table")
	regexConnRefused = regexp.MustCompile("connection refused")
	regexNotReach    = regexp.MustCompile("network is unreachable")
)

var (
	// ErrUnknown is system related error
	ErrUnknown = errors.New("Unknown Error")
	// ErrBadRequest is bad request
	ErrBadRequest = errors.New("Bad Request")
	// ErrDuplicate means request resources exist
	ErrDuplicate = errors.New("Requested resource Exist")
	// ErrInvalidPasswd means invalid username/password
	ErrInvalidPasswd = errors.New("Invalid Username/Password")
	// ErrInvalidToken means invalid token
	ErrInvalidToken = errors.New("Invalid Token")
	// ErrInvalidMonth means invalid request year argument
	ErrInvalidMonth = errors.New("Invalid Month")
	// ErrInvalidDay means invalid request day argument
	ErrInvalidDay = errors.New("Invalid Day")
	// ErrDifferentUsername means different username and owner name
	ErrDifferentUsername = errors.New("Different username and owner name")
	// ErrNoTimestamp means no timestam
	ErrNoTimestamp = errors.New("No timestamp")
	// ErrNotExistUser means user not exist
	ErrNotExistUser = errors.New("User is not exist")
	// ErrNotExistTokenDevice means not exist device for the token
	ErrNotExistTokenDevice = errors.New("Device for the token is not exist")
	// ErrEmptyGroupName means empty group name
	ErrEmptyGroupName = errors.New("Empty groupname")
	// ErrEmptyGroupOwner means empty group owner
	ErrEmptyGroupOwner = errors.New("Empty owner")
	// ErrNotInGroup means not belong to this group
	ErrNotInGroup = errors.New("Not belong to this group")
	// ErrDeviceNotMount means device is not mounted yet
	ErrDeviceNotMount = errors.New("Device is not mounted yet")
	// ErrDeviceNotLocate means unable to locate device uuid
	ErrDeviceNotLocate = errors.New("Unable to locate device UUID")
	// ErrNotImplementedFormat means not implemented format
	ErrNotImplementedFormat = errors.New("Not implemented")
	// ErrEmptyAsset means empty asset
	ErrEmptyAsset = errors.New("Empty asset")
	// ErrGroupExist means group already exist
	ErrGroupExist = errors.New("Group already exist")
	// ErrAssetNotSharedToUser means asset not shared to the user
	ErrAssetNotSharedToUser = errors.New("Asset not shared to the user")
	// ErrAssetNotExistForUser means asset not exist for user
	ErrAssetNotExistForUser = errors.New("Asset not exist for the user")
	// ErrInvalidDir means user home dir is not valid
	ErrInvalidDir = errors.New("User's directory is not absolute path")
	// ErrNotExistAsset means asset is not exist
	ErrNotExistAsset = errors.New("Asset is not exist")
	// ErrNotExistMaster means original asset is not exist
	ErrNotExistMaster = errors.New("Original asset is not exist")
	// ErrWrongAssetCache means wrong asset cache in memdb
	ErrWrongAssetCache = errors.New("Wrong asset cache")
	// ErrAssetDiffHash means uploaded asset hash is different from request
	ErrAssetDiffHash = errors.New("Uploaded asset has different hash")
	// ErrInternalAPI means the api can only visited internally
	ErrInternalAPI = errors.New("API can only be visited internally")
	// ErrMaintenance means the service is in maintenance
	ErrMaintenance = errors.New("Service is in maintenance mode")
	// ErrAssetLomoOriginSHANotFound means QuickTime:ComLomorageOriginhash tag not found
	ErrAssetLomoOriginSHANotFound = errors.New("QuickTime:ComLomorageOriginhash tag not found")
	// ErrEmptyUsername means empty user name during api request
	ErrEmptyUsername = errors.New("Empty user")
	// ErrInvalidAdminToken means invalid token
	ErrInvalidAdminToken = errors.New("Invalid admin token")
	// ErrInvalidName means invalid name, such as user name, nick name, etc
	ErrInvalidName = errors.New("Invalid name")
	// ErrInvalidUser means invalid user to be operated
	ErrInvalidUser = errors.New("Invalid user")
	// ErrRetryFailure means retry attempt also fail
	ErrRetryFailure = errors.New("Retry attempt failure")
	// ErrScanInProgress means scan not finished
	ErrScanInProgress = errors.New("Scan in progress")
	// ErrNotFound means resource not found
	ErrNotFound = errors.New("Resource not found")
	// ErrNoCreateTime means unable to find create time
	ErrNoCreateTime = errors.New("No create time")
	// ErrInvalidMntSBC means invalid mount directory at SBC board
	ErrInvalidMntSBC = errors.New("Invalid Disk Mount")
	// ErrMkdir means failure to create directory
	ErrMkdir = errors.New("Fail to mkdir")
	// ErrCreateFile means failure to create file
	ErrCreateFile = errors.New("Fail to create file")
	// ErrTryLater means resource is still using, and try later
	ErrTryLater = errors.New("try later")
	// ErrFailUpdate means resource update operation fail
	ErrFailUpdate = errors.New("Fail to update")
	// ErrGeoNoCountry means no country name in geo metadata process
	ErrGeoNoCountry = errors.New("Empty country data during geo name normalization")
	// ErrGeoNoState means no state name in geo metadata process
	ErrGeoNoState = errors.New("Empty state data during geo name normalization")
	// ErrInvalidID means invalid request ID
	ErrInvalidID = errors.New("Invalid ID")
	// ErrPreviewBusy means the preview-generation queue is already full of
	// waiting requests -- returned immediately instead of accepting the
	// request and blocking its goroutine indefinitely, which on
	// memory-constrained hardware lets a request burst (e.g. a mobile
	// gallery grid loading/scrolling quickly) pile up enough blocked
	// goroutines to exhaust RAM even though only a few are actually
	// generating previews at once.
	ErrPreviewBusy = errors.New("Too many preview requests, try again shortly")
)

type errStatus struct {
	err    error
	status int
}

type errType struct {
	id     int
	status int
}

var errMaps map[error]errType

func init() {
	errMaps = map[error]errType{}
	for id, err := range []errStatus{
		{ErrUnknown, http.StatusBadRequest},
		{ErrBadRequest, http.StatusBadRequest},
		{ErrDuplicate, http.StatusBadRequest},
		{ErrInvalidPasswd, http.StatusUnauthorized},
		{ErrInvalidToken, http.StatusUnauthorized},
		{ErrInvalidMonth, http.StatusBadRequest},
		{ErrInvalidDay, http.StatusBadRequest},
		{ErrDifferentUsername, http.StatusBadRequest},
		{ErrNoTimestamp, http.StatusBadRequest},
		{ErrNotExistUser, http.StatusNotFound},
		{ErrNotExistTokenDevice, http.StatusNotFound},
		{ErrEmptyGroupName, http.StatusBadRequest},
		{ErrEmptyGroupOwner, http.StatusBadRequest},
		{ErrNotInGroup, http.StatusBadRequest},
		{ErrDeviceNotMount, http.StatusInternalServerError},
		{ErrDeviceNotLocate, http.StatusInsufficientStorage},
		{ErrNotImplementedFormat, http.StatusBadRequest},
		{ErrEmptyAsset, http.StatusBadRequest},
		{ErrGroupExist, http.StatusBadRequest},
		{ErrAssetNotSharedToUser, http.StatusNotFound},
		{ErrAssetNotExistForUser, http.StatusNotFound},
		{ErrInvalidDir, http.StatusBadRequest},
		{ErrNotExistAsset, http.StatusNotFound},
		{ErrWrongAssetCache, http.StatusInternalServerError},
		{ErrAssetDiffHash, http.StatusBadRequest},
		{ErrInternalAPI, http.StatusBadRequest},
		{ErrMaintenance, http.StatusServiceUnavailable},
		{ErrAssetLomoOriginSHANotFound, http.StatusBadRequest},
		{ErrEmptyUsername, http.StatusBadRequest},
		{ErrInvalidAdminToken, http.StatusUnauthorized},
		{ErrInvalidName, http.StatusBadRequest},
		{ErrInvalidUser, http.StatusBadRequest},
		{ErrRetryFailure, http.StatusInternalServerError},
		{ErrScanInProgress, http.StatusServiceUnavailable},
		{ErrNotFound, http.StatusNotFound},
		{ErrNoCreateTime, http.StatusBadRequest},
		{ErrInvalidMntSBC, http.StatusInternalServerError},
		{ErrMkdir, http.StatusInternalServerError},
		{ErrCreateFile, http.StatusInternalServerError},
		{ErrTryLater, http.StatusInternalServerError},
		{ErrFailUpdate, http.StatusBadRequest},
		{ErrNotExistMaster, http.StatusNotFound},
		{ErrInvalidID, http.StatusBadRequest},
		{ErrPreviewBusy, http.StatusTooManyRequests},
	} {
		errMaps[err.err] = errType{id: id, status: err.status}
	}
}

// IsErrServiceDown checks if current service is down or not,
// it could be host not reachable or connection refused.
func IsErrServiceDown(err error) bool {
	if err == nil {
		return false
	}
	return regexConnRefused.MatchString(err.Error()) ||
		regexNotReach.MatchString(err.Error())
}

// IsErrNoTable checks if current error is no table.
func IsErrNoTable(err error) bool {
	if err == nil {
		return false
	}
	return regexNoTable.MatchString(err.Error())
}

// IsErrNoRows checks if current error is no row.
func IsErrNoRows(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), sql.ErrNoRows.Error())
}

// ReturnCheckErrNoRows check error type while return.
func ReturnCheckErrNoRows(err error) error {
	if err == nil {
		return nil
	}
	if IsErrNoRows(err) {
		return nil
	}
	return err
}

// GetErrID returns error ID by given error
func GetErrID(err error) int {
	e, ok := errMaps[err]
	if !ok {
		return 0
	}
	return e.id
}

func errToHTTPStatus(err error) int {
	e, ok := errMaps[err]
	if !ok {
		return http.StatusInternalServerError
	}
	return e.status
}
