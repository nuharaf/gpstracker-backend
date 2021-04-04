package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4/pgxpool"
)

type ServiceRegistry struct {
	svcs map[string]service
	*validator.Validate
	db *pgxpool.Pool
}

type service struct {
	reqType reflect.Type
	resType reflect.Type
	handler reflect.Value
}

var errBadRequest = errors.New("Bad request")

func New(db *pgxpool.Pool) *ServiceRegistry {
	svc := &ServiceRegistry{}
	svc.db = db
	return svc
}

func (sreg *ServiceRegistry) Add(tag string, i interface{}) {
	s := service{}
	s.handler = reflect.ValueOf(i)
	if s.handler.Type().NumIn() == 1 {
		s.reqType = nil
		s.resType = s.handler.Type().In(0).Elem()
	} else {
		s.reqType = s.handler.Type().In(0).Elem()
		s.resType = s.handler.Type().In(1).Elem()
	}
	sreg.svcs[tag] = s
}

func (sreg *ServiceRegistry) Call(tag string, w http.ResponseWriter, r io.Reader) {
	svc := sreg.svcs[tag]
	response := reflect.New(svc.resType)
	var svc_result []reflect.Value
	if svc.reqType != nil {
		request := reflect.New(svc.reqType)
		err := json.NewDecoder(r).Decode(request.Interface())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		err = sreg.Struct(request.Interface())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		svc_result = svc.handler.Call([]reflect.Value{request, response})
	} else {
		svc_result = svc.handler.Call([]reflect.Value{})
	}
	if len(svc_result) == 1 {
		err := svc_result[0].Interface().(error)
		if errors.Is(err, errBadRequest) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	json.NewEncoder(w).Encode(response)
}

type BasicResponse struct {
	Status int `json:"status"`
}
