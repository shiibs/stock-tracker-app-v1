package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

var (
	symbols = []string{"AAPL", "AMZN", "TSLA", "GOOGL", "NFLX", "PYPL"}

	broadcast = make(chan *BroadcastMessage)

	tempCandles = make(map[string]*TempCandle)

	mu sync.Mutex
)

func main() {
	env := EnvConfig()

	db := DBConn(env)

	finnhubWSConn := connectToFinnhub(env)
	defer finnhubWSConn.Close()

	go handleFinnhubMessage(finnhubWSConn, db)

}

// Connect to finnhub websockets
func connectToFinnhub(env *Env) *websocket.Conn {
	ws, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("wss://ws.finnhub.io?token=%s", env.API_KEY), nil)
	if err != nil {
		panic(err)
	}

	for _, s := range symbols {
		msg, _ := json.Marshal(map[string]interface{}{"type": "subscribe", "symbol": s})
		ws.WriteMessage(websocket.TextMessage, msg)
	}

	return ws
}

func handleFinnhubMessage(ws *websocket.Conn, db *gorm.DB) {
	finnhubMessage := &FinnhubMessage{}

	for {
		if err := ws.ReadJSON(finnhubMessage); err != nil {
			fmt.Println("Error reading message: ", err)
			continue
		}

		if finnhubMessage.Type == "trade" {
			for _, trade := range finnhubMessage.Data {
				processTradeData(&trade, db)
			}
		}

		cutOffTime := time.Now().Add(-20 * time.Minute)
		db.Where("timestamp < ?", cutOffTime).Delete(&Candle{})
	}
}

func processTradeData(trade *TradeData, db *gorm.DB) {
	mu.Lock()
	defer mu.Unlock()

	symbol := trade.Symbol
	price := trade.Price
	volume := float64(trade.Volume)
	timestamp := time.UnixMilli(trade.Timestamp)

	tempCandle, exists := tempCandles[symbol]

	if !exists || timestamp.After(tempCandle.CloseTime) {
		if exists {
			candle := tempCandle.toCandle()

			if err := db.Create(candle).Error; err != nil {
				fmt.Println("Error saving the candle to DB: ", err)
			}

			broadcast <- &BroadcastMessage{
				UpdateType: Closed,
				Candle:     candle,
			}
		}

		tempCandle = &TempCandle{
			Symbol:     symbol,
			OpenTime:   timestamp,
			CloseTime:  timestamp.Add(time.Minute),
			OpenPrice:  price,
			ClosePrice: price,
			HighPrice:  price,
			LowPrice:   price,
			Volume:     volume,
		}
	}

	tempCandle.ClosePrice = price
	tempCandle.Volume += volume
	if price > tempCandle.HighPrice {
		tempCandle.HighPrice = price
	}

	if price < tempCandle.LowPrice {
		tempCandle.LowPrice = price
	}

	tempCandles[symbol] = tempCandle

	broadcast <- &BroadcastMessage{
		UpdateType: Live,
		Candle:     tempCandle.toCandle(),
	}
}
