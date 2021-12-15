package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
	gpsv2 "nuha.dev/gpstracker/internal/gpsv2/server"
)

type ServiceRegistry struct {
	svcs map[string]service
	*validator.Validate
	db   *pgxpool.Pool
	log  log.Logger
	gsrv *gpsv2.Server
}

type ApiContextKeyType string

type service struct {
	reqType reflect.Type
	resType reflect.Type
	handler reflect.Value
}

func NewServiceRegistry(db *pgxpool.Pool, gsrv *gpsv2.Server) *ServiceRegistry {
	svc := &ServiceRegistry{}
	svc.svcs = make(map[string]service)
	svc.db = db
	svc.Validate = validator.New()
	svc.gsrv = gsrv
	return svc
}

func (sreg *ServiceRegistry) RegisterService() {
	tracker := Tracker{db: sreg.db, reg: sreg}
	sreg.Add("Echo", test_echo, false)
	// sreg.Add("GetRefSnType", tracker.GetRefSnType)
	// sreg.Add("CreateTracker", tracker.CreateTracker)
	sreg.Add("GetTrackers", tracker.GetTrackers, false)
	sreg.Add("SendCommand", tracker.SendCommand, true)
	sreg.Add("EditTrackerSettings", tracker.EditTrackerSettings, true)
	// sreg.Add("GetTrackersStatus", tracker.GetTrackersStatus)
	// sreg.Add("GetTrackerStatusDetail", tracker.GetTrackerStatusDetail)
	// sreg.Add("DeleteTracker", tracker.DeleteTracker)
	// sreg.Add("UpdateTrackerComment", tracker.UpdateTrackerComment)
	// sreg.Add("UpdateTrackerName", tracker.UpdateTrackerName)
	user := User{db: sreg.db}
	sreg.Add("CreateUser", user.CreateUser, true)
	sreg.Add("GetUsers", user.GetUsers, true)
	sreg.Add("SuspendUser", user.SuspendUser, true)
}

func (sreg *ServiceRegistry) Add(tag string, i interface{}) {
	s := service{}
	s.handler = reflect.ValueOf(i)
	if s.handler.Type().NumIn() == 2 {
		s.reqType = nil
		s.resType = s.handler.Type().In(1).Elem()
	} else {
		s.reqType = s.handler.Type().In(1).Elem()
		s.resType = s.handler.Type().In(2).Elem()
	}
	sreg.svcs[tag] = s
}

func (sreg *ServiceRegistry) Call(tag string, w http.ResponseWriter, r *http.Request) {
	svc, ok := sreg.svcs[tag]
	if !ok {
		http.Error(w, fmt.Sprintf("function \"%s\" not found", tag), http.StatusNotFound)
		return
	}
	var ctx context.Context
	var err error

	sid, err := r.Cookie("GSESS")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	var user_id uint64
	var valid_until time.Time
	var init_password bool
	row := sreg.db.QueryRow(r.Context(), `SELECT user_id,valid_until,init_password
	FROM session WHERE session.session_id = $1`, sid.Value)
	err = row.Scan(&user_id, &valid_until, &init_password)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		} else {
			sreg.log.Error().Err(err).Msg("")
			panic(err)
		}
	} else {
		if time.Now().After(valid_until) {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			sreg.log.Warn().Time("valid_until", valid_until.UTC()).Msg("expired session")
			return
		} else if init_password {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			sreg.log.Warn().Uint64("user_id", user_id).Msg("session only valid for init_password")
			return
		}
	}
	ctx = context.WithValue(r.Context(), ApiContextKeyType("user-session"), user_id)

	response := reflect.New(svc.resType)
	if svc.reqType != nil {
		request := reflect.New(svc.reqType)
		err := json.NewDecoder(r.Body).Decode(request.Interface())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = sreg.Struct(request.Interface())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		svc.handler.Call([]reflect.Value{reflect.ValueOf(ctx), request, response})
	} else {
		svc.handler.Call([]reflect.Value{reflect.ValueOf(ctx), response})
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response.Interface())
	if err != nil {
		sreg.log.Error().Err(err).Msg("")
	}
}



type Echo struct {
	Message string `json:"message"`
}

func test_echo(ctx context.Context, req *Echo, res *Echo) {
	res.Message = req.Message
}
