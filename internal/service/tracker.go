package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"nuha.dev/gpstracker/internal/util"
)

type TrackerModel struct {
	Id           string     `json:"id"`
	Name         string     `json:"name"`
	Family       string     `json:"family"`
	SerialNumber string     `json:"serial_number"`
	Comment      string     `json:"comment"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

type Tracker struct {
	db *pgxpool.Pool
}

type CreateTrackerRequest struct {
	Name         string `json:"name" validate:"required"`
	Family       string `json:"family" validate:"required"`
	SerialNumber string `json:"serial_number" validate:"required"`
}

type GetTrackerResponse struct {
	BasicResponse
	Trackers []*TrackerModel `json:"trackers"`
}

func (t *Tracker) CreateTracker(req *CreateTrackerRequest, res *BasicResponse) {
	sqlStmt := `INSERT INTO tracker (id,name,family,serial_number,created_at) VALUES($1,$2,$3,$4,now())`
	uuid := util.GenUUID()
	_, err := t.db.Exec(context.Background(), sqlStmt, uuid, req.Name, req.Family, req.SerialNumber)
	if err != nil {
		panic(err)
	}
	res.Status = 0
}

func (t *Tracker) GetTrackers(res *GetTrackerResponse) {
	sqlStmt := `SELECT id,name,family,serial_number,comment,created_at,updated_at FROM public."tracker"`
	rows, _ := t.db.Query(context.Background(), sqlStmt)
	defer rows.Close()
	trackers := make([]*TrackerModel, 0)

	for rows.Next() {
		tracker := &TrackerModel{}
		err := rows.Scan(&tracker.Id, &tracker.Name, &tracker.Family, &tracker.SerialNumber, &tracker.Comment, &tracker.CreatedAt, &tracker.UpdatedAt)
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

func (t *Tracker) DeleteTracker(req *DeleteTrackerRequest, res *BasicResponse) {
	sqlStmt := `DELETE FROM tracker WHERE id = $1`
	ct, err := t.db.Exec(context.Background(), sqlStmt, req.Id)
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

func (t *Tracker) UpdateTrackerName(req *UpdateTrackerNameRequest, res *BasicResponse) {
	sqlStmt := `UPDATE tracker set name = $1,updated_at = now() WHERE id = $2`
	ct, err := t.db.Exec(context.Background(), sqlStmt, req.Name, req.Id)
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

func (t *Tracker) UpdateTrackerComment(req *UpdateTrackerCommentRequest, res *BasicResponse) {
	sqlStmt := `UPDATE tracker set comment = $1 , updated_at = now() WHERE id = $2`
	ct, err := t.db.Exec(context.Background(), sqlStmt, req.Comment, req.Id)
	if err != nil {
		panic(err)
	} else if ct.RowsAffected() > 0 {
		res.Status = 0
	} else {
		res.Status = -1
	}

}
