package main

import (
	"context"

	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/spf13/viper"
	"nuha.dev/gpstracker/internal/service"
)

func main() {

	viper.SetDefault("db_url", "postgresql://postgres:postgres@localhost/gpsv2")
	pool, err := pgxpool.Connect(context.Background(), viper.GetString("db_url"))
	if err != nil {
		panic(err.Error())
	}
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	svc := service.New(pool)

	r.Post("/func/{name}", func(w http.ResponseWriter, r *http.Request) {
		svc.Call(chi.URLParam(r, "name"), w, r.Body)
		// case "CreateUser":
		// 	req := service.CreateUserRequest{}
		// 	res := service.BasicResponse{}
		// 	json.NewDecoder(r.Body).Decode(&req)
		// 	validate.Struct(req)
		// 	svc.CreateUser(&req, &res)
		// 	util.JsonWrite(w, res)
		// case "GetUsers":
		// 	res := service.GetUserResponse{}
		// 	svc.GetUsers(&res)
		// 	util.JsonWrite(w, res)
		// }
	})

	// r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
	// 	var req map[string]interface{}
	// 	res := make(map[string]interface{})
	// 	err := json.NewDecoder(r.Body).Decode(&req)
	// 	if err != nil {
	// 		http.Error(w, err.Error(), http.StatusBadRequest)
	// 	}
	// 	var username = req["username"].(string)
	// 	var password = req["password"].(string)
	// 	_user, ok := u.GetUserByCredential(username, password)
	// 	if ok && _user.Status == user.Enabled {
	// 		sessionId := util.GenRandomString(24)
	// 		csrfToken := util.GenRandomString(24)
	// 		wsToken := util.GenRandomString(24)
	// 		u.CreateSession(_user.Id, sessionId, csrfToken, wsToken)
	// 		http.SetCookie(w, &http.Cookie{
	// 			Secure:   true,
	// 			HttpOnly: true,
	// 			Name:     "GSESS",
	// 			Value:    sessionId,
	// 			Path:     "/func",
	// 			Expires:  time.Now().Add(time.Hour),
	// 		})

	// 		http.SetCookie(w, &http.Cookie{
	// 			Secure:   true,
	// 			HttpOnly: true,
	// 			Name:     "GSURF",
	// 			Value:    csrfToken,
	// 			Path:     "/func",
	// 			Expires:  time.Now().Add(time.Hour),
	// 		})
	// 		res["ok"] = true
	// 		res["csrf_token"] = csrfToken
	// 		res["ws_token"] = wsToken
	// 		util.JsonWrite(w, res)
	// 	} else {
	// 		res["ok"] = false
	// 		util.JsonWrite(w, res)
	// 	}

	// })

	s1 := &http.Server{
		Addr:           ":3333",
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	err = s1.ListenAndServe()
	if err != nil {
		panic(err.Error())
	}

	h2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

	})
	s2 := &http.Server{
		Addr:           ":3334",
		Handler:        http.HandlerFunc(h2),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	err = s2.ListenAndServe()
	if err != nil {
		panic(err.Error())
	}

}
