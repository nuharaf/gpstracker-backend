package webapp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
	gpsv2 "nuha.dev/gpstracker/internal/gpsv2/server"
	"nuha.dev/gpstracker/internal/web/login"
	"nuha.dev/gpstracker/internal/web/service"
)

type ApiConfig struct {
	ListenAddr   string
	VerifyCSRF   bool
	CookieDomain string
	// EnableMasterKey bool
}

type Api struct {
	r          chi.Router
	s          *http.Server
	config     *ApiConfig
	log        log.Logger
	master_key string
}

func NewApi(db *pgxpool.Pool, gsrv *gpsv2.Server, config *ApiConfig) *Api {
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
	// if config.EnableMasterKey {
	// 	// api.master_key = util.GenRandomString([]byte{}, 6)
	// 	api.master_key = "secretkey"
	// 	api.log.Info().Str("master-key", api.master_key).Msg("")
	// }
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	svc := service.NewServiceRegistry(db, gsrv)
	svc.RegisterService()
	login_handler := login.NewLoginHandler(db, config.CookieDomain)
	r.Post("/func/login", login_handler.Login)
	var final_router chi.Router
	if config.VerifyCSRF {
		final_router = r.With(xsrf_verify)
	} else {
		final_router = r
	}
	// if config.EnableMasterKey {
	// 	final_router = final_router.With(api.master_key_verify)
	// }
	final_router.Post("/func/{name}", func(w http.ResponseWriter, r *http.Request) {
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
	api.log = log.DefaultLogger
	api.log.Context = log.NewContext(nil).Str("module", "api-server").Value()
	return api
}

func (api *Api) Run() {
	api.log.Info().Msgf("starting api-server on : %s", api.s.Addr)
	err := api.s.ListenAndServe()
	if err != nil {
		api.log.Error().Err(err).Msg("")
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

// func (api *Api) master_key_verify(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		key := r.Header.Get("X-MASTER-KEY")

// 		if key == api.master_key {
// 			r = r.WithContext(context.WithValue(r.Context(), service.ApiContextKeyType("master-key-flag"), true))
// 		}
// 		next.ServeHTTP(w, r)
// 	})
// }
