package webapp

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"net/http"
	"time"

	"github.com/jackc/pgx/v4"
	"golang.org/x/crypto/bcrypt"
	"nuha.dev/gpstracker/internal/util"
)

type loginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type loginResponse struct {
	Status                int       `json:"status"`
	RequireChangePassword bool      `json:"require_change_pwd"`
	CsrfToken             string    `json:"csrf_token"`
	ValidUntil            time.Time `json:"valid_until"`
	Role                  string    `json:"role"`
}

func (api *Api) login(ctx context.Context, username, password string) (*loginResponse, string, error) {

	sqlStmt := `SELECT id,"password",require_change_pwd,suspend_login,session_length_sec,role FROM "user" WHERE username = $1`
	row := api.db.QueryRow(ctx, sqlStmt, username)
	var id uint64
	var hashpwd string
	var sess_length int
	var require_change_pwd bool
	var suspend_login bool
	var role string
	err := row.Scan(&id, &hashpwd, &require_change_pwd, &suspend_login, &sess_length, &role)
	if err == pgx.ErrNoRows {
		return &loginResponse{Status: -1}, "", nil
	} else if err != nil {
		return nil, "", err
	}
	if !suspend_login {
		err = bcrypt.CompareHashAndPassword([]byte(hashpwd), []byte(password))
		if err != nil {
			return &loginResponse{Status: -1}, "", nil
		} else {
			var prefix [4]byte
			binary.BigEndian.PutUint32(prefix[:], crc32.ChecksumIEEE([]byte(username)))
			session_id := util.GenRandomString(prefix[:], 24)
			csrf_token := util.GenRandomString(prefix[:], 24)

			sqlStmt := `INSERT INTO session (session_id,user_id,valid_until) 
			VALUES($1,$2,$3)`
			valid_until := time.Now().Add(time.Duration(sess_length) * time.Second)
			_, err := api.db.Exec(ctx, sqlStmt, session_id, id, valid_until)
			if err != nil {
				return nil, "", err
			}
			return &loginResponse{Status: 0, RequireChangePassword: require_change_pwd, CsrfToken: csrf_token, ValidUntil: valid_until, Role: role}, session_id, nil
		}
	} else {
		return &loginResponse{Status: -1}, "", nil
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

func (api *Api) Login(w http.ResponseWriter, r *http.Request) {
	req_body := loginRequest{}
	err := json.NewDecoder(r.Body).Decode(&req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	err = api.vld.Struct(req_body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	res, session_id, err := api.login(r.Context(), req_body.Username, req_body.Password)
	if err != nil {
		panic(err)
	}
	if res.Status == 0 {
		login_success_setCookie(w, session_id, res.CsrfToken, api.config.CookieDomain)
	}
	util.JsonWrite(w, res)
}
