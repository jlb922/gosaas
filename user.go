package gosaas

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strconv"

	"github.com/jlb922/gosaas/data"
	"github.com/jlb922/gosaas/internal/config"
	"github.com/jlb922/gosaas/model"
	"github.com/jlb922/gosaas/queue"
	"golang.org/x/crypto/bcrypt"
)

type pageData struct {
	Title  string
	Header string
}

type pwdResetData struct {
	ID    string
	token string
}

// User handles everything related to the /user requests
type User struct{}

func newUser() *Route {
	var u interface{} = User{}
	return &Route{
		AllowCrossOrigin: true,
		Logger:           true,
		MinimumRole:      model.RolePublic,
		WithDB:           true,
		GzipCompression:  true,
		Handler:          u.(http.Handler),
	}
}

// Handler for /user routes
func (u User) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var head string
	fmt.Println("Path:", r.URL.Path)
	head, r.URL.Path = ShiftPath(r.URL.Path)

	if head == "signup" {
		if r.Method == http.MethodGet {
			u.signup(w, r)
		} else if r.Method == http.MethodPost {
			u.create(w, r)
		}
	} else if head == "login" {
		if r.Method == http.MethodGet {
			u.login(w, r)
		} else if r.Method == http.MethodPost {
			u.signin(w, r)
		}
	} else if head == "forgot" {
		if r.Method == http.MethodGet {
			u.forgot(w, r)
		} else if r.Method == http.MethodPost {
			u.sendReset(w, r)
		}
	} else if head == "reset" {
		if r.Method == http.MethodGet {
			u.reset(w, r)
		} else if r.Method == http.MethodPost {
			u.resetFinish(w, r)
		}
	} else {
		// user route not Found, send to homepage
		fmt.Println("Page not found")
		ServePage(w, r, "index.html", nil)
	}
}

// reset route
func (u User) reset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var data = new(struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	})

	// get the  id and token from the URL
	ID, ok := r.URL.Query()["id"]
	Token, cool := r.URL.Query()["token"]

	// if a parameter missing, redirect to Forgot Page
	if !ok || len(ID[0]) < 1 || !cool || len(Token[0]) < 1 {
		alert := Notification{
			Title:     "Notice",
			Message:   "Password reset token missing. Please try again.",
			IsInfo:    false,
			IsSuccess: false,
			IsError:   true,
			IsWarning: false,
		}
		fmt.Printf("Key not found on URL")
		ServePage(w, r, config.Current.ForgotLoginTemplate, CreateViewData(ctx, &alert, nil))
		return
	}
	data.ID = ID[0]
	data.Token = Token[0]
	ServePage(w, r, config.Current.ResetLoginTemplate, CreateViewData(ctx, nil, data))
}

// reset route - final step
func (u User) resetFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	db := ctx.Value(ContextDatabase).(*data.DB)
	isJSON := ctx.Value(ContextContentIsJSON).(bool)

	// check if email is registered
	var data = new(struct {
		ID       int64  `json:"id"`
		Password string `json:"password"`
	})

	if isJSON {
		if err := ParseBody(r.Body, &data); err != nil {
			Respond(w, r, http.StatusBadRequest, err)
			return
		}
	} else {
		// get form values
		var err error
		r.ParseForm()
		data.ID, err = strconv.ParseInt(r.Form.Get("id"), 10, 64)
		if err != nil {
			Respond(w, r, http.StatusBadRequest, err)
			return
		}
		data.Password = r.Form.Get("password")
		fmt.Println(data)

		b, err := bcrypt.GenerateFromPassword([]byte(data.Password), bcrypt.DefaultCost)
		if err != nil {
			if isJSON {
				Respond(w, r, http.StatusInternalServerError, err)
			} else {
				http.Redirect(w, r, config.Current.SignUpErrorRedirect, http.StatusSeeOther)
			}
			return
		}
		// TODO the 2nd instance of data.ID is the account
		err = db.Users.ChangePassword(data.ID, data.ID, string(b))
		if err != nil {
			if isJSON {
				Respond(w, r, http.StatusInternalServerError, err)
			} else {
				fmt.Println(err)
				http.Redirect(w, r, config.Current.SignUpErrorRedirect, http.StatusSeeOther)
			}
			return
		}
		fmt.Println("Password changed!")

		if isJSON {
			// TODO the data.ID is the account
			Respond(w, r, http.StatusCreated, data.ID)
		} else {
			alert := Notification{
				Title:     "Success",
				Message:   "Password sucessfully changed",
				IsSuccess: true,
			}
			SetNotificationCookie(w, alert)
			fmt.Println("Nottificaiton Cookie set")
			http.Redirect(w, r, config.Current.PwdChgSuccessRedirect, http.StatusSeeOther)
		}
	}
}

