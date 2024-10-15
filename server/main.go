package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

var (
	symbols = []string{"AAPL", "AMZN", "TSLA", "GOOGL", "NFLX", "PYPL"}

	broadcast = make(chan *BroadcastMessage)

	tempCandles = make(map[string]*TempCandle)

	clientConns = make(map[*websocket.Conn]string)

	mu sync.Mutex
)

func main() {
	env := EnvConfig()

	db := DBConn(env)

	finnhubWSConn := connectToFinnhub(env)
	defer finnhubWSConn.Close()

	go handleFinnhubMessage(finnhubWSConn, db)

	go broadcastUpdates()

	http.HandleFunc("/ws", WSHandler)

	http.HandleFunc("/stocks-history", func(w http.ResponseWriter, r *http.Request) {
		StockHistoryHandler(w, r, db)
	})

	http.HandleFunc("stock-candles", func(w http.ResponseWriter, r *http.Request) {
		CandleHandler(w, r, db)
	})

}

// Fetch all past candles for all of the symbols
func StockHistoryHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var candles []Candle
	db.Order("timestamp asc").Find(&candles)

	groupData := make(map[string][]Candle)

	for _, candle := range candles {
		symbol := candle.Symbol
		groupData[symbol] = append(groupData[symbol], candle)
	}

	jsonResponse, _ := json.Marshal(groupData)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResponse)
}

// Fetch all past candles from a specific symbol
func CandleHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	symbol := r.URL.Query().Get("symbol")

	var candles []Candle
	db.Where("symbol= ?", symbol).Order("timestamp asc").Find(&candles)

	jsonCandles, _ := json.Marshal(candles)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonCandles)
}

// Websocket endpoint to connect clients to the latest updates on the symbol they're subscribed to
func WSHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade incoming GET request into a Websocket connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Close ws connection & unregister the client when they disconnect
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Panicln("Failed to upgrade connection: ", err)
	}

	defer conn.Close()
	defer func() {
		delete(clientConns, conn)
		log.Println("Client disconnected")
	}()

	// Register the new client to the symbol they're subscribing to
	for {
		_, symbol, err := conn.ReadMessage()
		clientConns[conn] = string(symbol)
		log.Println("New client connected")

		if err != nil {
			log.Println("Error reading from the client: ", err)
			break
		}
	}
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

// Handle Finnhub's WebSockets incoming messages
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

// Process each trade and update or create temporary candles
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

// Send candle updates to clients connected every 1 second at maximum, unless it's a closed candle
func broadcastUpdates() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var latestUpdate *BroadcastMessage

	for {
		select {
		case update := <-broadcast:
			if update.UpdateType == Closed {
				broadcastToClients(update)
			} else {
				latestUpdate = update
			}

		case <-ticker.C:
			if latestUpdate != nil {
				broadcastToClients(latestUpdate)
			}
			latestUpdate = nil
		}
	}
}

// Broadcast updates to clients
func broadcastToClients(update *BroadcastMessage) {
	jsonUpdate, _ := json.Marshal(update)

	for clientConn, symbol := range clientConns {
		if update.Candle.Symbol == symbol {
			err := clientConn.WriteMessage(websocket.TextMessage, jsonUpdate)
			if err != nil {
				log.Panicln("Error sending message to client: ", err)
				clientConn.Close()
				delete(clientConns, clientConn)
			}
		}
	}
}
