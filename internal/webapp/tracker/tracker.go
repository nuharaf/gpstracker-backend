package tracker

import (
	"context"

	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/gpsv2/device"
	"nuha.dev/gpstracker/internal/gpsv2/device/gt06"
	"nuha.dev/gpstracker/internal/gpsv2/server"
	"nuha.dev/gpstracker/internal/util"
	"nuha.dev/gpstracker/internal/webapp/common"
)

type EditTrackerRequestModel struct {
	TrackerId uint64          `json:"tracker_id"`
	Settings  TrackerSettings `json:"settings"`
}

type TrackerSettings struct {
	AllowConnect   *bool   `json:"allow_connect,omitempty"`
	AllowSubscribe *bool   `json:"allow_subscribe,omitempty"`
	Store          *bool   `json:"store,omitempty"`
	Broadcast      *bool   `json:"broadcast,omitempty"`
	LogLevel       *string `json:"log_level,omitempty"`
}

type TrackerIdRequestModel struct {
	TrackerId uint64 `json:"tracker_id"`
}

type TrackerLastLocationModel struct {
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	Speed      float32   `json:"speed"`
	Altitude   float32   `json:"altitude"`
	Timestamp  time.Time `json:"timestamp"`
	ServerTime time.Time `json:"server_time"`
}

type TrackerStatusResponseModel struct {
	Online                   bool       `json:"online"`
	LastMessage              *time.Time `json:"last_message"`
	TrackerLastLocationModel `json:"last_location"`
}

type TrackerDetailModel struct {
	TrackerId    uint64            `json:"tracker_id"`
	SnType       string            `json:"sn_type"`
	SerialNumber uint64            `json:"serial_number"`
	FSN          string            `json:"fsn"`
	Protocol     string            `json:"protocol"`
	Attributes   map[string]string `json:"attributes"`
}

type Tracker struct {
	db  *pgxpool.Pool
	gps *server.Server
	log log.Logger
}

