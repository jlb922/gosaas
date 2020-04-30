package gosaas

import (
	"fmt"
	"net/http"

	"github.com/jlb922/gosaas/data"
	"github.com/jlb922/gosaas/model"
)

// Tool handles everything related to the /tools requests
type Tool struct{}

func newTool() *Route {
	var t interface{} = Tool{}
	return &Route{
		AllowCrossOrigin: true,
		Logger:           true,
		MinimumRole:      model.RoleUser,
		WithDB:           true,
		Handler:          t.(http.Handler),
	}
}

// Handler for /tool routes
func (t Tool) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	var head string
	fmt.Println("Tools Route")
	head, r.URL.Path = ShiftPath(r.URL.Path)
	if head == "reload" {
		t.reload(w, r)
	} else if head == "profile" && r.Method == http.MethodGet {
		t.profile(w, r)
	} else {
		// route not Found
		ServePage(w, r, "index.html", nil)
	}
}

func (t Tool) reload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	isJSON := ctx.Value(ContextContentIsJSON).(bool)

	fmt.Println("About to load templates")
	LoadTemplates()
	fmt.Println("Templates reloaded!")
	if isJSON {
		Respond(w, r, http.StatusOK, nil)
	} else {
		ServePage(w, r, "index.html", nil)
	}
}

func (t Tool) profile(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	keys := ctx.Value(ContextAuth).(Auth)

	//keys := Auth{
	//	AccountID: 7,
	//	UserID:    7,
	//	Email:     "jlb922@gmail.com",
	//	Role:      model.RoleAdmin,
	//}

	db := ctx.Value(ContextDatabase).(*data.DB)

	acct, err := db.Users.GetDetail(keys.AccountID)
	if err != nil {
		Respond(w, r, http.StatusInternalServerError, err)
		return
	}

	Respond(w, r, http.StatusOK, acct)
}
