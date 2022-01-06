package webapp

import (
	"encoding/json"
	"net/http"

	"nuha.dev/gpstracker/internal/util"
)

type sessionCheckRequest struct {
	CsrfToken string `json:"csrf_token" validate:"required"`
}

type sessionCheckResponse struct {
	Status bool `json:"status"`
}

func (api *Api) SessionCheck(w http.ResponseWriter, r *http.Request) {
	req_body := sessionCheckRequest{}
	res_body := sessionCheckResponse{}
	err := json.NewDecoder(r.Body).Decode(&req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = api.vld.Struct(req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ct, err := r.Cookie("GSURF")
	if err != nil {
		res_body.Status = false
		util.JsonWrite(w, res_body)
		return
	}

	if ct.Value != req_body.CsrfToken {
		res_body.Status = false
		util.JsonWrite(w, res_body)
		return
	}

	ct, err = r.Cookie("GSESS")
	if err != nil {
		res_body.Status = false
		util.JsonWrite(w, res_body)
		return
	}

	select_sql := `SELECT session.user_id FROM session WHERE session.session_id = $1 AND session.valid_until > now()`
	var user_id uint64
	err = api.db.QueryRow(r.Context(), select_sql, ct.Value).Scan(&user_id)
	if err != nil {
		res_body.Status = false
		util.JsonWrite(w, res_body)
		return
	} else {
		res_body.Status = true
		util.JsonWrite(w, res_body)
		return
	}
}
