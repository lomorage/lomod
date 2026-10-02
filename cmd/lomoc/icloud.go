package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manifoldco/promptui"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/google/uuid"
	"github.com/urfave/cli"
)

const (
	setupEndpoint       = "https://setup.icloud.com/setup/ws/1"
	authenticationURL   = setupEndpoint + "/login"
	listDevicesURL      = setupEndpoint + "/listDevices"
	storageInfoURL      = setupEndpoint + "/storageUsageInfo"
	validateVCodeURL    = setupEndpoint + "/validateVerificationCode"
	sendVCodeURL        = setupEndpoint + "/sendVerificationCode"
	icloudResponse1     = "icloud_response1.txt"
	icloudResponseData1 = "icloud_response_data1.json"
	icloudSessionData1  = "icloud_session1.json"
	icloudResponse2     = "icloud_response2.txt"
	icloudResponseData2 = "icloud_response_data2.json"
	icloudSessionData2  = "icloud_session2.json"
)

var (
	debugLog   = false
	httpClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 5,
		},
		Timeout: 30 * time.Second,
	}
	queries = map[string]string{
		"clientBuildNumber":     "17DHotfix5",
		"clientMasteringNumber": "17DHotfix5",
		"ckjsBuildVersion":      "17DProjectDev77",
		"ckjsVersion":           "2.0.5",
		"clientId":              uuid.New().String(),
	}
	headers = map[string]string{
		"Origin":     "https://www.icloud.com",
		"Referer":    "https://www.icloud.com/",
		"User-Agent": "Opera/9.52 (X11; Linux i686; U; en)",
	}
)

type icloudUserLogin struct {
	ID       string `json:"apple_id"`
	Password string `json:"password"`
	// ExtendedLogin is like "remember me"
	ExtendedLogin bool `json:"extended_login"`
}

type icloudWebService struct {
	URL         string `json:"url"`
	Status      string `json:"status"`
	PcsRequired bool   `json:"pcsRequired"`
}

type icloudUserLoginReply struct {
	HsaChallengeRequired bool                        `json:"hsaChallengeRequired"`
	DsInfo               dsInfo                      `json:"dsInfo"`
	WebServices          map[string]icloudWebService `json:"webservices"`
}

type dsInfo struct {
	Dsid       string `json:"dsid"`
	HsaVersion int    `json:"hsaVersion"`
	HsaEnabled bool   `json:"hsaEnabled"`
}

type icloudSession struct {
	HsaChallengeRequired bool
	HsaVersion           int
	WebServices          map[string]icloudWebService
	Query                map[string]string
	Cookie               []*http.Cookie
}

type icloudTrustedDevice struct {
	ID          string `json:"deviceId"`
	Name        string `json:"deviceName"`
	Type        string `json:"deviceType"`
	AreaCode    string `json:"areaCode"`
	PhoneNumber string `json:"phoneNumber"`
}

type verifyDevice struct {
	Code         string `json:"verificationCode"`
	TrustBrowser bool   `json:"trustBrowser"`
}

type icloudTrustedDevices struct {
	Devices []icloudTrustedDevice `json:"devices"`
}

type icloudValidationResult struct {
	Success bool `json:"success"`
}

type icloudStorageUsage struct {
	Key   string `json:"mediaKey"`
	Label string `json:"displayLabel"`
	Color string `json:"displayColor"`
	Usage int    `json:"usageInBytes"`
}

type icloudStorageUsageInfo struct {
	CompInBytess    int `json:"compStorageInBytes"`
	UsedInBytes     int `json:"usedStorageInBytes"`
	TotalInBytes    int `json:"totalStorageInBytes"`
	CommerceInBytes int `json:"commerceStorageInBytes"`
}

type icloudStorageQuotaStatus struct {
	OverQuota    bool `json:"overQuota"`
	MaxQuotaTier bool `json:"haveMaxQuotaTier"`
	AlmostFull   bool `json:"almost-full"`
	PaidQuota    bool `json:"paidQuota"`
}

