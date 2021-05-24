package main

import (
	"context"

	"github.com/jackc/pgx/v4/pgxpool"
	"nuha.dev/gpstracker/internal/util"
)

func main() {
	pool, err := pgxpool.Connect(context.Background(), "postgresql://postgres:postgres@localhost/gpsv2")
	if err != nil {
		panic(err.Error())
	}
	hashedPwd := util.CryptPwd("passw")
	username := "userw"
	uuid := util.GenUUID()
	sqlStmt := `INSERT INTO public."user" (id,username,"password",status,created_at,role,session_length_sec) VALUES ($1,$2,$3,$4,now(),$5,9)`
	_, err = pool.Exec(context.Background(), sqlStmt, uuid, username, hashedPwd, "init", "superadmin")
	if err != nil {
		panic(err.Error())
	}
}
