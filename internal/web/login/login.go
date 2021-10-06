package login

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"nuha.dev/gpstracker/internal/util"
)

type LoginHandler struct {
	db *pgxpool.Pool
	*validator.Validate
	cookieDomain string
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
	Status       int `json:"status"`
	*UserInfo    `json:"user_info,omitempty"`
	*SessionInfo `json:"session_info,omitempty"`
}

type UserInfo struct {
	InitDone      bool `json:"init_done"`
	SessionLength int  `json:"session_length"`
}

type SessionInfo struct {
	CsrfToken string `json:"csrf_token"`
	WsToken   string `json:"ws_token"`
}

type GetWsToken struct {
	WsToken string `json:"ws_token"`
}

type session struct {
	session_id string
	ws_token   string
	csrf_token string
}

func NewLoginHandler(db *pgxpool.Pool, cookieDomain string) *LoginHandler {
	return &LoginHandler{db: db, Validate: validator.New(), cookieDomain: cookieDomain}
}

var errUserSuspended = errors.New("user suspended")
var errUserInvalid = errors.New("user invalid")

func (l LoginHandler) login(ctx context.Context, username, password string) (*session, *UserInfo, error) {

	sqlStmt := `SELECT id,"password",init_done,suspended,session_length_sec FROM "user" WHERE username = $1`
	row := l.db.QueryRow(ctx, sqlStmt, username)
	var id uint64
	var hashpwd string
	var sess_length int
	user_info := UserInfo{}
	var init_done bool
	var suspended bool
	err := row.Scan(&id, &hashpwd, &init_done, &suspended, &sess_length)
	if err == pgx.ErrNoRows {
		return nil, nil, errUserInvalid
	} else if err != nil {
		panic(err)
	}
	if !suspended {
		err = bcrypt.CompareHashAndPassword([]byte(hashpwd), []byte(password))
		if err != nil {
			return nil, nil, errUserInvalid
		} else {
			var prefix [4]byte
			binary.BigEndian.PutUint32(prefix[:], crc32.ChecksumIEEE([]byte(username)))
			session_id := util.GenRandomString(prefix[:], 24)
			csrf_token := util.GenRandomString(prefix[:], 24)
			ws_token := util.GenRandomString(prefix[:], 24)

			sqlStmt := `INSERT INTO session (session_id,user_id,csrf_token,ws_token,created_at,valid_until) 
			VALUES($1,$2,$3,$4,now(),$5)`
			_, err := l.db.Exec(ctx, sqlStmt, session_id, id, csrf_token, ws_token,
				time.Now().Add(time.Duration(sess_length)*time.Second))
			if err != nil {
				panic(err)
			}
			user_info.InitDone = init_done
			user_info.SessionLength = sess_length
			return &session{session_id: session_id, ws_token: ws_token, csrf_token: csrf_token}, &user_info, nil
		}
	} else {
		return nil, nil, errUserSuspended
	}
}

func login_success_setCookie(w http.ResponseWriter, sessionId, csrfToken, domain string) {
	http.SetCookie(w, &http.Cookie{
		Domain:   domain,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Name:     "GSESS",
		Value:    sessionId,
		Path:     "/func",
		Expires:  time.Now().Add(time.Hour),
	})

	http.SetCookie(w, &http.Cookie{
		Domain:   domain,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Name:     "GSURF",
		Value:    csrfToken,
		Path:     "/func",
		Expires:  time.Now().Add(time.Hour),
	})
}

// func login_success(w http.ResponseWriter, , sessionId, csrfToken string, init_done bool) {

// 	res := LoginResponse{}
// 	if init_done {
// 		res.Status = 0
// 	} else {
// 		res.Status = 1
// 	}
// 	util.JsonWrite(w, res)
// }

// func login_wrong(w http.ResponseWriter) {
// 	res := LoginResponse{Status: -1}
// 	util.JsonWrite(w, res)
// }

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
	sess, user_info, err := l.login(r.Context(), req_body.Username, req_body.Password)
	if err == nil {
		login_success_setCookie(w, sess.session_id, sess.csrf_token, l.cookieDomain)
		res := LoginResponse{Status: 0, UserInfo: user_info, SessionInfo: &SessionInfo{CsrfToken: sess.csrf_token, WsToken: sess.ws_token}}
		util.JsonWrite(w, res)
		// login_success(w, l.cookieDomain, sess.session_id, sess.csrf_token, sess.ws_token, status)
	} else {
		res := LoginResponse{Status: -1}
		util.JsonWrite(w, res)
		// login_wrong(w)
	}
}

func (l *LoginHandler) init_password(ctx context.Context, session_id string, req *ChangePwdRequest, w http.ResponseWriter) {
	var user_passwd string
	var user_id uint64
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

	//change password, set init_done flag
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
