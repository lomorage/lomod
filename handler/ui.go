package handler

import (
	"bytes"
	"context"
	"database/sql"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"

	rice "github.com/GeertJohan/go.rice"
	"github.com/chai2010/gettext-go"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"

	"bitbucket.org/lomoware/lomo-backend/common/dbx"
)

func (h *Handler) loadUIHandler(r *mux.Router) {
	r.HandleFunc("/static/lomo/js/conf.js", ConfJsHandler)
	box := rice.MustFindBox("../cmd/lomod/static")
	staticFileServer := http.StripPrefix("/static/", http.FileServer(box.HTTPBox()))
	r.PathPrefix("/static/").Handler(staticFileServer)
	r.HandleFunc("/", h.loginPageHandler).Methods("GET")
	r.HandleFunc("/welcome", h.welcomePageHandler).Methods("GET")
	r.HandleFunc("/welcome/qrcode.png", h.welcomeQRCodeHandler).Methods("GET")
	r.HandleFunc("/welcome/info", h.welcomeInfoHandler).Methods("GET")
	r.HandleFunc("/import", h.importPageHandler).Methods("GET", "HEAD")
	r.HandleFunc("/gallery", h.galleryPageHandler).Methods("GET")
	r.HandleFunc("/inbox", h.inboxPageHandler).Methods("GET")
	r.HandleFunc("/localimport", h.localImportPageHandler).Methods("GET")
	r.HandleFunc("/users", h.usersPageHandler).Methods("GET")
}

