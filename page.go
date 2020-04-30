package gosaas

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jlb922/gosaas/model"
)

var (
	pageTemplates *template.Template
	languagePacks map[string]map[string]string
)

func init() {
	LoadTemplates()
	loadLanguagePacks()
}

// LoadTemplates reads templates into memory
func LoadTemplates() {
	var tmpl []string

	// Experimental code
	err := filepath.Walk("./templates",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			//fmt.Println(path, info.Name())
			if !info.IsDir() {
				tmpl = append(tmpl, path)
			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}

	// read template directory
	//files, err := ioutil.ReadDir("./templates")
	//if err != nil {
	//	if os.IsNotExist(err) == false {
	//		log.Fatal("unable to load templates", err)
	//	}
	//	return
	//}

	//for _, f := range files {
	//	if !f.IsDir() {
	//		tmpl = append(tmpl, path.Join("./templates", f.Name()))
	//	}
	//}

	t, err := template.New("").Funcs(template.FuncMap{
		"translate":  Translate,
		"translatef": Translatef,
		"money": func(amount int) string {
			m := float64(amount) / 100.0
			return fmt.Sprintf("%.2f $", m)
		},
		//	}).ParseFiles(tmpl...)
	}).ParseFiles(tmpl...)
	if err != nil {
		log.Fatal("error while parsing templates", err)
	}

	pageTemplates = t
}

// ServePage will render and respond with an HTML template.ServePage
//
// HTML templates should be saved into a directory named templates.ServePage
//
// Example usage:
//
// 	func handler(w http.ResponseWriter, r *http.Request) {
// 		data := HomePage{Title: "Hello world!"}
// 		gosaas.ServePage(w, r, "index.html", data)
// 	}
func ServePage(w http.ResponseWriter, r *http.Request, name string, data interface{}) {

	t := pageTemplates.Lookup(name)

	if err := t.Execute(w, data); err != nil {
		fmt.Println("error while rendering the template ", err)
	}

	logRequest(r, http.StatusOK)
}

func loadLanguagePacks() {
	languagePacks = make(map[string]map[string]string)

	files, err := ioutil.ReadDir("./languagepacks")
	if err != nil {
		log.Println("unable to load language packs: ", err)
		return
	}

	var pack = new(struct {
		Language string `json:"lang"`
		Keys     []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"keys"`
	})

	for _, f := range files {
		b, err := ioutil.ReadFile(path.Join("./languagepacks", f.Name()))
		if err != nil {
			log.Fatal("unable to read language pack: ", f.Name(), ": ", err)
		}

		if err := json.Unmarshal(b, &pack); err != nil {
			log.Fatal("unable to parse language pack: ", f.Name(), ": ", err)
		}

		values := make(map[string]string)
		for _, k := range pack.Keys {
			values[k.Key] = k.Value
		}

		languagePacks[pack.Language] = values
	}
}

// Translate finds a key in a language pack file (saved in directory named languagepack)
// and return the value as template.HTML so it's safe to use HTML inside the language pack file.Translate
//
// The language pack file are simple JSON file named lng.json like en.json:
//
// 	{
// 		"lang": "en",
// 		"keys": [
// 			{"key": "landing-title", "value": "Welcome to my site"}
// 		]
// 	}
func Translate(lng, key string) template.HTML {
	if s, ok := languagePacks[lng][key]; ok {
		return template.HTML(s)
	}
	return template.HTML(fmt.Sprintf("key %s not found", key))
}

// Translatef finds a translation key and substitute the formatting parameters.
func Translatef(lng, key string, a ...interface{}) string {
	if s, ok := languagePacks[lng][key]; ok {
		return fmt.Sprintf(s, a...)
	}
	return fmt.Sprintf("key %s not found", key)
}

// ExtractLimitAndOffset BUG(dom): This needs more thinking...
func ExtractLimitAndOffset(r *http.Request) (limit int, offset int) {
	limit = 50
	offset = 0

	p := r.URL.Query().Get("limit")
	if len(p) > 0 {
		i, err := strconv.Atoi(p)
		if err == nil {
			limit = i
		}
	}

	p = r.URL.Query().Get("offset")
	if len(p) > 0 {
		i, err := strconv.Atoi(p)
		if err == nil {
			offset = i
		}
	}

	return
}

// ViewData is the base data needed for all pages to render.
//
// It will automatically get the user's language, role and if there's an alert
// to display. You can view this a a wrapper around what you would have sent to the
// page being redered.
type ViewData struct {
	Language string
	Role     model.Roles
	Alert    *Notification
	Data     interface{}
}

// Notification can be used to display alert to the user in an HTML template.
type Notification struct {
	//Title     template.HTML
	//Message   template.HTML
	Title     string
	Message   string
	IsSuccess bool
	IsError   bool
	IsWarning bool
	IsInfo    bool
}

func getLanguage(ctx context.Context) string {
	lng, ok := ctx.Value(ContextLanguage).(string)
	if !ok {
		lng = "en"
	}
	return lng
}

func getRole(ctx context.Context) model.Roles {
	auth, ok := ctx.Value(ContextAuth).(Auth)
	if !ok {
		return model.RolePublic
	}
	return auth.Role
}

// CreateViewData wraps the data into a ViewData type where the language, role and
// notification will be automatically added along side the data.
func CreateViewData(ctx context.Context, alert *Notification, data interface{}) ViewData {
	return ViewData{
		Alert:    alert,
		Data:     data,
		Language: getLanguage(ctx),
		Role:     getRole(ctx),
	}
}

// TokenizeNotificationCookie takes a Notification struct and eturns a string with pipe separated alert valuee.
func TokenizeNotificationCookie(alert Notification) string {
	return fmt.Sprintf("%s|%s|%t|%t|%t|%t", alert.Title, alert.Message, alert.IsInfo, alert.IsSuccess, alert.IsError, alert.IsWarning)
}

// ParseNotificationCookie Splits a notification string froma cookie back into a Notification Struct
func ParseNotificationCookie(alertString string) (Notification, error) {
	var tmpNotif Notification
	pairs := strings.Split(alertString, "|")
	if len(pairs) != 6 {
		return tmpNotif, fmt.Errorf("Too few fields in cookie")
	}
	tmpNotif.Title = pairs[0]
	tmpNotif.Message = pairs[1]
	tmpNotif.IsInfo, _ = strconv.ParseBool(pairs[2])
	tmpNotif.IsSuccess, _ = strconv.ParseBool(pairs[3])
	tmpNotif.IsError, _ = strconv.ParseBool(pairs[4])
	tmpNotif.IsWarning, _ = strconv.ParseBool(pairs[5])
	return tmpNotif, nil
}

// ProcessNotificationCookie will read and untokenize a Notification Cookie
func ProcessNotificationCookie(w http.ResponseWriter, r *http.Request) (Notification, error) {
	var alert Notification
	// fetch the cookie and check for errors
	ck, err := r.Cookie("FLASH-ALERT")
	if err != nil {
		// If it's ErrNoCookie we must continue otherwise this is a legit error
		if err != http.ErrNoCookie {
			return alert, err
		}
	} else {
		// unpack the alert
		alert, err = ParseNotificationCookie(ck.Value)

		// Set an empty cookie so the alert only displays once
		ck = &http.Cookie{
			Name:    "FLASH-ALERT",
			MaxAge:  -1,
			Expires: time.Unix(1, 0),
			Value:   "deleteme",
		}
		http.SetCookie(w, ck)
	}
	return alert, err
}

// SetNotificationCookie sets a flash notification cookie
func SetNotificationCookie(w http.ResponseWriter, alert Notification) {

	ck := &http.Cookie{
		Name:  "FLASH-ALERT",
		Path:  "/",
		Value: TokenizeNotificationCookie(alert),
	}

	// we set the cookie so they will have their authentication token on the next request.
	http.SetCookie(w, ck)
}
