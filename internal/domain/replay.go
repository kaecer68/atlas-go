package domain

import "time"

type DailyBar struct {
	Date   time.Time
	Symbol string
	Name   string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
	Source string
}
