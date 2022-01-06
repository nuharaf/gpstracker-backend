package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/gpsv2/server"
	"nuha.dev/gpstracker/internal/util"
	"nuha.dev/gpstracker/internal/webapp/common"
)

type Status string

type UserMgmt struct {
	db  *pgxpool.Pool
	log log.Logger
}

func NewUserMgmtApi(db *pgxpool.Pool, gps *server.Server) *UserMgmt {
	u := &UserMgmt{}
	u.db = db
	u.log = log.DefaultLogger
	return u
}

type UserModel struct {
	UserId           uint64 `json:"user_id"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	RequireChangePwd bool   `json:"require_change_pwd"`
	SuspendLogin     bool   `json:"suspend_login"`
	Roles            string `json:"roles"`
}

type AddUserRequest struct {
	Username      string `json:"username" validate:"required"`
	Password      string `json:"password" validate:"required"`
	Role          string `json:"role" validate:"oneof=tracker-monitor tracker-admin admin"`
	SessionLength uint64 `json:"session_length" validate:"required"`
}

func (u *UserMgmt) AddUser(ctx context.Context, req *AddUserRequest, res *common.BasicResponse) error {
	hashedPwd := util.CryptPwd(req.Password)
	sqlStmt := `INSERT INTO "user" (username,"password",require_change_pwd,suspend_login,role,session_length_sec) VALUES ($1,$2,true,false,$4,$5)`
	_, err := u.db.Exec(ctx, sqlStmt, req.Username, hashedPwd, req.Role, req.SessionLength)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == "user_username_key" {
				res.Status = -1
				u.log.Warn().Msg("trying to create user with existing username")
				return nil
			}
		}
		return err
	}
	res.Status = 0
	return nil
}

func (u *UserMgmt) GetUsers(ctx context.Context, res *[]*UserModel) error {
	sqlStmt := `SELECT id,username,"password",suspended,init_done,created_at,updated_at FROM "user"`
	rows, _ := u.db.Query(ctx, sqlStmt)
	defer rows.Close()
	users := make([]*UserModel, 0)

	for rows.Next() {
		user := &UserModel{}
		var pwd string
		err := rows.Scan(&user.UserId, &user.Username, pwd, &user.SuspendLogin, &user.RequireChangePwd)
		user.Password = fmt.Sprintf("****%s", pwd[:4])
		if err != nil {
			return err
		}
		users = append(users, user)
	}
	*res = users
	return nil
}

type SetSuspendFlagRequest struct {
	UserId  uint64 `json:"user_id" validate:"required"`
	Suspend bool   `json:"suspend" validate:"required"`
}

func (u *UserMgmt) SetSuspendFlag(ctx context.Context, req *SetSuspendFlagRequest, res *common.BasicResponse) error {
	sqlStmt := `UPDATE "user" SET suspend_login = $1 WHERE id = $2`
	ct, err := u.db.Exec(ctx, sqlStmt, req.Suspend, req.UserId)
	if err != nil {
		return err
	}
	if row := ct.RowsAffected(); row == 1 {
		res.Status = 0
	} else {
		res.Status = -1
	}
	return nil
}

type ListSessionRequest struct {
	UserId uint64 `json:"user_id" validate:"required"`
}

type ListSessionResponse struct {
	SessionId  string    `json:"session_id"`
	ValidUntil time.Time `json:"valid_until"`
	WsToken    string    `json:"ws_token"`
}

func (u *UserMgmt) ListSession(ctx context.Context, req *ListSessionRequest, res *[]*ListSessionResponse) error {
	sqlStmt := `SELECT session.session_id,websocket_session.ws_token, session.valid_until FROM session LEFT JOIN websocket_session ON websocket_session.session_id = session.session_id WHERE session.user_id = $1`
	rows, _ := u.db.Query(ctx, sqlStmt, req.UserId)
	defer rows.Close()
	sessions := make([]*ListSessionResponse, 0)

	for rows.Next() {
		sess := &ListSessionResponse{}
		err := rows.Scan(&sess.SessionId, &sess.WsToken, &sess.ValidUntil)
		if err != nil {
			return err
		}
		sessions = append(sessions, sess)
	}
	*res = sessions
	return nil
}

type PurgeSessionRequest struct {
	UserId uint64 `json:"user_id" validate:"required"`
	All    bool   `json:"all"`
}

type PurgeSessionResponse struct {
	Count int64 `json:"user_id"`
}

func (u *UserMgmt) PurgeSession(ctx context.Context, req *PurgeSessionRequest, res *PurgeSessionResponse) error {
	var sqlExec string
	if req.All {
		sqlExec = `DELETE FROM session WHERE session.user_id = $1`
	} else {
		sqlExec = `DELETE FROM  session where session.user_id = $1 AND session.valid_until > now()`
	}
	ct, err := u.db.Exec(ctx, sqlExec, req.UserId)
	if err != nil {
		return err
	}
	res.Count = ct.RowsAffected()
	return nil

}

// type ChangePasswordRequest struct {
// 	CurrentPassword string `json:"current_password" validate:"required"`
// 	NewPassword     string `json:"new_password" validate:"required"`
// }

// func (u *User) ChangeSelfPassword(ctx context.Context, req *UpdateUserStatusRequest, res *BasicResponse) {
// 	sqlStmt := `UPDATE "user" SET status = $1 WHERE id = $2`
// 	ct, err := u.db.Exec(ctx, sqlStmt, req.Status, req.Id)
// 	if err != nil {
// 		panic(err)
// 	}
// 	if row := ct.RowsAffected(); row == 1 {
// 		res.Status = 0
// 	} else {
// 		res.Status = -1
// 	}
// }

// func (u *User) getUserById(id uint64) (*UserModel, bool) {
// 	sqlStmt := `SELECT id,username,"password",status FROM "user" WHERE id=$1 `
// 	row := u.db.QueryRow(context.Background(), sqlStmt, id)
// 	user := &UserModel{}
// 	err := row.Scan(&user.Id, &user.Username, &user.Password, &user.Status)
// 	if err == pgx.ErrNoRows {
// 		return nil, false
// 	} else if err != nil {
// 		panic(err)
// 	}
// 	return user, true
// }

// func (u *User) getUserByCredential(username, password string) (*UserModel, bool) {
// 	sqlStmt := `SELECT id,username,password",status FROM "user" WHERE username = $1`
// 	row := u.db.QueryRow(context.Background(), sqlStmt, username)
// 	user := &UserModel{}
// 	err := row.Scan(&user.Id, &user.Username, &user.Password, &user.Status)
// 	if err == pgx.ErrNoRows {
// 		return nil, false
// 	} else if err != nil {
// 		panic(err)
// 	}
// 	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
// 	if err != nil {
// 		return nil, false
// 	} else {
// 		return user, true
// 	}
// }

// func (u *User) createSession(id string, sessionId, csrfToken, wsToken string) {
// 	sqlStmt := `INSERT INTO session (session_id,user_id,csrf_token,ws_token,created_at) VALUES($1,$2,$3,$4,now())`
// 	_, err := u.db.Exec(context.Background(), sqlStmt, sessionId, id, csrfToken, wsToken)
// 	if err != nil {
// 		panic(err)
// 	}
// }

// func (u *User) getSessionData(sessionId string) (*SessionModel, bool) {
// 	sqlStmt := `SELECT session_id,user_id,csrf_token,ws_token,created_at FROM session WHERE session_id=$1`
// 	row := u.db.QueryRow(context.Background(), sqlStmt, sessionId)
// 	sess := &SessionModel{}
// 	err := row.Scan(&sess.sessionId, &sess.userId, &sess.csrfToken, &sess.wsToken, &sess.createdAt)
// 	if err == pgx.ErrNoRows {
// 		return nil, false
// 	} else if err != nil {
// 		panic(err)
// 	}
// 	return sess, true
// }

// func (u *User) getSessionFromWsToken(wsToken string) (*SessionModel, bool) {
// 	sqlStmt := `SELECT session_id,user_id,csrf_token,ws_token,created_at FROM session WHERE session_id=$1`
// 	row := u.db.QueryRow(context.Background(), sqlStmt, wsToken)
// 	sess := &SessionModel{}
// 	err := row.Scan(&sess.sessionId, &sess.userId, &sess.csrfToken, &sess.wsToken, &sess.createdAt)
// 	if err == pgx.ErrNoRows {
// 		return nil, false
// 	} else if err != nil {
// 		panic(err)
// 	}
// 	return sess, true
// }

// func (u *User) clearSession(sessionId string) {
// 	sqlStmt := `DELETE FROM session where session_id = $1)`
// 	_, err := u.db.Exec(context.Background(), sqlStmt, sessionId)
// 	if err != nil {
// 		panic(err)
// 	}
// }
