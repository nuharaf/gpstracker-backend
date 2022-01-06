package record

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type RecordApi struct {
	db *pgxpool.Pool
}

type GetRecordRequest struct {
	FsnList []string  `json:"fsn_list"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
}

type GetRecordResponse struct {
	Data map[string][]DeviceRecord
}

type DeviceRecord struct {
	Fsn             string `json:"fsn"`
	Latitude        float64
	Longitude       float64
	Altitude        float32
	Speed           float32
	GpsTimestamp    time.Time
	ServerTimestamp time.Time
}

func (r *RecordApi) GetRecord(ctx context.Context, req *GetRecordRequest, res *GetRecordResponse) error {
	query := `SELECT fsn,longitude,latitude,altitude,speed,gps_timestamp,server_timestamp FROM locations WHERE fsn IN $1`
	res.Data = make(map[string][]DeviceRecord)
	for _, fsn := range req.FsnList {
		res.Data[fsn] = make([]DeviceRecord, 0)
	}
	rows, err := r.db.Query(ctx, query, req.FsnList)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		} else {
			return err
		}
	}
	for rows.Next() {
		rec := DeviceRecord{}
		err := rows.Scan(&rec.Fsn, &rec.Longitude, &rec.Latitude, rec.Altitude, rec.Speed, rec.GpsTimestamp, rec.ServerTimestamp)
		if err != nil {
			return err
		}
		res.Data[rec.Fsn] = append(res.Data[rec.Fsn], rec)
	}
	return nil
}