// forgot route - show the forgot form
func (u User) forgot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ServePage(w, r, config.Current.ForgotLoginTemplate, CreateViewData(ctx, nil, nil))
}

// sendReset is called after the forgot page to set a temporary password and send the
// reset email with link
func (u User) sendReset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	db := ctx.Value(ContextDatabase).(*data.DB)
	isJSON := ctx.Value(ContextContentIsJSON).(bool)

	// check if email is registered
	var data = new(struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	})

	if isJSON {
		if err := ParseBody(r.Body, &data); err != nil {
			Respond(w, r, http.StatusBadRequest, err)
			return
		}
	} else {
		r.ParseForm()
		data.Email = r.Form.Get("email")
	}
	// check if user exists and grab info for user ID
	user, err := db.Users.GetUserByEmail(data.Email)
	if err != nil {
		if isJSON {
			Respond(w, r, http.StatusInternalServerError, err)
		} else {
			alert := Notification{
				Title:   "Notice",
				Message: err.Error(),
				IsError: true,
			}
			ServePage(w, r, config.Current.ForgotLoginTemplate, CreateViewData(ctx, &alert, nil))
		}
		return
	}
	// user exists, generate random password and token password reset token
	data.Password = randStringRunes(7)
	b, err := bcrypt.GenerateFromPassword([]byte(data.Password), bcrypt.DefaultCost)
	fmt.Printf("%s:%s\n)", data.Password, b)
	if err != nil {
		if isJSON {
			Respond(w, r, http.StatusInternalServerError, err)
		} else {
			// TODO: figure out where this should go
			// TODO save an error cookie
			http.Redirect(w, r, config.Current.SignUpErrorRedirect, http.StatusSeeOther)
		}
		return
	}
	// store in pwdreset table
	err = db.Users.StoreTempPassword(user.ID, data.Email, string(b))
	if err != nil {
		if isJSON {
			Respond(w, r, http.StatusInternalServerError, err)
		} else {
			fmt.Println(err)
			// TODO save an error cookie
			http.Redirect(w, r, config.Current.SignUpErrorRedirect, http.StatusSeeOther)
		}
		return
	}
	// send email with link to user
	u.sendForgotEmail(user.ID, data.Email, string(b))
	alert := Notification{
		Title:     "Success",
		Message:   "Password email sent",
		IsSuccess: true,
	}
	SetNotificationCookie(w, alert)
	// TODO: change where this goes?
	http.Redirect(w, r, config.Current.PwdChgSuccessRedirect, http.StatusSeeOther)
}

// signup route
func (u User) signup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := pageData{Title: "Sign Up", Header: "Sign up for an account"}
	// fmt.Printf("%+v\n", data)
	ServePage(w, r, config.Current.SignUpTemplate, CreateViewData(ctx, nil, data))
}

// create is called on Post from signup or from JSON
func (u User) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	db := ctx.Value(ContextDatabase).(*data.DB)
	isJSON := ctx.Value(ContextContentIsJSON).(bool)

	var data = new(struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		First    string `json:"first"`
		Last     string `json:"last"`
	})

	if isJSON {
		if err := ParseBody(r.Body, &data); err != nil {
			Respond(w, r, http.StatusBadRequest, err)
			return
		}
	} else {
		r.ParseForm()
		data.Email = r.Form.Get("email")
		data.Password = r.Form.Get("password")
		data.First = r.Form.Get("first_name")
		data.Last = r.Form.Get("last_name")

	}

	// Check if email already exists and redirect to forgot page if it;s there
	_, err := db.Users.GetUserByEmail(data.Email)
	// user found
	if err == nil {
		if isJSON {
			Respond(w, r, http.StatusInternalServerError, err)
		} else {
			alert := Notification{
				Title:   "Notice!",
				Message: "Email address already registered",
				IsError: true,
			}
			fmt.Printf("Email already exists: %s\n", data.Email)
			ServePage(w, r, config.Current.SignUpTemplate, CreateViewData(ctx, &alert, nil))
		}
		return
	}

	fmt.Println(data)
	if len(data.Password) == 0 {
		data.Password = randStringRunes(7)
		fmt.Println("TODO: remove this, temporary password", data.Password)
	}

	b, err := bcrypt.GenerateFromPassword([]byte(data.Password), bcrypt.DefaultCost)
	if err != nil {
		if isJSON {
			Respond(w, r, http.StatusInternalServerError, err)
		} else {
			http.Redirect(w, r, config.Current.SignUpErrorRedirect, http.StatusSeeOther)
		}
		return
	}

	acct, err := db.Users.SignUp(data.Email, string(b), data.First, data.Last)
	if err != nil {
		if isJSON {
			Respond(w, r, http.StatusInternalServerError, err)
		} else {
			http.Redirect(w, r, config.Current.SignUpErrorRedirect, http.StatusSeeOther)
		}
		return
	}

	if config.Current.SignUpSendEmailValidation {
		u.sendEmail(acct.Email, data.Password)
	}

	if isJSON {
		Respond(w, r, http.StatusCreated, acct)
	} else {
		ck := &http.Cookie{
			Name:  "X-API-KEY",
			Path:  "/",
			Value: acct.Users[0].Token,
		}

		// we set the cookie so they will have their authentication token on the next request.
		http.SetCookie(w, ck)

		http.Redirect(w, r, config.Current.SignUpSuccessRedirect, http.StatusSeeOther)
	}
}

