package vehicle

import (
	"log"

	"github.com/jackc/pgx/v4/pgxpool"
	"nuha.dev/gpstracker/internal/gpsv2/server"
)

type Vehicle struct {
	db  *pgxpool.Pool
	gps *server.Server
	log log.Logger
}

type AddNewVehicleRequest struct {
	Name      string
	Label     map[string]string
	TrackerId uint64
}

func (v *Vehicle) AddNewVehicle() {

}
