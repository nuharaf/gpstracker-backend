package service

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
	"github.com/rs/zerolog"
)

type ServiceRegistry struct {
	svcs map[string]service
	*validator.Validate
	db     *pgxpool.Pool
	logger zerolog.Logger
}

type userSessionKeyType struct{}

var userSessionKey userSessionKeyType

type service struct {
	reqType reflect.Type
	resType reflect.Type
	handler reflect.Value
}

func NewServiceRegistry(db *pgxpool.Pool) *ServiceRegistry {
	svc := &ServiceRegistry{}
	svc.svcs = make(map[string]service)
	svc.db = db
	svc.Validate = validator.New()
	return svc
}

func (sreg *ServiceRegistry) RegisterService() {
	tracker := Tracker{db: sreg.db}
	sreg.Add("Echo", test_echo)
	sreg.Add("CreateTracker", tracker.CreateTracker)
	sreg.Add("GetTrackers", tracker.GetTrackers)
	sreg.Add("DeleteTracker", tracker.DeleteTracker)
	sreg.Add("UpdateTrackerComment", tracker.UpdateTrackerComment)
	sreg.Add("UpdateTrackerName", tracker.UpdateTrackerName)
	user := User{db: sreg.db}
	sreg.Add("CreateUser", user.CreateUser)
	sreg.Add("GetUsers", user.GetUsers)
	sreg.Add("SuspendUser", user.SuspendUser)
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
	sid, err := r.Cookie("GSESS")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	var user_id, role string
	var suspended, init_done bool
	var valid_until time.Time
	row := sreg.db.QueryRow(r.Context(), `select "user".id,"user".suspended,"user".init_done,"user".role,session.valid_until from "user" inner join session on session.user_id = "user".id where session.session_id = $1`, sid.Value)
	err = row.Scan(&user_id, &suspended, &init_done, &role, &valid_until)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		} else {
			panic(err)
		}
	} else {
		if time.Now().After(valid_until) {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
	}

	ctx := context.WithValue(r.Context(), userSessionKey, user_id)

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
	err = json.NewEncoder(w).Encode(response.Interface())
	if err != nil {
		sreg.logger.Err(err).Msg("")
	}
}

type BasicResponse struct {
	Status int `json:"status"`
}

type Echo struct {
	Message string `json:"message"`
}

func test_echo(ctx context.Context, req *Echo, res *Echo) {
	res.Message = req.Message
}