// Send password reset link to user
func (u User) sendForgotEmail(id int64, email, pass string) {
	fmt.Println("TODO update u.sendEmail to use config values")

	//Body:    "<html><body>Use this <a href=localhost:8080/users/reset?id=" + strconv.FormatInt(id, 10) + "&token=" + pass + ">link</a> to reset your password.</html></body>",
	emailInfo := queue.SendEmailParameter{
		From:    config.Current.EmailFrom,
		To:      email,
		Subject: "Password reset link",
		Body:    "localhost:8080/users/reset?id=" + strconv.FormatInt(id, 10) + "&token=" + pass,
	}
	// fmt.Println(emailInfo.Body)
	queue.Enqueue(queue.TaskEmail, emailInfo)
}

func (u User) sendEmail(email, pass string) {
	//TODO: implement this
	fmt.Println("TODO update u.sendEmail to use config values")
	emailInfo := queue.SendEmailParameter{
		From:    config.Current.EmailFrom,
		To:      email,
		Subject: "Welcome to the SAAS",
		Body:    "This is the email body",
	}

	queue.Enqueue(queue.TaskEmail, emailInfo)
	//queue.Enqueue(queue.TaskEmail, queue.SendEmailParameter{})
}

// login presents the login form and calls signin after POST
func (u User) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	//data := pageData{Title: "SignIn", Header: "Sign in to your account"}
	ServePage(w, r, config.Current.SignInTemplate, CreateViewData(ctx, nil, nil))
}

// signin takes user credentials from login form and processes the signin
func (u User) signin(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Users/Signin")
	ctx := r.Context()
	db := ctx.Value(ContextDatabase).(*data.DB)
	isJSON := ctx.Value(ContextContentIsJSON).(bool)

	var data = new(struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	})

	var authKey = new(struct {
		Name  string `json:"name"`
		Token string `json:"token"`
	})

	if isJSON {
		fmt.Println("JSON Login Route")
		b, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			panic(err)
		}
		//Save data into Job struct
		err = json.Unmarshal(b, &data)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		fmt.Println(data)
	} else {
		r.ParseForm()
		data.Email = r.Form.Get("email")
		data.Password = r.Form.Get("password")
	}
	fmt.Println(data)

	// TODO?? Moving this the Users.go
	// UserLogin(email string)

	user, err := db.Users.GetUserByEmail(data.Email)

	// user not found
	if err != nil {
		if isJSON {
			Respond(w, r, http.StatusInternalServerError, err)
		} else {
			alert := Notification{
				Title:     "Notice!",
				Message:   "Username or password incorrect",
				IsInfo:    false,
				IsSuccess: false,
				IsError:   true,
				IsWarning: false,
			}
			fmt.Printf("User not found: %+v\n", CreateViewData(ctx, &alert, data))
			ServePage(w, r, config.Current.SignInTemplate, CreateViewData(ctx, &alert, nil))
		}
		return
	}

	// invalid password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(data.Password)); err != nil {
		alert := Notification{
			Title:     "Notice!",
			Message:   "Username or password incorrect",
			IsInfo:    false,
			IsSuccess: false,
			IsError:   true,
			IsWarning: false,
		}
		fmt.Printf("Password Incorrect: %+v\n", CreateViewData(ctx, &alert, data))
		ServePage(w, r, config.Current.SignInTemplate, CreateViewData(ctx, &alert, nil))
		return
	}
	// log time of last logn
	err = db.Users.UpdateLastLogin(user.ID)

	if isJSON {
		authKey.Name = "X-API-KEY"
		authKey.Token = user.Token
		Respond(w, r, http.StatusOK, authKey)
	} else {
		ck := &http.Cookie{
			Name:  "X-API-KEY",
			Path:  "/",
			Value: user.Token,
		}

		// we set the cookie so they will have their authentication token on the next request.
		http.SetCookie(w, ck)
		//TODO - display a flash welcome cookie?
		http.Redirect(w, r, config.Current.SignInSuccessRedirect, http.StatusSeeOther)
	}
}

func (u User) profile(w http.ResponseWriter, r *http.Request) {

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

func randStringRunes(n int) string {
	letterRunes := []rune("abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ2345679")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}

	return string(b)
}
