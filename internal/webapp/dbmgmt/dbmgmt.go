package dbmgmt

import (
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
)

type DbMgmt struct {
	db  *pgxpool.Pool
	log log.Logger
}

type Statistic struct {
	
	Chunks []Chunk `json:"chunks"`
}

type Chunk struct {
	Name string `json:"name"`
}

func (db *DbMgmt) GetStatistic() {

}

func (db *DbMgmt) DecompressChunk() {

}

func (db *DbMgmt) CompressChunk() {

}
