package main

import "time"

// Candle struct represents a single OHLC (High, Low, Open, Close) candle
type Candle struct {
	Symbol    string    `json:"symbol"`
	Open      float64   `json:"open"`
	Close     float64   `json:"close"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Timestamp time.Time `json:"timestamp"`
}

type FinnhubMessage struct {
	Data []TradeData `json:"data"`
	Type string      `json:"type"`
}

type TradeData struct {
	Close     []string `json:"c"`
	Price     float64  `json:"p"`
	Symbol    string   `json:"s"`
	Timestamp int64    `json:"t"`
	Volume    int      `json:"v"`
}

type TempCandle struct {
	Symbol     string
	OpenTime   time.Time
	CloseTime  time.Time
	OpenPrice  float64
	ClosePrice float64
	HighPrice  float64
	LowPrice   float64
	Volume     float64
}

type BroadcastMessage struct {
	UpdateType UpdateType `json:"updateType"`
	Candle     *Candle    `json:"candle"`
}

type UpdateType string

const (
	Live   UpdateType = "live"
	Closed UpdateType = "closed"
)

func (tc *TempCandle) toCandle() *Candle {
	return &Candle{
		Symbol:    tc.Symbol,
		Open:      tc.OpenPrice,
		Close:     tc.ClosePrice,
		High:      tc.HighPrice,
		Low:       tc.LowPrice,
		Timestamp: tc.CloseTime,
	}
}