const i18nMessage = `{
	"zh_CN": {
		"LC_MESSAGES": {
			"message.json": [
				{
					"msgctxt"     : "",
					"msgid"       : "Login",
					"msgid_plural": "",
					"msgstr"      : ["登陆"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Login form",
					"msgid_plural": "",
					"msgstr"      : ["登陆表单"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Username",
					"msgid_plural": "",
					"msgstr"      : ["用户名"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Password",
					"msgid_plural": "",
					"msgstr"      : ["密码"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Confirm Password",
					"msgid_plural": "",
					"msgstr"      : ["确认密码"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Storage Location",
					"msgid_plural": "",
					"msgstr"      : ["存储位置"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Create Account",
					"msgid_plural": "",
					"msgstr"      : ["创建账户"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "No phone handy? Create your account on this page instead",
					"msgid_plural": "",
					"msgstr"      : ["没有手机在身边？在此页面创建账户"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Back to the QR code",
					"msgid_plural": "",
					"msgstr"      : ["返回二维码"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Letters, numbers, hyphens, and underscores only. Must start with a letter.",
					"msgid_plural": "",
					"msgstr"      : ["仅限字母、数字、连字符和下划线，且必须以字母开头。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "At least 6 characters.",
					"msgid_plural": "",
					"msgstr"      : ["至少6个字符。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Where your photos and videos will be stored on this computer.",
					"msgid_plural": "",
					"msgstr"      : ["您的照片和视频将存储在此计算机上的位置。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Submit",
					"msgid_plural": "",
					"msgstr"      : ["提交"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Gallery",
					"msgid_plural": "",
					"msgstr"      : ["图库"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Inbox",
					"msgid_plural": "",
					"msgstr"      : ["收件箱"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Import",
					"msgid_plural": "",
					"msgstr"      : ["导入"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Upload",
					"msgid_plural": "",
					"msgstr"      : ["上传"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Logout",
					"msgid_plural": "",
					"msgstr"      : ["退出"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Lomorage Gallery",
					"msgid_plural": "",
					"msgstr"      : ["Lomorage图库"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Set up Lomorage",
					"msgid_plural": "",
					"msgstr"      : ["设置Lomorage"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Scan this code with your phone's camera to connect and create your account. If the Lomorage app isn't installed yet, you'll be shown where to get it.",
					"msgid_plural": "",
					"msgstr"      : ["用手机相机扫描此二维码，即可连接并创建你的账户。如果还没安装Lomorage应用，会先引导你下载。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Or enter this address manually in the app:",
					"msgid_plural": "",
					"msgstr"      : ["或在应用中手动输入此地址："]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Already have an account? Log in",
					"msgid_plural": "",
					"msgstr"      : ["已有账户？去登录"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Assets Import",
					"msgid_plural": "",
					"msgstr"      : ["资源导入"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Drag and drop image and video files or directory to import them into Lomorage",
					"msgid_plural": "",
					"msgstr"      : ["要导入图片或视频到Lomorage，请将其拖拽到下面区域"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Add files...",
					"msgid_plural": "",
					"msgstr"      : ["添加文件..."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Cancel upload",
					"msgid_plural": "",
					"msgstr"      : ["取消上传"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Delete selected",
					"msgid_plural": "",
					"msgstr"      : ["删除选择"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Processing...",
					"msgid_plural": "",
					"msgstr"      : ["处理中..."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Edit",
					"msgid_plural": "",
					"msgstr"      : ["编辑"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Start",
					"msgid_plural": "",
					"msgstr"      : ["开始"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Cancel",
					"msgid_plural": "",
					"msgstr"      : ["取消"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Error",
					"msgid_plural": "",
					"msgstr"      : ["错误"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Delete",
					"msgid_plural": "",
					"msgstr"      : ["删除"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Local Import",
					"msgid_plural": "",
					"msgstr"      : ["本地导入"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Folder to scan",
					"msgid_plural": "",
					"msgstr"      : ["要扫描的文件夹"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Import photos and videos from a folder or drive connected to this computer. Files already in your library are skipped automatically.",
					"msgid_plural": "",
					"msgstr"      : ["从这台电脑上的文件夹、U 盘或存储卡导入照片和视频。已经在图库中的文件会自动跳过。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Choose a folder",
					"msgid_plural": "",
					"msgstr"      : ["选择文件夹"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Find photos",
					"msgid_plural": "",
					"msgstr"      : ["查找照片"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Tip: plug in your USB drive or SD card first, then click Browse... to pick it.",
					"msgid_plural": "",
					"msgstr"      : ["提示：先插好 U 盘或存储卡，再点“浏览...”选中它。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Choose what to import",
					"msgid_plural": "",
					"msgstr"      : ["选择要导入的内容"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "How to import",
					"msgid_plural": "",
					"msgstr"      : ["导入方式"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Copy into my library",
					"msgid_plural": "",
					"msgstr"      : ["复制到我的图库"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Recommended for USB drives and SD cards. Your original files stay as they are, and you can unplug the drive afterwards.",
					"msgid_plural": "",
					"msgstr"      : ["推荐用于 U 盘和存储卡。原文件保持不变，导入完成后可以直接拔掉。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Leave files where they are",
					"msgid_plural": "",
					"msgstr"      : ["保留在原位置"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Uses no extra space, but the photos will go missing from your library if this folder is moved, renamed or unplugged.",
					"msgid_plural": "",
					"msgstr"      : ["不占用额外空间，但如果这个文件夹被移动、改名或拔掉，图库里的这些照片就会找不到。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "View in Gallery",
					"msgid_plural": "",
					"msgstr"      : ["去图库查看"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Import another folder",
					"msgid_plural": "",
					"msgstr"      : ["导入其他文件夹"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Show import log",
					"msgid_plural": "",
					"msgstr"      : ["查看导入日志"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Select all",
					"msgid_plural": "",
					"msgstr"      : ["全选"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Select none",
					"msgid_plural": "",
					"msgstr"      : ["全不选"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Import Selected",
					"msgid_plural": "",
					"msgstr"      : ["导入所选"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Browse...",
					"msgid_plural": "",
					"msgstr"      : ["浏览..."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Select This Folder",
					"msgid_plural": "",
					"msgstr"      : ["选择此文件夹"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "No subfolders",
					"msgid_plural": "",
					"msgstr"      : ["没有子文件夹"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Users",
					"msgid_plural": "",
					"msgstr"      : ["用户"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Accounts",
					"msgid_plural": "",
					"msgstr"      : ["账号"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Add a new account to this Lomorage instance.",
					"msgid_plural": "",
					"msgstr"      : ["为这台 Lomorage 添加一个新账户。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "New User",
					"msgid_plural": "",
					"msgstr"      : ["新建用户"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Share Sign-In QR Code",
					"msgid_plural": "",
					"msgstr"      : ["分享登录二维码"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Generate a QR code so they can scan it in the Lomorage app and sign in instantly -- for a user you just created above, or any existing account.",
					"msgid_plural": "",
					"msgstr"      : ["生成一个二维码，对方在 Lomorage App 里扫一下就能直接登录——可以是刚在上面创建的新用户，也可以是任意已有账户。"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Show QR Code",
					"msgid_plural": "",
					"msgstr"      : ["显示二维码"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Open Lomorage on their phone and tap Scan to Sign In.",
					"msgid_plural": "",
					"msgstr"      : ["让对方在手机上打开 Lomorage，点击“扫码登录”。"]
				}
			]
		}
	},

	"en_US": {
		"LC_MESSAGES": {
			"message.json": [
				{
					"msgctxt"     : "",
					"msgid"       : "Login",
					"msgid_plural": "",
					"msgstr"      : ["Login"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Login form",
					"msgid_plural": "",
					"msgstr"      : ["Login form"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Username",
					"msgid_plural": "",
					"msgstr"      : ["Username"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Password",
					"msgid_plural": "",
					"msgstr"      : ["Password"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Confirm Password",
					"msgid_plural": "",
					"msgstr"      : ["Confirm Password"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Storage Location",
					"msgid_plural": "",
					"msgstr"      : ["Storage Location"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Create Account",
					"msgid_plural": "",
					"msgstr"      : ["Create Account"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "No phone handy? Create your account on this page instead",
					"msgid_plural": "",
					"msgstr"      : ["No phone handy? Create your account on this page instead"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Back to the QR code",
					"msgid_plural": "",
					"msgstr"      : ["Back to the QR code"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Letters, numbers, hyphens, and underscores only. Must start with a letter.",
					"msgid_plural": "",
					"msgstr"      : ["Letters, numbers, hyphens, and underscores only. Must start with a letter."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "At least 6 characters.",
					"msgid_plural": "",
					"msgstr"      : ["At least 6 characters."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Where your photos and videos will be stored on this computer.",
					"msgid_plural": "",
					"msgstr"      : ["Where your photos and videos will be stored on this computer."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Submit",
					"msgid_plural": "",
					"msgstr"      : ["Submit"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Gallery",
					"msgid_plural": "",
					"msgstr"      : ["Gallery"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Inbox",
					"msgid_plural": "",
					"msgstr"      : ["Inbox"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Import",
					"msgid_plural": "",
					"msgstr"      : ["Import"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Upload",
					"msgid_plural": "",
					"msgstr"      : ["Upload"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Logout",
					"msgid_plural": "",
					"msgstr"      : ["Logout"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Lomorage Gallery",
					"msgid_plural": "",
					"msgstr"      : ["Lomorage Gallery"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Set up Lomorage",
					"msgid_plural": "",
					"msgstr"      : ["Set up Lomorage"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Scan this code with your phone's camera to connect and create your account. If the Lomorage app isn't installed yet, you'll be shown where to get it.",
					"msgid_plural": "",
					"msgstr"      : ["Scan this code with your phone's camera to connect and create your account. If the Lomorage app isn't installed yet, you'll be shown where to get it."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Or enter this address manually in the app:",
					"msgid_plural": "",
					"msgstr"      : ["Or enter this address manually in the app:"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Already have an account? Log in",
					"msgid_plural": "",
					"msgstr"      : ["Already have an account? Log in"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Assets Import",
					"msgid_plural": "",
					"msgstr"      : ["Assets Import"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Drag and drop image and video files or directory to import them into Lomorage",
					"msgid_plural": "",
					"msgstr"      : ["Drag and drop image and video files or directory to import them into Lomorage"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Add files...",
					"msgid_plural": "",
					"msgstr"      : ["Add files..."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Cancel upload",
					"msgid_plural": "",
					"msgstr"      : ["Cancel upload"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Delete selected",
					"msgid_plural": "",
					"msgstr"      : ["Delete selected"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Processing...",
					"msgid_plural": "",
					"msgstr"      : ["Processing..."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Edit",
					"msgid_plural": "",
					"msgstr"      : ["Edit"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Start",
					"msgid_plural": "",
					"msgstr"      : ["Start"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Cancel",
					"msgid_plural": "",
					"msgstr"      : ["Cancel"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Error",
					"msgid_plural": "",
					"msgstr"      : ["Error"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Delete",
					"msgid_plural": "",
					"msgstr"      : ["Delete"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Local Import",
					"msgid_plural": "",
					"msgstr"      : ["Local Import"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Folder to scan",
					"msgid_plural": "",
					"msgstr"      : ["Folder to scan"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Select all",
					"msgid_plural": "",
					"msgstr"      : ["Select all"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Select none",
					"msgid_plural": "",
					"msgstr"      : ["Select none"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Import Selected",
					"msgid_plural": "",
					"msgstr"      : ["Import Selected"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Browse...",
					"msgid_plural": "",
					"msgstr"      : ["Browse..."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Select This Folder",
					"msgid_plural": "",
					"msgstr"      : ["Select This Folder"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "No subfolders",
					"msgid_plural": "",
					"msgstr"      : ["No subfolders"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Users",
					"msgid_plural": "",
					"msgstr"      : ["Users"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Accounts",
					"msgid_plural": "",
					"msgstr"      : ["Accounts"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Add a new account to this Lomorage instance.",
					"msgid_plural": "",
					"msgstr"      : ["Add a new account to this Lomorage instance."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "New User",
					"msgid_plural": "",
					"msgstr"      : ["New User"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Share Sign-In QR Code",
					"msgid_plural": "",
					"msgstr"      : ["Share Sign-In QR Code"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Generate a QR code so they can scan it in the Lomorage app and sign in instantly -- for a user you just created above, or any existing account.",
					"msgid_plural": "",
					"msgstr"      : ["Generate a QR code so they can scan it in the Lomorage app and sign in instantly -- for a user you just created above, or any existing account."]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Show QR Code",
					"msgid_plural": "",
					"msgstr"      : ["Show QR Code"]
				},
				{
					"msgctxt"     : "",
					"msgid"       : "Open Lomorage on their phone and tap Scan to Sign In.",
					"msgid_plural": "",
					"msgstr"      : ["Open Lomorage on their phone and tap Scan to Sign In."]
				}
			]
		}
	}
}`

