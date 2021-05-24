package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4/pgxpool"
	"nuha.dev/gpstracker/internal/util"
)

type Status string

type User struct {
	db *pgxpool.Pool
}

type UserModel struct {
	Id        string       `json:"id"`
	Username  string       `json:"username"`
	Password  string       `json:"password"`
	InitDone  bool         `json:"init_done"`
	Suspended bool         `json:"suspended"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt sql.NullTime `json:"updated_at"`
}

type CreateUserRequest struct {
	Username      string `json:"username" validate:"required"`
	Password      string `json:"password" validate:"required"`
	Role          string `json:"role" validate:"oneof=monitor admin superadmin"`
	SessionLength uint64 `json:"session_length" validate:"required"`
}

func (u *User) CreateUser(ctx context.Context, req *CreateUserRequest, res *BasicResponse) {
	hashedPwd := util.CryptPwd(req.Password)
	uuid := util.GenUUID()
	sqlStmt := `INSERT INTO public."user" (id,username,"password",init_done,suspended,role,session_length_sec,created_at) VALUES ($1,$2,$3,false,false,$4,$5,now())`
	_, err := u.db.Exec(ctx, sqlStmt, uuid, req.Username, hashedPwd, req.Role, req.SessionLength)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == "user_username_key" {
				res.Status = -1
				return
			}
		}
		panic(err)
	}
	res.Status = 0
}

type GetUserResponse struct {
	BasicResponse
	Users []*UserModel `json:"users"`
}

func (u *User) GetUsers(ctx context.Context, res *GetUserResponse) {
	sqlStmt := `SELECT id,username,"password",suspended,init_done,created_at,updated_at FROM public."user"`
	rows, _ := u.db.Query(ctx, sqlStmt)
	defer rows.Close()
	users := make([]*UserModel, 0)

	for rows.Next() {
		user := &UserModel{}
		err := rows.Scan(&user.Id, &user.Username, &user.Password, &user.Suspended, &user.InitDone, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			panic(err)
		}
		users = append(users, user)
	}
	res.Users = users
	res.Status = 0
}

type SuspendUserRequest struct {
	Id string `json:"id" validate:"required"`
}

func (u *User) SuspendUser(ctx context.Context, req *SuspendUserRequest, res *BasicResponse) {
	sqlStmt := `UPDATE "user" SET suspended = true WHERE id = $1`
	ct, err := u.db.Exec(ctx, sqlStmt, req.Id)
	if err != nil {
		panic(err)
	}
	if row := ct.RowsAffected(); row == 1 {
		res.Status = 0
	} else {
		res.Status = -1
	}
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
