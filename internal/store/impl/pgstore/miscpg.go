package pgstore

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
)

type PgMiscStore struct {
	db  *pgxpool.Pool
	log log.Logger
}

func NewMiscStore(db *pgxpool.Pool) *PgMiscStore {
	m := PgMiscStore{}
	m.db = db
	m.log.Context = log.NewContext(nil).Str("module", "misc_store").Value()
	return &m
}

func (st *PgMiscStore) SaveCommandResponse(tid uint64, server_flag uint32, command string, ct time.Time, response string, rt time.Time) {
	var err error
	_, err = st.db.Exec(context.Background(), `INSERT INTO gt06_command_response (tracker_id,server_flag,command,command_time,response,response_time) VALUES ($1,$2,$3,$4,$5,$6)`, tid, server_flag, command, ct, response, rt)
	if err != nil {
		st.log.Error().Err(err).Msg("error saving command response")
	}
}

func (st *PgMiscStore) SaveEvent(tid uint64, event_type string, message string, message_json interface{}, t time.Time) {
	_, err := st.db.Exec(context.Background(), `INSERT INTO event_message (event_type,message,message_json,tracker_id,event_timestamp) VALUES ($1,$2,$3,$4,$5)`, event_type, message, message_json, tid, t)
	if err != nil {
		st.log.Error().Err(err).Msg("error saving event")
	}

}

func (st *PgMiscStore) UpdateAttribute(tid uint64, key string, value string) {
	_, err := st.db.Exec(context.Background(), `UPDATE tracker SET attribute = attribute || jsonb_build_object($1::text,$2::text) where id = $3`, key, value, tid)
	if err != nil {
		st.log.Error().Err(err).Msg("error updating attribute")
	}
}
