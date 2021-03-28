package h02

import (
	"errors"
	"strconv"
	"time"
)

type H02GPSMessage struct {
	Latitude  float64
	Longitude float64
	Timestamp time.Time
}

var ErrBadFrame = errors.New("Bad frame")

func ParseGPSMessage(param []string) (*H02GPSMessage, error) {
	m := &H02GPSMessage{}
	dd, err := getDegree(param[5][:2], param[5][2:])
	if err != nil {
		return nil, err
	}
	if param[6] == "S" {
		m.Latitude = 0 - dd
	} else {
		m.Latitude = dd
	}

	dd, err = getDegree(param[7][:3], param[7][3:])
	if err != nil {
		return nil, err
	}
	if param[8] == "E" {
		m.Longitude = dd
	} else {
		m.Longitude = 0 - dd
	}

	hms, err := parseDT(param[3])
	if err != nil {
		return nil, err
	}
	dmy, err := parseDT(param[11])
	if err != nil {
		return nil, err
	}
	m.Timestamp = time.Date(dmy[2]+2000, time.Month(dmy[1]), dmy[0], hms[0], hms[1], hms[2], 0, time.UTC)
	return m, nil
}

func getDegree(d, m string) (float64, error) {
	dd, err := strconv.Atoi(d)
	if err != nil {
		return 0, ErrBadFrame
	}
	mm, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return 0, ErrBadFrame
	}
	return float64(dd) + mm/60, nil
}

func parseDT(p string) ([]int, error) {
	p1, err := strconv.Atoi(p[:2])
	if err != nil {
		return nil, err
	}
	p2, err := strconv.Atoi(p[2:4])
	if err != nil {
		return nil, err
	}
	p3, err := strconv.Atoi(p[4:6])
	if err != nil {
		return nil, err
	}
	return []int{p1, p2, p3}, nil
}