var gText gettext.Gettexter = gettext.New("message", "", i18nMessage).SetLanguage("en_US")

func changeLocale(locale string) {
	gText.SetLanguage(locale)
}

func translate(input string) string {
	return gText.PGettext("", input)
}

type WebData struct {
	Foot template.HTML
}

// LoadFile load html file
func (h *Handler) loadTemplateFile(fileName string) (string, error) {
	// find a rice.Box
	templateBox, err := rice.FindBox("../cmd/lomod/templates")
	if err != nil {
		log.Printf("error finding templates: %v\n", err)
		return "", err
	}
	// get file contents as string
	templateString, err := templateBox.String(fileName)
	if err != nil {
		log.Printf("error reading templates: %v\n", err)
		return "", err
	}

	funcMap := template.FuncMap{
		"gettext": translate,
	}
	t, _ := template.New("foo").Funcs(funcMap).Parse(templateString)
	var tpl bytes.Buffer
	wd := WebData{
		Foot: template.HTML(h.conf.WebFootHtml),
	}
	if err := t.Execute(&tpl, &wd); err != nil {
		return "", err
	}

	return tpl.String(), nil
}

// ChangePreferedLanguage change lang according to http header
func (h *Handler) changePreferedLanguage(r *http.Request) {
	var matcher = language.NewMatcher([]language.Tag{
		language.English, // The first language is used as fallback.
		language.Chinese,
	})

	accept := r.Header.Get("Accept-Language")
	tag, _ := language.MatchStrings(matcher, accept)
	if strings.HasPrefix(tag.String(), "zh") {
		changeLocale("zh_CN")
	} else {
		changeLocale("en_US")
	}
}

