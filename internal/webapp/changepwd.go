package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v4"
	"golang.org/x/crypto/bcrypt"
	"nuha.dev/gpstracker/internal/util"
)

type changePwdRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required"`
}

type changePwdResponse struct {
	Status int `json:"status"`
}

func (api *Api) change_password(ctx context.Context, session_id string, req *changePwdRequest, w http.ResponseWriter) {
	var user_passwd string
	var user_id uint64
	tx, err := api.db.Begin(ctx)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	row := tx.QueryRow(ctx, `SELECT "user".id,"user".password 
	FROM "user" INNER JOIN session ON session.user_id = "user".id 
	WHERE session.session_id = $1 and session.valid_until > now() 
	and not "user".suspend_login
	FOR UPDATE OF "user" NOWAIT`, session_id)
	err = row.Scan(&user_id, &user_passwd)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		} else {
			panic(err)
		}
	}

	//verify current password
	err = bcrypt.CompareHashAndPassword([]byte(user_passwd), []byte(req.CurrentPassword))
	if err != nil {
		util.JsonWrite(w, changePwdResponse{Status: -1})
		return
	}

	//change password, set init_done flag
	_, err = tx.Exec(ctx, `UPDATE "user" SET password = $1,require_change_pwd = false WHERE id = $2`,
		util.CryptPwd(req.NewPassword), user_id)
	if err != nil {
		panic(err)
	}

	//invalidate all session
	_, err = tx.Exec(ctx, `DELETE FROM session WHERE user_id = $1`, user_id)
	if err != nil {
		panic(err)
	}

	http.SetCookie(w, &http.Cookie{
		Secure:   true,
		HttpOnly: true,
		Name:     "GSESS",
		Value:    "",
		Path:     "/func",
		Expires:  time.Unix(0, 0),
	})

	util.JsonWrite(w, changePwdResponse{Status: 0})
	err = tx.Commit(ctx)
	if err != nil {
		panic(err)
	}
}

func (api *Api) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var err error
	ck, err := r.Cookie("GSESS")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req_body := changePwdRequest{}
	err = json.NewDecoder(r.Body).Decode(&req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = api.vld.Struct(req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	api.change_password(r.Context(), ck.Value, &req_body, w)
}