type icloudStorageUsages struct {
	Usages []icloudStorageUsage     `json:"storageUsageByMedia"`
	Info   icloudStorageUsageInfo   `json:"storageUsageInfo"`
	Quota  icloudStorageQuotaStatus `json:"quotaStatus"`
}

func sendRequest(method, u string, q map[string]string, h map[string]string, c []*http.Cookie, body io.Reader) (*http.Response, []byte, error) {
	vars := []string{}
	for k, v := range q {
		vars = append(vars, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v)))
	}
	u = u + "?" + strings.Join(vars, "&")
	if debugLog {
		fmt.Println("----> sending request: " + u)
	}

	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, nil, err
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}
	for _, cookie := range c {
		req.AddCookie(cookie)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	} else if resp.StatusCode > http.StatusInternalServerError {
		return nil, nil, fmt.Errorf("server side error (%s), please try again", http.StatusText(resp.StatusCode))
	} else if resp.StatusCode == http.StatusForbidden {
		return nil, nil, errors.New("invalid username / password")
	} else if resp.StatusCode != http.StatusOK {
		if debugLog && resp != nil {
			fmt.Printf("<---- receive response: %v\n", *resp)
		}
		return nil, nil, fmt.Errorf("receive error - %s", http.StatusText(resp.StatusCode))
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if debugLog {
		fmt.Printf("<---- receive response: %v\n", *resp)
		if len(data) > 0 {
			fmt.Printf("<---- receive body: %s\n", string(data))
		}
	}
	return resp, data, nil
}

func icloudLoadSession() (*icloudSession, error) {
	dir, err := getDefaultLomoDir()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	session := &icloudSession{}
	data, err := ioutil.ReadFile(filepath.Join(dir, icloudSessionData2))
	if err != nil {
		return nil, err
	}
	return session, json.Unmarshal(data, session)
}

func icloudLogin(ctx *cli.Context) error {
	if len(ctx.Args()) != 2 {
		return errors.New("please input user name / password")
	}
	if ctx.GlobalBool("debug-log") {
		debugLog = true
	}

	dir, err := getDefaultLomoDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(dir, common.DefaultFolderPermission); err != nil {
			return err
		}
	}
	fileResponse := filepath.Join(dir, icloudResponse1)
	fileResponseData := filepath.Join(dir, icloudResponseData1)
	fileSessionData := filepath.Join(dir, icloudSessionData1)

	var session *icloudSession
	if _, err := os.Stat(fileSessionData); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		session, err = icloudLoginRequest(ctx.Args()[0], ctx.Args()[1], fileResponse, fileResponseData, fileSessionData, nil)
		if err != nil {
			return err
		}
	} else {
		fmt.Println("Load cached session data")
		session = &icloudSession{}
		data, err := ioutil.ReadFile(fileSessionData)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, session); err != nil {
			return err
		}
	}
	fmt.Println("Initial authentication success")
	// check Two-step/two-factor authentication
	if !(session.HsaChallengeRequired && session.HsaVersion >= 1) {
		return nil
	}
	fmt.Println("Start two step authentication")
	if err := icloudListTrustedDevice(session); err != nil {
		return err
	}

	fmt.Println("Finish two step authentication, and re-authenticate")

	fileResponse = filepath.Join(dir, icloudResponse2)
	fileResponseData = filepath.Join(dir, icloudResponseData2)
	fileSessionData = filepath.Join(dir, icloudSessionData2)

	_, err = icloudLoginRequest(ctx.Args()[0], ctx.Args()[1], fileResponse, fileResponseData, fileSessionData, session)
	if err != nil {
		return err
	}

	fmt.Println("Finish re-authenticate, you are good now")
	return nil
}

