package webapp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/gpsv2/server"
	"nuha.dev/gpstracker/internal/webapp/tracker"
)

type ApiConfig struct {
	ListenAddr   string
	VerifyCSRF   bool
	CookieDomain string
	// EnableMasterKey bool
}

type Api struct {
	r      chi.Router
	s      *http.Server
	config *ApiConfig
	log    log.Logger
	db     *pgxpool.Pool
	vld    *validator.Validate
}

func NewApi(db *pgxpool.Pool, gps *server.Server, config *ApiConfig) *Api {
	api := &Api{config: config}
	api.db = db
	api.log = log.DefaultLogger
	api.log.Context = log.NewContext(nil).Str("module", "api-server").Value()
	api.vld = validator.New()
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
	r.Use(middleware.Recoverer)
	// r.Use(api.recover)
	disp := NewDispatcher(db)
	tracker_api := tracker.NewTrackerApi(db, gps)
	disp.Add("GetTrackers", tracker_api.GetTrackers, "tracker-monitor")
	disp.Add("GetTrackerDetail", tracker_api.GetTrackerDetail, "tracker-monitor")
	disp.Add("GetTrackerEvent", tracker_api.GetTrackerEvent, "tracker-monitor")
	disp.Add("GetGT06CmdHistory", tracker_api.GetGT06CmdHistory, "tracker-monitor")
	disp.Add("GetTrackerCurrentConnInfo", tracker_api.GetTrackerCurrentConnInfo, "tracker-monitor")
	disp.AddRaw("GetTrackerLocationHistory", tracker_api.GetTrackerLocationHistory, "tracker-monitor")

	disp.Add("SendCommand2", tracker_api.SendCommand2, "tracker-admin")
	disp.Add("EditTrackerSettings", tracker_api.EditTrackerSettings, "tracker-admin")
	disp.Add("SetTrackerName", tracker_api.SetTrackerName, "tracker-admin")
	disp.Add("PurgeTracker", tracker_api.PurgeTracker, "tracker-admin")
	disp.Add("CreateWsToken", tracker_api.CreateWsToken, "tracker-monitor")
	disp.Add("GetWsToken", tracker_api.GetWsToken, "tracker-monitor")

	r.Post("/func/login", func(w http.ResponseWriter, r *http.Request) {
		api.Login(w, r)
	})
	r.Post("/func/logout", func(w http.ResponseWriter, r *http.Request) {
		api.Logout(w, r)
	})
	r.Post("/func/sess_check", func(w http.ResponseWriter, r *http.Request) {
		api.SessionCheck(w, r)
	})
	var final_router chi.Router
	if config.VerifyCSRF {
		final_router = r.With(xsrf_verify)
	} else {
		final_router = r
	}
	final_router.Post("/func/{name}", func(w http.ResponseWriter, r *http.Request) {
		f := chi.URLParam(r, "name")
		if f == "ChangePassword" {
			api.ChangePassword(w, r)
		} else {
			disp.Call(f, w, r)
		}

	})

	api.r = final_router
	s := &http.Server{
		Addr:           api.config.ListenAddr,
		Handler:        api.r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	api.s = s

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

// func (api *Api) recover(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		defer func() {
// 			if rvr := recover(); rvr != nil {
// 				err, ok := rvr.(error)
// 				if ok {
// 					// api.log.Error().Err(err).Msg("error throwed")
// 					panic(err)

// 				} else {
// 					panic(rvr)
// 				}
// 			}
// 		}()
// 	})
// }