// LoginPageHandler for GET
func (h *Handler) loginPageHandler(response http.ResponseWriter, request *http.Request) {
	h.changePreferedLanguage(request)

	if h.needsSetupRedirect() {
		http.Redirect(response, request, "/welcome", http.StatusFound)
		return
	}

	var body, _ = h.loadTemplateFile("login.html")
	io.WriteString(response, body)
}

// needsSetupRedirect reports whether "/" should send the visitor to the
// setup QR page instead of the login form: true whenever no account has
// been created yet (not just on the very first visit), so the QR stays
// reachable until an account actually exists.
func (h *Handler) needsSetupRedirect() bool {
	hasUser := true
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		hasUser, err = h.hasAnyUser(ctx, tx)
		return err
	})
	if err != nil {
		logrus.Warnf("checking setup status: %v", err)
		return false
	}
	return !hasUser
}

// ImportPageHandler for GET
func (h *Handler) importPageHandler(response http.ResponseWriter, request *http.Request) {
	h.changePreferedLanguage(request)
	var body, _ = h.loadTemplateFile("import.html")
	io.WriteString(response, body)
}

// GalleryPageHandler for GET
func (h *Handler) galleryPageHandler(response http.ResponseWriter, request *http.Request) {
	h.changePreferedLanguage(request)
	var body, _ = h.loadTemplateFile("gallery.html")
	io.WriteString(response, body)
}

