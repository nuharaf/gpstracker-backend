package webapp

import (
	"context"
	"encoding/binary"
	"encoding/json"
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
	Status                int       `json:"status"`
	RequireChangePassword bool      `json:"require_change_pwd"`
	CsrfToken             string    `json:"csrf_token"`
	ValidUntil            time.Time `json:"valid_until"`
	Role                  string    `json:"role"`
}

func NewLoginHandler(db *pgxpool.Pool, cookieDomain string) *LoginHandler {
	return &LoginHandler{db: db, Validate: validator.New(), cookieDomain: cookieDomain}
}

func (l LoginHandler) login(ctx context.Context, username, password string) (*LoginResponse, string, error) {

	sqlStmt := `SELECT id,"password",require_change_pwd,suspend_login,session_length_sec,role FROM "user" WHERE username = $1`
	row := l.db.QueryRow(ctx, sqlStmt, username)
	var id uint64
	var hashpwd string
	var sess_length int
	var require_change_pwd bool
	var suspend_login bool
	var role string
	err := row.Scan(&id, &hashpwd, &require_change_pwd, &suspend_login, &sess_length, &role)
	if err == pgx.ErrNoRows {
		return &LoginResponse{Status: -1}, "", nil
	} else if err != nil {
		return nil, "", err
	}
	if !suspend_login {
		err = bcrypt.CompareHashAndPassword([]byte(hashpwd), []byte(password))
		if err != nil {
			return &LoginResponse{Status: -1}, "", nil
		} else {
			var prefix [4]byte
			binary.BigEndian.PutUint32(prefix[:], crc32.ChecksumIEEE([]byte(username)))
			session_id := util.GenRandomString(prefix[:], 24)
			csrf_token := util.GenRandomString(prefix[:], 24)

			sqlStmt := `INSERT INTO session (session_id,user_id,csrf_token,valid_until) 
			VALUES($1,$2,$3,$4)`
			valid_until := time.Now().Add(time.Duration(sess_length) * time.Second)
			_, err := l.db.Exec(ctx, sqlStmt, session_id, id, csrf_token,
				valid_until)
			if err != nil {
				return nil, "", err
			}
			return &LoginResponse{Status: 0, RequireChangePassword: require_change_pwd, CsrfToken: csrf_token, ValidUntil: valid_until, Role: role}, session_id, nil
		}
	} else {
		return &LoginResponse{Status: -1}, "", err
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
	res, session_id, err := l.login(r.Context(), req_body.Username, req_body.Password)
	if err != nil {
		panic(err)
	}
	if res.Status == 0 {
		login_success_setCookie(w, session_id, res.CsrfToken, l.cookieDomain)
	}
	util.JsonWrite(w, res)
}

func (l *LoginHandler) change_password(ctx context.Context, session_id string, req *ChangePwdRequest, w http.ResponseWriter) {
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
		util.JsonWrite(w, ChangePwdResponse{Status: -1})
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
	l.change_password(r.Context(), ck.Value, &req_body, w)
}
