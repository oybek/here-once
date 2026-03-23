package model

import "time"

type HereOnce struct {
	ID       int64
	Lat      float64
	Lon      float64
	Note     string
	PhotoIDs []string
	Created  time.Time
}