// InboxPageHandler for GET
func (h *Handler) inboxPageHandler(response http.ResponseWriter, request *http.Request) {
	h.changePreferedLanguage(request)
	var body, _ = h.loadTemplateFile("inbox.html")
	io.WriteString(response, body)
}

// LocalImportPageHandler for GET
func (h *Handler) localImportPageHandler(response http.ResponseWriter, request *http.Request) {
	h.changePreferedLanguage(request)
	var body, _ = h.loadTemplateFile("localimport.html")
	io.WriteString(response, body)
}

// UsersPageHandler for GET
func (h *Handler) usersPageHandler(response http.ResponseWriter, request *http.Request) {
	h.changePreferedLanguage(request)
	var body, _ = h.loadTemplateFile("users.html")
	io.WriteString(response, body)
}

// ConfJsTemplate conf.js template
var ConfJsTemplate = `

var CONFIG = {
    LOGIN_URI: 'login',
    ASSERT_URI: 'asset',
	PREVIEW_URI: 'preview',
	CATEGORY_URI: 'assets/merkletree',
	RECEIVE_URI: 'receive',
	RECEIVE_USER_URI: 'receive/user',
	RECEIVE_GROUP_URI: 'receive/group',
	RECEIVE_ASSERT_URI: 'receive/asset',
	RECEIVE_PREVIEW_URI: 'receive/preview',
	USER_URI: 'user',
	MOUNT_URI: 'mount',
	GROUP_URI: 'group',
	SCAN_URI: 'assets/scan',
	SCAN_LOG_URI: 'assets/scan/log',
	SCAN_IMPORT_URI: 'assets/scan/import',
	SCAN_BROWSE_URI: 'assets/scan/browse',
	SCAN_STATUS_URI: 'assets/scan/status',
	DEV_SERVER_KEY: 'lomo_dev_server',

	// Lets a developer point this page at a different lomod instance than the
	// one that served it, e.g. to test local frontend changes against a
	// remote/staging backend: open any page once with ?server=http://host:port
	// and it's remembered (sessionStorage) across page-to-page navigation for
	// the rest of the tab. ?server= (empty) or ?server=clear removes it.
	initDevServerOverride: function() {
		var params = new URLSearchParams(window.location.search);
		if (!params.has('server')) {
			return;
		}
		var value = params.get('server');
		if (!value || value === 'clear') {
			sessionStorage.removeItem(CONFIG.DEV_SERVER_KEY);
		} else {
			sessionStorage.setItem(CONFIG.DEV_SERVER_KEY, value.replace(/\/+$/, ''));
		}
	},

	getServiceUrl: function() {
		var override = sessionStorage.getItem(CONFIG.DEV_SERVER_KEY);
		if (override) {
			return override;
		}
		return window.location.protocol + "//" + window.location.hostname + ":" + window.location.port;
	},

    getLoginUrl: function() {
        return CONFIG.getServiceUrl() + '/' + CONFIG.LOGIN_URI;
    },

    getUploadUrl: function() {
        return CONFIG.getServiceUrl() + '/' + CONFIG.ASSERT_URI;
    },

    getAssetUrl: function(name) {
		var ext = name.split('.').pop().toLowerCase();
		if (ext == "tif") {
			return CONFIG.getServiceUrl() + '/' + CONFIG.ASSERT_URI + '/' + name + "?token=" + sessionStorage.getItem("token") + "&icodec=jpg";
		} else {
			return CONFIG.getServiceUrl() + '/' + CONFIG.ASSERT_URI + '/' + name + "?token=" + sessionStorage.getItem("token") + "&orig=1";
		}
    },

    getPreviewUrl: function(name) {
        return CONFIG.getServiceUrl() + '/' + CONFIG.PREVIEW_URI + '/' + name + "?width=320&height=-1&token=" + sessionStorage.getItem("token");
	},

	getMonthLevelMerkleTreeUrl: function() {
		return CONFIG.getServiceUrl() + '/' + CONFIG.CATEGORY_URI;
	},

	getAssetLevelMerkleTreeUrl: function(year, month) {
		return CONFIG.getServiceUrl() + '/' + CONFIG.CATEGORY_URI + '/' + year + '/' + month;
	},

	getUsersUrl: function() {
		return CONFIG.getServiceUrl() + '/' + CONFIG.USER_URI;
	},

	getMountUrl: function() {
		return CONFIG.getServiceUrl() + '/' + CONFIG.MOUNT_URI;
	},

	getScanUrl: function(path, extraParams) {
		var url = CONFIG.getServiceUrl() + '/' + CONFIG.SCAN_URI + '?path=' + encodeURIComponent(path);
		return extraParams ? url + '&' + extraParams : url;
	},

	getScanLogUrl: function(path) {
		return CONFIG.getServiceUrl() + '/' + CONFIG.SCAN_LOG_URI + '?path=' + encodeURIComponent(path);
	},

	getScanImportUrl: function(name) {
		return CONFIG.getServiceUrl() + '/' + CONFIG.SCAN_IMPORT_URI + '/' + encodeURIComponent(name);
	},

	getScanBrowseUrl: function(path) {
		var url = CONFIG.getServiceUrl() + '/' + CONFIG.SCAN_BROWSE_URI;
		return path ? url + '?path=' + encodeURIComponent(path) : url;
	},

	getScanStatusUrl: function() {
		return CONFIG.getServiceUrl() + '/' + CONFIG.SCAN_STATUS_URI;
	},

	getGroupsUrl: function() {
		return CONFIG.getServiceUrl() + '/' + CONFIG.GROUP_URI;
	},

	getInboxUrl: function() {
		return CONFIG.getServiceUrl() + '/' + CONFIG.RECEIVE_URI + "?token=" + sessionStorage.getItem("token");
	},

	getUserInboxUrl: function(uid) {
		return CONFIG.getServiceUrl() + '/' + CONFIG.RECEIVE_USER_URI + '/' + uid + "?token=" + sessionStorage.getItem("token");
	},

	getGroupInboxUrl: function(gid) {
		return CONFIG.getServiceUrl() + '/' + CONFIG.RECEIVE_GROUP_URI + '/' + gid + "?token=" + sessionStorage.getItem("token");
	},

	getInboxAssetUrl: function(shareid) {
        return CONFIG.getServiceUrl() + '/' + CONFIG.RECEIVE_ASSERT_URI + '/' + shareid + "?token=" + sessionStorage.getItem("token") + "&orig=1";
    },

    getInboxPreviewUrl: function(shareid) {
        return CONFIG.getServiceUrl() + '/' + CONFIG.RECEIVE_PREVIEW_URI + '/' + shareid + "?width=320&height=-1&token=" + sessionStorage.getItem("token");
	}
}

CONFIG.initDevServerOverride();

(function() {
	var server = sessionStorage.getItem(CONFIG.DEV_SERVER_KEY);
	if (!server) {
		return;
	}
	var banner = document.createElement('div');
	banner.textContent = 'Dev mode — talking to ' + server;
	banner.style.cssText = 'position:fixed;left:0;right:0;bottom:0;z-index:2000;' +
		'background:var(--warning,#cf7a12);color:#fff;font:12px var(--font-ui,sans-serif);' +
		'text-align:center;padding:4px 8px;';
	document.body.appendChild(banner);
})();
`

// ConfJsHandler server conf.js
func ConfJsHandler(response http.ResponseWriter, request *http.Request) {
	io.WriteString(response, ConfJsTemplate)
}
