package tracker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4"
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

func (t *Tracker) CreateTracker(name string, family string, serialNumber string) {
	sqlStmt := `INSERT INTO tracker (id,name,family,serial_number,created_at) VALUES($1,$2,$3,$4,now())`
	uuid := util.GenUUID()
	_, err := t.db.Exec(context.Background(), sqlStmt, uuid, name, family, serialNumber)
	if err != nil {
		panic(err)
	}
}

func (t *Tracker) GetTrackers() []*TrackerModel {
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
	return trackers
}

func (t *Tracker) DeleteTracker(id string) bool {
	sqlStmt := `DELETE FROM tracker WHERE id = $1`
	ct, err := t.db.Exec(context.Background(), sqlStmt, id)
	if err != nil {
		panic(err)
	} else if ct.RowsAffected() > 0 {
		return true
	} else {
		return false
	}
}

func (t *Tracker) UpdateTrackerName(id uint64, name string) bool {
	sqlStmt := `UPDATE tracker set name = $1,updated_at = now() WHERE id = $2`
	ct, err := t.db.Exec(context.Background(), sqlStmt, name, id)
	if err != nil {
		panic(err)
	} else if ct.RowsAffected() > 0 {
		return true
	} else {
		return false
	}
}

func (t *Tracker) UpdateTrackerComment(id string, comment string) bool {
	sqlStmt := `UPDATE tracker set comment = $1 , updated_at = now() WHERE id = $2`
	ct, err := t.db.Exec(context.Background(), sqlStmt, comment, id)
	if err != nil {
		panic(err)
	} else if ct.RowsAffected() > 0 {
		return true
	} else {
		return false
	}

}

func (t *Tracker) VerifyTracker(family, serialNumber string) bool {
	sqlStmt := `SELECT id,name FROM public."tracker" where family = $1 AND serial_number =$2`
	err := t.db.QueryRow(context.Background(), sqlStmt, family, serialNumber).Scan()
	if err != nil {
		if err != pgx.ErrNoRows {
			return false
		}
		panic(err)
	} else {
		return true
	}
}
