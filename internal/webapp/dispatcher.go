package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/webapp/common"

	"github.com/go-playground/validator/v10"
)

type Dispatcher struct {
	funcs     map[string]_function
	validator *validator.Validate
	log       log.Logger
	db        *pgxpool.Pool
}

type _function struct {
	reqType reflect.Type
	resType reflect.Type
	handler reflect.Value
	role    string
}

func has_role(role string, target string) bool {
	if target == role {
		return true
	} else if target == "tracker-monitor" {
		return true
	} else {
		return false
	}
}

func NewDispatcher(db *pgxpool.Pool) *Dispatcher {

	d := &Dispatcher{}
	d.funcs = make(map[string]_function)
	d.validator = validator.New()
	d.db = db
	return d
}

func (disp *Dispatcher) Call(funcname string, w http.ResponseWriter, r *http.Request) {
	sid, err := r.Cookie("GSESS")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	user_session := disp.session_check(r.Context(), sid.Value)
	if user_session == nil || user_session.RequireChangePassword || time.Now().After(user_session.ValidUntil) {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	_func, ok := disp.funcs[funcname]
	if !ok {
		http.Error(w, fmt.Sprintf("function \"%s\" not found", funcname), http.StatusNotFound)
		return
	}
	if !(has_role(user_session.Roles, _func.role)) {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	disp.call(_func, user_session, r, w)

}

func (disp *Dispatcher) session_check(ctx context.Context, session_id string) *common.UserSessionAtrribute {
	select_sql := `SELECT "user".require_change_pwd,"user".role,session.valid_until FROM "user" INNER JOIN session ON "user".id = session.user_id WHERE session.session_id = $1`
	var require_change_pwd bool
	var role string
	var valid_until time.Time
	err := disp.db.QueryRow(ctx, select_sql, session_id).Scan(&require_change_pwd, &role, &valid_until)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		} else {
			panic(err)
		}
	}
	u := &common.UserSessionAtrribute{}
	u.RequireChangePassword = require_change_pwd
	u.Roles = role
	u.ValidUntil = valid_until
	u.SessionId = session_id
	return u

}

func (disp *Dispatcher) call(_func _function, user_session *common.UserSessionAtrribute, r *http.Request, w http.ResponseWriter) {
	var err error
	response := reflect.New(_func.resType)
	var err_ref []reflect.Value
	_ctx := context.WithValue(r.Context(), common.ApiContextKeyType("session_attribute"), user_session)
	if _func.reqType != nil {
		request := reflect.New(_func.reqType)
		err := json.NewDecoder(r.Body).Decode(request.Interface())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = disp.validator.Struct(request.Interface())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err_ref = _func.handler.Call([]reflect.Value{reflect.ValueOf(_ctx), request, response})
	} else {
		err_ref = _func.handler.Call([]reflect.Value{reflect.ValueOf(_ctx), response})
	}
	if !err_ref[0].IsNil() {
		panic(err_ref[0].Interface())
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response.Interface())
	if err != nil {
		disp.log.Error().Err(err).Msg("")
	}
}

func (disp *Dispatcher) Add(funcname string, f interface{}, role string) {
	s := _function{}
	s.handler = reflect.ValueOf(f)
	if s.handler.Type().NumIn() == 2 {
		s.reqType = nil
		s.resType = s.handler.Type().In(1).Elem()
	} else {
		s.reqType = s.handler.Type().In(1).Elem()
		s.resType = s.handler.Type().In(2).Elem()
	}
	s.role = role
	disp.funcs[funcname] = s
}
