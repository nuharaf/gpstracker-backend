package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	gps "nuha.dev/gpstracker/internal/gps/serverimpl"
)

type TrackerDetailModel struct {
	TrackerId    uint64    `json:"tracker_id"`
	SnType       string    `json:"sn_type"`
	SerialNumber uint64    `json:"serial_number"`
	FSN          string    `json:"fsn"`
	AllowConnect bool      `json:"allow_connect"`
	RegisteredAt time.Time `json:"registered_at"`
}

type Tracker struct {
	db  *pgxpool.Pool
	reg *ServiceRegistry
}

type TrackerStatusDetailRequest struct {
	TId uint64 `json:"tid"`
}

type TrackerHistoryRequestModel struct {
	FSNs string
	From time.Time
	To   time.Time
}

type TrackerHistoryResponseModel struct {
	Longitude  float64
	Latitude   float64
	Altitude   float32
	Speed      float32
	GpsTime    time.Time
	ServerTime time.Time
}

// type CreateTrackerRequest struct {
// 	Name         string `json:"name" validate:"required"`
// 	Family       string `json:"family"`
// 	SerialNumber string `json:"serial_number" validate:"required"`
// 	Notes        string `json:"notes"`
// }

// func (t *Tracker) CreateTracker(ctx context.Context, req *CreateTrackerRequest, res *BasicResponse) {
// 	sqlStmt := `INSERT INTO tracker (name,family,serial_number,notes,created_at) VALUES($1,$2,$3,$4,$5,now())`
// 	_, err := t.db.Exec(ctx, sqlStmt, req.Name, req.Family, req.SerialNumber, req.Notes)
// 	if err != nil {
// 		var pgErr *pgconn.PgError
// 		if errors.As(err, &pgErr) {
// 			if pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == "tracker_serial_number_key" {
// 				res.Status = -1
// 				return
// 			}
// 		}
// 		panic(err)
// 	}
// 	res.Status = 0
// }

// func (t *Tracker) GetRefSnType(ctx context.Context, res *[]string) {
// 	sqlStmt := `SELECT type FROM ref_sn_type`
// 	rows, _ := t.db.Query(ctx, sqlStmt)
// 	defer rows.Close()
// 	sn_type := make([]string, 0)

// 	for rows.Next() {
// 		var s string
// 		err := rows.Scan(&s)
// 		if err != nil {
// 			panic(err)
// 		}
// 		sn_type = append(sn_type, s)
// 	}
// 	*res = sn_type
// }

func (t *Tracker) GetRegisteredTrackers(ctx context.Context, res *[]*TrackerDetailModel) {
	sqlStmt := `SELECT id,sn_type,serial_number,fsn, allow_connect,registered_at FROM tracker`
	rows, _ := t.db.Query(ctx, sqlStmt)
	defer rows.Close()
	trackers := make([]*TrackerDetailModel, 0)

	for rows.Next() {
		tracker := &TrackerDetailModel{}
		err := rows.Scan(&tracker.TrackerId, &tracker.SnType, &tracker.SerialNumber, &tracker.FSN, &tracker.AllowConnect, &tracker.RegisteredAt)
		if err != nil {
			panic(err)
		}
		trackers = append(trackers, tracker)
	}
	*res = trackers
}

func (t *Tracker) GetTrackersStatus(ctx context.Context, res *[]gps.ClientStatus) {
	*res = t.reg.gsrv.GetClientsStatus()
}

func (t *Tracker) GetTrackerStatusDetail(ctx context.Context, req *TrackerStatusDetailRequest, res **gps.ClientStatusDetail) {
	*res = t.reg.gsrv.GetClientStatus(req.TId)
}

func (t *Tracker) GetTrackerHistory(ctx context.Context, req *TrackerHistoryRequestModel, res *[]*TrackerHistoryResponseModel) {
	sqlStmt := "SELECT latitude,longitude,speed,gps_time,server_time FROM locations WHERE fsn = $1 AND server_time BETWEEN $2 AND $3"
	rows, _ := t.db.Query(ctx, sqlStmt)
	defer rows.Close()
	locs := make([]*TrackerHistoryResponseModel, 0)

	for rows.Next() {
		loc := &TrackerHistoryResponseModel{}
		err := rows.Scan(&loc.Latitude, &loc.Longitude, &loc.Speed, &loc.GpsTime, &loc.ServerTime)
		if err != nil {
			panic(err)
		}

		locs = append(locs, loc)

	}
	*res = locs

}

// type DeleteTrackerRequest struct {
// 	Id string `json:"id" validate:"required"`
// }

// func (t *Tracker) DeleteTracker(ctx context.Context, req *DeleteTrackerRequest, res *BasicResponse) {
// 	sqlStmt := `DELETE FROM tracker WHERE id = $1`
// 	ct, err := t.db.Exec(ctx, sqlStmt, req.Id)
// 	if err != nil {
// 		panic(err)
// 	} else if ct.RowsAffected() > 0 {
// 		res.Status = 0
// 	} else {
// 		res.Status = -1
// 	}
// }

// type UpdateTrackerNameRequest struct {
// 	Id   uint64 `json:"id" validate:"required"`
// 	Name string `json:"name" validate:"required"`
// }

// func (t *Tracker) UpdateTrackerName(ctx context.Context, req *UpdateTrackerNameRequest, res *BasicResponse) {
// 	sqlStmt := `UPDATE tracker set name = $1,updated_at = now() WHERE id = $2`
// 	ct, err := t.db.Exec(ctx, sqlStmt, req.Name, req.Id)
// 	if err != nil {
// 		panic(err)
// 	} else if ct.RowsAffected() > 0 {
// 		res.Status = 0
// 	} else {
// 		res.Status = -1
// 	}
// }

// type UpdateTrackerCommentRequest struct {
// 	Id      uint64 `json:"id" validate:"required"`
// 	Comment string `json:"comment" validate:"required"`
// }

// func (t *Tracker) UpdateTrackerComment(ctx context.Context, req *UpdateTrackerCommentRequest, res *BasicResponse) {
// 	sqlStmt := `UPDATE tracker set comment = $1 , updated_at = now() WHERE id = $2`
// 	ct, err := t.db.Exec(ctx, sqlStmt, req.Comment, req.Id)
// 	if err != nil {
// 		panic(err)
// 	} else if ct.RowsAffected() > 0 {
// 		res.Status = 0
// 	} else {
// 		res.Status = -1
// 	}

// }