func icloudLoginRequest(username, password, fileResponse, fileResponseData, fileSessionData string, session *icloudSession) (*icloudSession, error) {
	login := &icloudUserLogin{ID: username, Password: password}
	data, err := json.Marshal(login)
	if err != nil {
		return nil, err
	}

	qs := queries
	var cookies []*http.Cookie
	if session != nil {
		qs = session.Query
		cookies = session.Cookie
	}
	resp, data, err := sendRequest(http.MethodPost, authenticationURL, qs, headers, cookies, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := ioutil.WriteFile(fileResponse, []byte(fmt.Sprintf("%v", *resp)), common.DefaultFolderPermission); err != nil {
		return nil, err
	}
	if err := ioutil.WriteFile(fileResponseData, []byte(fmt.Sprintf("%v", *resp)), common.DefaultFolderPermission); err != nil {
		return nil, err
	}
	reply := &icloudUserLoginReply{}
	if err := json.Unmarshal(data, reply); err != nil {
		return nil, err
	}

	queries["dsid"] = reply.DsInfo.Dsid
	newSession := &icloudSession{
		HsaVersion:           reply.DsInfo.HsaVersion,
		HsaChallengeRequired: reply.HsaChallengeRequired,
		Query:                queries,
		Cookie:               resp.Cookies(),
		WebServices:          reply.WebServices,
	}
	data, err = json.Marshal(newSession)
	if err != nil {
		return nil, err
	}

	return newSession, ioutil.WriteFile(fileSessionData, data, common.DefaultFolderPermission)
}

func icloudListTrustedDevice(session *icloudSession) error {
	resp, data, err := sendRequest(http.MethodGet, listDevicesURL, session.Query, headers, session.Cookie, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	devices := &icloudTrustedDevices{}
	if err := json.Unmarshal(data, devices); err != nil {
		return err
	}

	names := []string{}
	for _, d := range devices.Devices {
		if d.Name != "" {
			names = append(names, d.Name)
		} else {
			names = append(names, "SMS to "+d.PhoneNumber)
		}
	}
	names = append(names, "Enter two-factor authentication code")

	for {
		device := icloudTrustedDevice{}
		prompt := promptui.Select{
			Label: "Select device to send verification code",
			Items: names,
			Size:  len(names),
		}

		idx, _, err := prompt.Run()
		if err != nil {
			return err
		}

		if idx < len(devices.Devices) {
			device = devices.Devices[idx]
			if err := sendValidationCode(session, device); err != nil {
				return err
			}
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Please enter two-factor authentication code: -> ")
		code, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		ok, err := validationCode(session, device, code)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		fmt.Println("Verification fail. Please try again")
	}
}

func sendValidationCode(session *icloudSession, device icloudTrustedDevice) error {
	data, err := json.Marshal(device)
	if err != nil {
		return err
	}

	resp, _, err := sendRequest(http.MethodPost, sendVCodeURL, session.Query, headers, session.Cookie, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func validationCode(session *icloudSession, d icloudTrustedDevice, code string) (bool, error) {
	fmt.Printf("icloude trusted device: %v\n", d)

	device := verifyDevice{}
	device.Code = code
	device.TrustBrowser = true

	data, err := json.Marshal(device)
	if err != nil {
		return false, err
	}

	resp, data, err := sendRequest(http.MethodPost, validateVCodeURL, session.Query, headers, session.Cookie, bytes.NewBuffer(data))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	result := &icloudValidationResult{}
	if err := json.Unmarshal(data, result); err != nil {
		return false, err
	}

	if !result.Success {
		return false, nil
	}

	if len(resp.Cookies()) > 0 {
		session.Cookie = resp.Cookies()
	}
	return true, nil
}

func icloudStorageUsageDump(ctx *cli.Context) error {
	if ctx.GlobalBool("debug-log") {
		debugLog = true
	}
	session, err := icloudLoadSession()
	if err != nil {
		return err
	}
	result, err := getStorageInfo(session)
	if err != nil {
		return err
	}

	fmt.Printf("Total: %d Bytes, Used: %d Bytes\n", result.Info.TotalInBytes, result.Info.UsedInBytes)

	for _, usage := range result.Usages {
		fmt.Printf("\t%s\t: %d Bytes (%.2f%%)\n", usage.Label, usage.Usage, 100.0*float32(usage.Usage)/float32(result.Info.UsedInBytes))
	}
	return nil
}

func getStorageInfo(session *icloudSession) (*icloudStorageUsages, error) {
	resp, data, err := sendRequest(http.MethodGet, storageInfoURL, session.Query, headers, session.Cookie, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result := &icloudStorageUsages{}
	return result, json.Unmarshal(data, result)
}
