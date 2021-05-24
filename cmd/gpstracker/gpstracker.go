package main

import (
	"context"
	"sync"

	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"nuha.dev/gpstracker/internal/login"
	"nuha.dev/gpstracker/internal/service"
	"nuha.dev/gpstracker/internal/webstream"
)

func main() {

	viper.SetDefault("db_url", "postgresql://postgres:postgres@localhost/gpsv2")
	pool, err := pgxpool.Connect(context.Background(), viper.GetString("db_url"))
	if err != nil {
		panic(err.Error())
	}
	err = pool.Ping(context.Background())
	if err != nil {
		log.Err(err).Msg("Unable to connect to database")
	}
	log.Info().Msgf("Connected to database at %s", pool.Config().ConnString())
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		// AllowedOrigins:   []string{"https://foo.com"}, // Use this to allow specific origin hosts
		AllowedOrigins: []string{"https://*", "http://*"},
		// AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-XSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	svc := service.NewServiceRegistry(pool)
	svc.RegisterService()
	login_handler := login.NewLoginHandler(pool)

	r.Post("/func/{name}", func(w http.ResponseWriter, r *http.Request) {
		f := chi.URLParam(r, "name")
		if f == "InitPassword" {
			login_handler.InitPassword(w, r)
		} else if f == "GetWsToken" {
			login_handler.GetWsToken(w, r)
		} else {
			svc.Call(f, w, r)
		}
	})

	r.Post("/func/login", login_handler.Login)

	s1 := &http.Server{
		Addr:           ":3333",
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	ws_server := webstream.NewWebstream(3334)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_ = s1.ListenAndServe()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		ws_server.Run()
		wg.Done()
	}()
	wg.Wait()
}

func xsrf_verify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hsrf := r.Header.Get("X-XSRF-TOKEN")
		ct, err1 := r.Cookie("GSURF")
		_, err2 := r.Cookie("GSESS")
		if err1 != nil || err2 != nil || hsrf != ct.Value {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
