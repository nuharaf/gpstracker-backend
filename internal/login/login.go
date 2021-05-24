package login

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"nuha.dev/gpstracker/internal/util"
)

type LoginHandler struct {
	db *pgxpool.Pool
	*validator.Validate
}

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type ChangePwdRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required"`
}

type ChangePwdResponse struct {
	Status int `json:"status"`
}

type LoginResponse struct {
	Status int `json:"status"`
}

type GetWsToken struct {
	WsToken string `json:"ws_token"`
}

type session struct {
	session_id string
	ws_token   string
	csrf_token string
}

func NewLoginHandler(db *pgxpool.Pool) *LoginHandler {
	return &LoginHandler{db: db, Validate: validator.New()}
}

var errUserSuspended = errors.New("user suspended")
var errUserInvalid = errors.New("user invalid")

func (l LoginHandler) login(ctx context.Context, username, password string) (*session, bool, error) {

	sqlStmt := `SELECT id,"password",init_done,suspended,session_length_sec FROM "user" WHERE username = $1`
	row := l.db.QueryRow(ctx, sqlStmt, username)
	uuid := pgtype.UUID{}
	var hashpwd string
	var sess_length int
	var init_done bool
	var suspended bool
	err := row.Scan(&uuid, &hashpwd, &init_done, &suspended, &sess_length)
	if err == pgx.ErrNoRows {
		return nil, false, errUserInvalid
	} else if err != nil {
		panic(err)
	}
	if !suspended {
		err = bcrypt.CompareHashAndPassword([]byte(hashpwd), []byte(password))
		if err != nil {
			return nil, false, errUserInvalid
		} else {
			session_id := util.GenRandomString(uuid.Bytes[:], 24)
			csrf_token := util.GenRandomString(uuid.Bytes[:], 24)
			ws_token := util.GenRandomString(uuid.Bytes[:], 24)

			sqlStmt := `INSERT INTO session (session_id,user_id,csrf_token,ws_token,created_at,valid_until) 
			VALUES($1,$2,$3,$4,now(),$5)`
			_, err := l.db.Exec(ctx, sqlStmt, session_id, &uuid, csrf_token, ws_token,
				time.Now().Add(time.Duration(sess_length)*time.Second))
			if err != nil {
				panic(err)
			}
			return &session{session_id: session_id, ws_token: ws_token, csrf_token: csrf_token}, init_done, nil
		}
	} else {
		return nil, init_done, errUserSuspended
	}
}

func login_success(w http.ResponseWriter, sessionId, csrfToken, wsToken string, init_done bool) {
	http.SetCookie(w, &http.Cookie{
		Domain:   "localhost",
		HttpOnly: true,
		Name:     "GSESS",
		Value:    sessionId,
		Path:     "/func",
		Expires:  time.Now().Add(time.Hour),
	})

	http.SetCookie(w, &http.Cookie{
		Domain:  "localhost",
		Name:    "GSURF",
		Value:   csrfToken,
		Path:    "/func",
		Expires: time.Now().Add(time.Hour),
	})

	res := LoginResponse{}
	if init_done {
		res.Status = 0
	} else {
		res.Status = 1
	}
	util.JsonWrite(w, res)
}

func login_wrong(w http.ResponseWriter) {
	res := LoginResponse{Status: -1}
	util.JsonWrite(w, res)
}

func (l *LoginHandler) Login(w http.ResponseWriter, r *http.Request) {
	req_body := LoginRequest{}
	err := json.NewDecoder(r.Body).Decode(&req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	err = l.Validate.Struct(req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	sess, status, err := l.login(r.Context(), req_body.Username, req_body.Password)
	if err == nil {
		login_success(w, sess.session_id, sess.csrf_token, sess.ws_token, status)
	} else {
		login_wrong(w)
	}
}

func (l *LoginHandler) init_password(ctx context.Context, session_id string, req *ChangePwdRequest, w http.ResponseWriter) {
	var user_passwd string
	user_id := make([]byte, 16)
	tx, err := l.db.Begin(ctx)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	row := tx.QueryRow(ctx, `SELECT "user".id,"user".password 
	FROM "user" INNER JOIN session ON session.user_id = "user".id 
	WHERE session.session_id = $1 and session.valid_until > now() 
	and not "user".suspended
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
		util.JsonWrite(w, ChangePwdResponse{Status: -1})
		return
	}

	//change password
	_, err = tx.Exec(ctx, `UPDATE "user" SET password = $1,init_done = true,updated_at = now() WHERE id = $2`,
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

	util.JsonWrite(w, ChangePwdResponse{Status: 0})
	err = tx.Commit(ctx)
	if err != nil {
		panic(err)
	}
}

func (l *LoginHandler) InitPassword(w http.ResponseWriter, r *http.Request) {
	ck, _ := r.Cookie("GSESS")
	req_body := ChangePwdRequest{}
	err := json.NewDecoder(r.Body).Decode(&req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	err = l.Validate.Struct(req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	l.init_password(r.Context(), ck.Value, &req_body, w)
}

func (l *LoginHandler) GetWsToken(w http.ResponseWriter, r *http.Request) {
	var ws_token string
	ck, _ := r.Cookie("GSESS")
	row := l.db.QueryRow(r.Context(), `SELECT session.ws_token
	FROM "user" INNER JOIN session ON session.user_id = "user".id 
	WHERE session.session_id = $1 
	and session.valid_until < now()  
	and session.status = 'enabled' 
	and not "user".suspended
	and FOR UPDATE OF "user" NOWAIT`, ck.Value)
	err := row.Scan(&ws_token)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		} else {
			panic(err)
		}
	}
	util.JsonWrite(w, GetWsToken{WsToken: ws_token})
}
