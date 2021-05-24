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

type TrackerModel struct {
	Id           string       `json:"id"`
	Name         string       `json:"name"`
	Family       string       `json:"family"`
	SerialNumber string       `json:"serial_number"`
	Notes        string       `json:"comment"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    sql.NullTime `json:"updated_at"`
}

type Tracker struct {
	db *pgxpool.Pool
}

type CreateTrackerRequest struct {
	Name         string `json:"name" validate:"required"`
	Family       string `json:"family" validate:"required,oneof=gt06 json"`
	SerialNumber string `json:"serial_number" validate:"required"`
	Notes        string `json:"notes"`
}

type GetTrackerResponse struct {
	BasicResponse
	Trackers []*TrackerModel `json:"trackers"`
}

func (t *Tracker) CreateTracker(ctx context.Context, req *CreateTrackerRequest, res *BasicResponse) {
	sqlStmt := `INSERT INTO tracker (id,name,family,serial_number,notes,created_at) VALUES($1,$2,$3,$4,$5,now())`
	uuid := util.GenUUID()
	_, err := t.db.Exec(ctx, sqlStmt, uuid, req.Name, req.Family, req.SerialNumber, req.Notes)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == "tracker_serial_number_key" {
				res.Status = -1
				return
			}
		}
		panic(err)
	}
	res.Status = 0
}

func (t *Tracker) GetTrackers(ctx context.Context, res *GetTrackerResponse) {
	sqlStmt := `SELECT id,name,family,serial_number,notes,created_at,updated_at FROM tracker`
	rows, _ := t.db.Query(ctx, sqlStmt)
	defer rows.Close()
	trackers := make([]*TrackerModel, 0)

	for rows.Next() {
		tracker := &TrackerModel{}
		err := rows.Scan(&tracker.Id, &tracker.Name, &tracker.Family, &tracker.SerialNumber, &tracker.Notes, &tracker.CreatedAt, &tracker.UpdatedAt)
		if err != nil {
			panic(err)
		}
		trackers = append(trackers, tracker)
	}
	res.Status = 0
	res.Trackers = trackers
}

type DeleteTrackerRequest struct {
	Id string `json:"id" validate:"required"`
}

func (t *Tracker) DeleteTracker(ctx context.Context, req *DeleteTrackerRequest, res *BasicResponse) {
	sqlStmt := `DELETE FROM tracker WHERE id = $1`
	ct, err := t.db.Exec(ctx, sqlStmt, req.Id)
	if err != nil {
		panic(err)
	} else if ct.RowsAffected() > 0 {
		res.Status = 0
	} else {
		res.Status = -1
	}
}

type UpdateTrackerNameRequest struct {
	Id   string `json:"id" validate:"required"`
	Name string `json:"name" validate:"required"`
}

func (t *Tracker) UpdateTrackerName(ctx context.Context, req *UpdateTrackerNameRequest, res *BasicResponse) {
	sqlStmt := `UPDATE tracker set name = $1,updated_at = now() WHERE id = $2`
	ct, err := t.db.Exec(ctx, sqlStmt, req.Name, req.Id)
	if err != nil {
		panic(err)
	} else if ct.RowsAffected() > 0 {
		res.Status = 0
	} else {
		res.Status = -1
	}
}

type UpdateTrackerCommentRequest struct {
	Id      string `json:"id" validate:"required"`
	Comment string `json:"comment" validate:"required"`
}

func (t *Tracker) UpdateTrackerComment(ctx context.Context, req *UpdateTrackerCommentRequest, res *BasicResponse) {
	sqlStmt := `UPDATE tracker set comment = $1 , updated_at = now() WHERE id = $2`
	ct, err := t.db.Exec(ctx, sqlStmt, req.Comment, req.Id)
	if err != nil {
		panic(err)
	} else if ct.RowsAffected() > 0 {
		res.Status = 0
	} else {
		res.Status = -1
	}

}
