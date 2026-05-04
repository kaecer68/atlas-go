package domain

import "time"

type DailyBar struct {
	Date   time.Time `json:"date"`
	Symbol string    `json:"symbol"`
	Name   string    `json:"name"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume int64     `json:"volume"`
	Source string    `json:"source"`
}
