package tracker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"

	"time"

	"github.com/jackc/pgconn"
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
	TrackerId uint64         `json:"tracker_id" validate:"required"`
	Config    TrackerConfigs `json:"config" validate:"required"`
}

type SetTrackerNameRequestModel struct {
	TrackerId uint64 `json:"tracker_id" validate:"required"`
	Name      string `json:"name" validate:"required"`
}

type TrackerConfigs struct {
	AllowConnect *bool   `json:"allow_connect,omitempty"`
	SublistSend  *bool   `json:"sublist_send,omitempty"`
	Store        *bool   `json:"store,omitempty"`
	Broadcast    *bool   `json:"broadcast,omitempty"`
	LogLevel     *string `json:"log_level,omitempty" validate:"omitempty,oneof=trace debug warn info error"`
	ReadDeadline *int    `json:"read_deadline,omitempty" validate:"omitempty,ne=0"`
}

type TrackerIdRequestModel struct {
	TrackerId uint64 `json:"tracker_id" validate:"required"`
}

type TrackerConnInfo struct {
	ConnInfo []string `json:"conn_info"`
	Status   int      `json:"status"`
}

type TrackerIdTimeRequestModel struct {
	TrackerId uint64    `json:"tracker_id"`
	Timestamp time.Time `json:"timestamp"`
	Pointer   uint64    `json:"pointer"`
	Limit     int       `json:"limit"`
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

type TrackerModel struct {
	TrackerId          uint64  `json:"tracker_id"`
	Name               string  `json:"name"`
	SnType             string  `json:"sn_type"`
	SerialNumber       uint64  `json:"serial_number"`
	NSerialNumber      uint64  `json:"nserial_number"`
	SerialNumberPretty string  `json:"serial_number_pretty"`
	Protocol           string  `json:"protocol"`
	Vehicle            *string `json:"vehicle"`
	// Attributes   map[string]string    `json:"attributes,omitempty"`
	// Config       *device.DeviceConfig `json:"config,omitempty"`
}

type TrackerDetailModel struct {
	TrackerId          uint64               `json:"tracker_id"`
	Name               string               `json:"name"`
	SnType             string               `json:"sn_type"`
	NSerialNumber      uint64               `json:"nserial_number"`
	SerialNumber       uint64               `json:"serial_number"`
	SerialNumberPretty string               `json:"serial_number_pretty"`
	Protocol           string               `json:"protocol"`
	Vehicle            *string              `json:"vehicle"`
	Attributes         map[string]string    `json:"attributes,omitempty"`
	Config             *device.DeviceConfig `json:"config,omitempty"`
}

type TrackerEventModel struct {
	Id             uint64          `json:"id"`
	TrackerId      uint64          `json:"tracker_id"`
	EventType      string          `json:"event_type"`
	Message        string          `json:"message"`
	MessageJson    json.RawMessage `json:"message_obj"`
	EventTimestamp time.Time       `json:"event_timestamp"`
}

type TrackersEventModel struct {
	Id             uint64          `json:"id"`
	TrackerId      uint64          `json:"tracker_id"`
	SerialNumber   uint64          `json:"serial_number"`
	EventType      string          `json:"event_type"`
	Message        string          `json:"message"`
	MessageJson    json.RawMessage `json:"message_obj"`
	EventTimestamp time.Time       `json:"event_timestamp"`
}

type GT06CmdResponseModel struct {
	Id           uint64    `json:"id"`
	TrackerId    uint64    `json:"tracker_id"`
	ServerFlag   uint32    `json:"server_flag"`
	Command      string    `json:"command"`
	CommandTime  time.Time `json:"command_timestamp"`
	Response     string    `json:"response"`
	ResponseTime time.Time `json:"response_timestamp"`
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
	NSN   []uint64  `json:"nsn" validate:"required"`
	From  time.Time `json:"from" validate:"required"`
	To    time.Time `json:"to" validate:"required"`
	Limit int       `json:"limit"`
	Chunk int       `json:"chunk"`
}

type TrackerLocationHistoryResponseModel struct {
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
	ct, err := t.db.Exec(ctx, sqlStmt, req.Config, req.TrackerId)
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

func (t *Tracker) SetTrackerName(ctx context.Context, req *SetTrackerNameRequestModel, res *common.BasicResponse) error {
	sqlStmt := `UPDATE tracker SET name = $1  where tracker.id = $2`
	ct, err := t.db.Exec(ctx, sqlStmt, req.Name, req.TrackerId)
	if err != nil {
		pgerr, ok := err.(*pgconn.PgError)
		if !ok {
			return err
		} else if pgerr.Code == "23505" {
			res.Status = -1
			res.Message = "duplicate name found"
			return nil
		}
	} else if ct.RowsAffected() < 1 {
		res.Status = 0
		res.Message = "tracker not found"

	} else {
		res.Status = 1
		res.Message = "success"
	}
	return nil
}

func (t *Tracker) GetTrackers(ctx context.Context, res *[]*TrackerModel) error {
	sqlStmt := `SELECT id,name,nsn,protocol,vehicle FROM tracker`
	rows, _ := t.db.Query(ctx, sqlStmt)
	defer rows.Close()
	trackers := make([]*TrackerModel, 0)

	for rows.Next() {
		tracker := &TrackerModel{}
		var nsn uint64
		var id uint64
		err := rows.Scan(&id, &tracker.Name, &nsn, &tracker.Protocol, &tracker.Vehicle)
		ser := device.NewSerial2(nsn)
		tracker.SnType = ser.SnTypeString()
		tracker.SerialNumber = ser.Sn()
		tracker.SerialNumberPretty = ser.SnString()
		tracker.NSerialNumber = nsn
		tracker.TrackerId = id

		if err != nil {
			return err
		}
		trackers = append(trackers, tracker)
	}
	*res = trackers
	return nil
}

func (t *Tracker) GetTrackerDetail(ctx context.Context, req *TrackerIdRequestModel, res *TrackerDetailModel) error {
	sqlStmt := `SELECT id,name,nsn,protocol,vehicle,attribute,config FROM tracker WHERE id = $1`
	res.Attributes = map[string]string{}
	res.Config = &device.DeviceConfig{}
	var nsn uint64
	err := t.db.QueryRow(ctx, sqlStmt, req.TrackerId).Scan(&res.TrackerId, &res.Name, &nsn, &res.Protocol, &res.Vehicle, &res.Attributes, &res.Config)
	if err != nil {
		return err
	}
	ser := device.NewSerial2(nsn)
	res.SnType = ser.SnTypeString()
	res.SerialNumber = ser.Sn()
	res.SerialNumberPretty = ser.SnString()
	res.NSerialNumber = nsn
	return nil
}

func (t *Tracker) GetTrackerCurrentConnInfo(ctx context.Context, req *TrackerIdRequestModel, res *TrackerConnInfo) error {
	dev, ok := t.gps.GetDevice(req.TrackerId)
	if !ok {
		res.Status = -1
	} else {
		res.Status = 0
		res.ConnInfo = dev.Dev.CurrentConnInfo()
	}
	return nil
}

func (t *Tracker) GetTrackerEvent(ctx context.Context, req *TrackerIdTimeRequestModel, res *[]*TrackerEventModel) error {
	var query string
	var rows pgx.Rows
	var err error
	events := make([]*TrackerEventModel, 0)
	var before time.Time
	var tracker_id_flag bool
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Timestamp.IsZero() {
		before = time.Now()
	} else {
		before = req.Timestamp
	}
	if req.TrackerId == 0 {
		tracker_id_flag = true
	} else {
		tracker_id_flag = false
	}
	if req.Pointer == 0 {
		query = `SELECT id,tracker_id, event_timestamp,event_type,message,message_json FROM event_message WHERE (tracker_id = $1 OR $2) AND event_timestamp < $3 ORDER BY event_timestamp DESC LIMIT $4`
		rows, err = t.db.Query(ctx, query, req.TrackerId, tracker_id_flag, before, req.Limit)
	} else {
		query = `SELECT id,tracker_id, event_timestamp,event_type,message,message_json FROM event_message WHERE (tracker_id = $1 OR $2) AND id < $3 ORDER BY id DESC LIMIT $4`
		rows, err = t.db.Query(ctx, query, req.TrackerId, tracker_id_flag, req.Pointer, req.Limit)
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		evt := &TrackerEventModel{}
		err := rows.Scan(&evt.Id, &evt.TrackerId, &evt.EventTimestamp, &evt.EventType, &evt.Message, &evt.MessageJson)
		if err != nil {
			return err
		}
		events = append(events, evt)
	}
	*res = events
	return nil
}

func (t *Tracker) GetGT06CmdHistoryPaginate(ctx context.Context, req *TrackerIdTimeRequestModel, res *[]*GT06CmdResponseModel) {

}

func (t *Tracker) GetGT06CmdHistory(ctx context.Context, req *TrackerIdTimeRequestModel, res *[]*GT06CmdResponseModel) error {
	var query string
	var rows pgx.Rows
	var err error
	cmds := make([]*GT06CmdResponseModel, 0)
	if req.Limit == 0 {
		req.Limit = 20
	}
	var before time.Time
	var tracker_id_flag bool
	if req.Timestamp.IsZero() {
		before = time.Now()
	} else {
		before = req.Timestamp
	}
	if req.TrackerId == 0 {
		tracker_id_flag = true
	} else {
		tracker_id_flag = false
	}
	if req.Pointer == 0 {
		query = `SELECT id, tracker_id,server_flag,response, response_time,command,command_time FROM gt06_command_response WHERE (tracker_id = $1 OR $2) AND response_time < $3 ORDER BY response_time DESC LIMIT $4`
		rows, err = t.db.Query(ctx, query, req.TrackerId, tracker_id_flag, before, req.Limit)
	} else {
		query = `SELECT id, tracker_id,server_flag,response, response_time,command,command_time FROM gt06_command_response WHERE (tracker_id = $1 OR $2) AND id < $3 ORDER BY id DESC LIMIT $4`
		rows, err = t.db.Query(ctx, query, req.TrackerId, tracker_id_flag, req.Pointer, req.Limit)
	}

	defer rows.Close()
	if err != nil {
		return err
	}

	for rows.Next() {
		cmd := &GT06CmdResponseModel{}
		err := rows.Scan(&cmd.Id, &cmd.TrackerId, &cmd.ServerFlag, &cmd.Response, &cmd.ResponseTime, &cmd.Command, &cmd.CommandTime)
		if err != nil {
			return err
		}
		cmds = append(cmds, cmd)
	}
	*res = cmds
	return nil
}

type CloseTrackerRequest struct {
	TrackerId uint64 `json:"tracker_id"`
}

func (t *Tracker) PurgeTracker(ctx context.Context, req *CloseTrackerRequest, res *common.BasicResponse) {
	if t.gps.PurgeDevice(req.TrackerId) {
		res.Status = 0
	} else {
		res.Status = -1
		res.Message = "device not found"
	}
}

// func (t *Tracker) GetTrackerStatus(ctx context.Context, res *[]*TrackerModel) {

// }

// func (t *Tracker) GetTrackersStatus(ctx context.Context, res *[]gps.ClientStatus) {
// 	*res = t.reg.gsrv.GetClientsStatus()
// }

// func (t *Tracker) GetTrackerStatusDetail(ctx context.Context, req *TrackerStatusDetailRequest, res **gps.ClientStatusDetail) {
// 	*res = t.reg.gsrv.GetClientStatus(req.TId)
// }

func (t *Tracker) GetTrackerLocationHistory(ctx context.Context, req *TrackerHistoryRequestModel, w http.ResponseWriter) error {
	var query string
	var rows pgx.Rows
	var err error
	if req.Limit == 0 || len(req.NSN) > 1 {
		query = `SELECT nsn,latitude,longitude,altitude,speed,gps_timestamp,server_timestamp FROM locations_history WHERE nsn = ANY($1) AND server_timestamp BETWEEN $2 AND $3 ORDER BY server_timestamp ASC`
		rows, err = t.db.Query(ctx, query, req.NSN, req.From, req.To)
	} else {
		query = `SELECT nsn,latitude,longitude,altitude,speed,gps_timestamp,server_timestamp FROM locations_history WHERE nsn = ANY($1)  AND server_timestamp BETWEEN $2 AND $3 ORDER BY server_timestamp ASC LIMIT $4`
		rows, err = t.db.Query(ctx, query, req.NSN, req.From, req.To, req.Limit)
	}

	defer rows.Close()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	buf := new(bytes.Buffer)
	group_cnt := make(map[uint64]uint64)
	count := 0
	flusher, _ := w.(http.Flusher)
	var total_size int64
	for rows.Next() {
		var lat, lon float64
		var alt, speed float32
		var gps_time, server_time time.Time
		var nsn uint64
		err := rows.Scan(&nsn, &lat, &lon, &alt, &speed, &gps_time, &server_time)
		if err != nil {
			return err
		}
		_ = binary.Write(buf, binary.LittleEndian, nsn)
		_ = binary.Write(buf, binary.LittleEndian, lat)
		_ = binary.Write(buf, binary.LittleEndian, lon)
		_ = binary.Write(buf, binary.LittleEndian, alt)
		_ = binary.Write(buf, binary.LittleEndian, speed)
		_ = binary.Write(buf, binary.LittleEndian, gps_time.UnixMilli())
		_ = binary.Write(buf, binary.LittleEndian, server_time.UnixMilli())
		group_cnt[nsn]++

		count++
		if req.Chunk != 0 && count == req.Chunk {
			count = 0
			n, err := buf.WriteTo(w)
			total_size = total_size + n
			flusher.Flush()
			if err != nil {
				return err
			} else {
				t.log.Trace().Str("api", "GetTrackerLocationHistory").Int64("bytes", n).Msg("writing chunk")
				buf.Reset()
			}
		}
	}

	n, err := buf.WriteTo(w)
	total_size = total_size + n
	if err != nil {
		return err
	} else {
		t.log.Trace().Str("api", "GetTrackerLocationHistory").Int64("bytes", n).Msg("writing chunk")
		buf.Reset()
	}

	t.log.Trace().Str("api", "GetTrackerLocationHistory").Int64("bytes_total", total_size).Msg("writing final chunk")

	b, err := json.Marshal(group_cnt)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	if err != nil {
		return err
	}
	meta_len := uint32(len(b))
	err = binary.Write(w, binary.LittleEndian, meta_len)
	if err != nil {
		return err
	}
	return nil
}

type LastKnownLocation struct {
	Location device.Location `json:"location"`
	Status   int             `json:"status"`
}

func (t *Tracker) GetLastKnownLocation(ctx context.Context, req *TrackerIdRequestModel, res *LastKnownLocation) {
	dev, ok := t.gps.GetDevice(req.TrackerId)
	if ok {
		res.Status = 0
		res.Location = dev.Dev.GetLocation()
	} else {

	}
}

type SendCommandReq struct {
	TrackerId  uint64 `json:"tracker_id"`
	Command    string `json:"command"`
	ServerFlag uint32 `json:"server_flag"`
	Serial     int    `json:"serial"`
}

// func (t *Tracker) SendCommand(ctx context.Context, req *SendCommandReq, res *common.BasicResponse) error {
// 	dev, ok := t.gps.GetDevice(req.TrackerId)
// 	if !ok {
// 		res.Status = -1
// 		res.Message = "device not found"

// 		return nil
// 	} else {
// 		gt06dev, ok := dev.Dev.(*gt06.GT06)
// 		if !ok {
// 			res.Status = -1
// 			res.Message = "device is not gt06"
// 			return nil
// 		} else {
// 			_ = gt06dev.SendMessage(req.Command, req.ServerFlag, req.Serial)
// 			// res.Message = err.Error()
// 			// t.reg.log.Error().Err(err).Msg("")
// 			return nil

// 		}
// 	}

// }

type SendCommand2Req struct {
	TrackerId uint64 `json:"tracker_id"`
	Command   string `json:"command"`
	Force     bool   `json:"force"`
}

func (t *Tracker) SendCommand2(ctx context.Context, req *SendCommand2Req, res *common.BasicResponse) error {
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
			pending, err := gt06dev.SendCommand(req.Command, req.Force)
			if pending {
				res.Status = -1
				res.Message = "has pending message, use force flag"
			}
			if err != nil {
				res.Message = err.Error()
				res.Status = -1
			}

			return nil

		}
	}

}
