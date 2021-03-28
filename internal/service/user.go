package service

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"nuha.dev/gpstracker/internal/util"
)

type Status string

type User struct {
	db *pgxpool.Pool
}

const (
	Enabled   Status = "enabled"
	Reset     Status = "reset"
	Suspended Status = "suspended"
)

type CreateUserRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type GetUserResponse struct {
	BasicResponse
	Users []*UserModel `json:"users"`
}

type UserModel struct {
	Id        string       `json:"id"`
	Username  string       `json:"username"`
	Password  string       `json:"password"`
	Status    Status       `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt sql.NullTime `json:"updated_at"`
}

type SessionModel struct {
	sessionId string
	csrfToken string
	wsToken   string
	userId    uint64
	createdAt time.Time
	updatedAt *time.Time
}

func (u *User) CreateUser(req *CreateUserRequest, res *BasicResponse) {
	hashedPwd := util.CryptPwd(req.Password)
	uuid := util.GenUUID()
	sqlStmt := `INSERT INTO public."user" (id,username,"password",status,created_at) VALUES ($1,$2,$3,$4,now())`
	_, err := u.db.Exec(context.Background(), sqlStmt, uuid, req.Username, hashedPwd, Reset)
	if err != nil {
		panic(err)
	}
	res.Status = 0
}

func (u *User) GetUsers(res *GetUserResponse) {
	sqlStmt := `SELECT id,username,"password",status,created_at,updated_at FROM public."user"`
	rows, _ := u.db.Query(context.Background(), sqlStmt)
	defer rows.Close()
	users := make([]*UserModel, 0)

	for rows.Next() {
		user := &UserModel{}
		err := rows.Scan(&user.Id, &user.Username, &user.Password, &user.Status, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			panic(err)
		}
		users = append(users, user)
	}
	res.Users = users
	res.Status = 0
}

// func (u *User) changeUserStatus(id uint64, status string) bool {
// 	sqlStmt := `UPDATE "user" SET status = $1 WHERE id = id`
// 	res, err := u.db.Exec(context.Background(), sqlStmt, status)
// 	if err != nil {
// 		panic(err)
// 	}
// 	if row := res.RowsAffected(); row == 1 {
// 		return true
// 	}
// 	return false
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
