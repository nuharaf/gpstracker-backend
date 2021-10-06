package web

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	gps "nuha.dev/gpstracker/internal/gps/serverimpl"
	"nuha.dev/gpstracker/internal/web/login"
	"nuha.dev/gpstracker/internal/web/service"
)

type ApiConfig struct {
	ListenAddr   string
	VerifyCSRF   bool
	CookieDomain string
}

type Api struct {
	r      chi.Router
	s      *http.Server
	config *ApiConfig
	log    zerolog.Logger
}

func NewApi(db *pgxpool.Pool, gsrv *gps.Server, config *ApiConfig) *Api {
	log.With().Str("module", "api").Logger()
	api := &Api{config: config}
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
	svc := service.NewServiceRegistry(db, gsrv)
	svc.RegisterService()
	login_handler := login.NewLoginHandler(db, config.CookieDomain)
	r.Post("/func/login", login_handler.Login)
	r.With(xsrf_verify).Post("/func/{name}", func(w http.ResponseWriter, r *http.Request) {
		f := chi.URLParam(r, "name")
		if f == "InitPassword" {
			login_handler.InitPassword(w, r)
		} else if f == "GetWsToken" {
			login_handler.GetWsToken(w, r)
		} else {
			svc.Call(f, w, r)
		}
	})

	api.r = r
	s := &http.Server{
		Addr:           api.config.ListenAddr,
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	api.s = s
	return api
}

func (api *Api) Run() {
	err := api.s.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

func xsrf_verify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hsrf := r.Header.Get("X-XSRF-TOKEN")
		ct, err1 := r.Cookie("GSURF")
		var cookie_token string
		if err1 == nil {
			cookie_token = ct.Value
		} else {
			cookie_token = ""
		}
		if err1 != nil || hsrf != cookie_token {
			log.Debug().Err(err1).Str("header_token", hsrf).Str("cookie_token", cookie_token).Msg("mismatched csrf token")
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