func NewTrackerApi(db *pgxpool.Pool, gps *server.Server) *Tracker {
	t := &Tracker{}
	t.db = db
	t.gps = gps
	t.log = log.DefaultLogger
	return t
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

func (t *Tracker) GetWsToken(ctx context.Context, res *common.StringResponse) error {
	session := ctx.Value(common.ApiContextKeyType("session_attribute")).(*common.UserSessionAtrribute)
	select_sql := `SELECT ws_token FROM websocket_session WHERE session_id = $1`
	err := t.db.QueryRow(ctx, select_sql, session.SessionId).Scan(&res.Value)
	if err != nil {
		if err == pgx.ErrNoRows {
			res.Value = ""
			return nil

		} else {
			return err
		}
	}
	return nil
}

func (t *Tracker) CreateWsToken(ctx context.Context, res *common.StringResponse) error {
	session := ctx.Value(common.ApiContextKeyType("session_attribute")).(*common.UserSessionAtrribute)
	ws_token := util.GenRandomString([]byte{}, 32)
	create_sql := `INSERT INTO websocket_session VALUES ($1,$2) ON CONFLICT (session_id) DO UPDATE SET ws_token = $1 `
	_, err := t.db.Exec(ctx, create_sql, ws_token, session.SessionId)
	if err != nil {
		return err
	} else {
		res.Value = ws_token
		return nil
	}
}

func (t *Tracker) EditTrackerSettings(ctx context.Context, req *EditTrackerRequestModel, res *common.BasicResponse) error {
	sqlStmt := `UPDATE tracker SET config = config || $1 where tracker.id = $2`
	ct, err := t.db.Exec(ctx, sqlStmt, req.Settings, req.TrackerId)
	if err != nil {
		return err
	} else {
		if ct.RowsAffected() < 1 {
			res.Status = -1
		} else {
			res.Status = 0
		}
	}
	return nil
}

func (t *Tracker) GetTrackers(ctx context.Context, res *[]*TrackerDetailModel) error {
	sqlStmt := `SELECT id,sn_type,serial_number,protocol,attribute FROM tracker`
	rows, _ := t.db.Query(ctx, sqlStmt)
	defer rows.Close()
	trackers := make([]*TrackerDetailModel, 0)

	for rows.Next() {
		tracker := &TrackerDetailModel{}
		tracker.Attributes = make(map[string]string)
		err := rows.Scan(&tracker.TrackerId, &tracker.SnType, &tracker.SerialNumber, &tracker.Protocol, &tracker.Attributes)
		tracker.FSN = device.JoinSn(tracker.SnType, tracker.SerialNumber)
		if err != nil {
			return err
		}
		trackers = append(trackers, tracker)
	}
	*res = trackers
	return nil
}

type CloseTrackerRequest struct {
	TrackerId uint64 `json:"tracker_id"`
}

func (t *Tracker) CloseTracker(ctx context.Context, req *CloseTrackerRequest, res *common.BasicResponse) {
	if t.gps.StopDevice(req.TrackerId) {
		res.Status = 0
	} else {
		res.Status = -1
		res.Message = "device not found"
	}

}

func (t *Tracker) GetTrackerStatus(ctx context.Context, res *[]*TrackerDetailModel) {

}

// func (t *Tracker) GetTrackersStatus(ctx context.Context, res *[]gps.ClientStatus) {
// 	*res = t.reg.gsrv.GetClientsStatus()
// }

// func (t *Tracker) GetTrackerStatusDetail(ctx context.Context, req *TrackerStatusDetailRequest, res **gps.ClientStatusDetail) {
// 	*res = t.reg.gsrv.GetClientStatus(req.TId)
// }

func (t *Tracker) GetTrackerHistory(ctx context.Context, req *TrackerHistoryRequestModel, res *[]*TrackerHistoryResponseModel) error {
	sqlStmt := "SELECT latitude,longitude,speed,gps_time,server_time FROM locations WHERE fsn = $1 AND server_time BETWEEN $2 AND $3"
	rows, _ := t.db.Query(ctx, sqlStmt)
	defer rows.Close()
	locs := make([]*TrackerHistoryResponseModel, 0)

	for rows.Next() {
		loc := &TrackerHistoryResponseModel{}
		err := rows.Scan(&loc.Latitude, &loc.Longitude, &loc.Speed, &loc.GpsTime, &loc.ServerTime)
		if err != nil {
			return err
		}

		locs = append(locs, loc)

	}
	*res = locs
	return nil

}

type SendCommandReq struct {
	TrackerId  uint64 `json:"tracker_id"`
	Command    string `json:"command"`
	ServerFlag uint32 `json:"server_flag"`
	Serial     int    `json:"serial"`
}

func (t *Tracker) SendCommand(ctx context.Context, req *SendCommandReq, res *common.BasicResponse) error {
	dev, ok := t.gps.GetDevice(req.TrackerId)
	if !ok {
		res.Status = -1
		res.Message = "device not found"

		return nil
	} else {
		gt06dev, ok := dev.Dev.(*gt06.GT06)
		if !ok {
			res.Status = -1
			res.Message = "device is not gt06"
			return nil
		} else {
			_ = gt06dev.SendMessage(req.Command, req.ServerFlag, req.Serial)
			// res.Message = err.Error()
			// t.reg.log.Error().Err(err).Msg("")
			return nil

		}
	}

}

type SendCommand2Req struct {
	TrackerId uint64 `json:"tracker_id"`
	Command   string `json:"command"`
}

func (t *Tracker) SendCommand2(ctx context.Context, req *SendCommandReq, res *common.BasicResponse) error {
	dev, ok := t.gps.GetDevice(req.TrackerId)
	if !ok {
		res.Status = -1
		res.Message = "device not found"

		return nil
	} else {
		gt06dev, ok := dev.Dev.(*gt06.GT06)
		if !ok {
			res.Status = -1
			res.Message = "device is not gt06"
			return nil
		} else {
			_ = gt06dev.SendCommand(req.Command)
			// res.Message = err.Error()
			// t.reg.log.Error().Err(err).Msg("")
			return nil

		}
	}

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
