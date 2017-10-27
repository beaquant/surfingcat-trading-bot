package main

import (
	"fmt"
	"github.com/thebotguys/golang-bittrex-api/bittrex"
	"github.com/markcheno/go-talib"
	"time"
	"net/http"
	"strconv"
	"encoding/json"
)

type PlotPoint struct {
	Date string
	Value string
}
type PlotPoints []PlotPoint




func handleChartBtcUsd(w http.ResponseWriter, r *http.Request) {
	
	err := bittrex.IsAPIAlive()
	if err != nil {
		fmt.Println("Can not reach Bittrex API servers: ", err)
	}
	
	candleSticks, err := bittrex.GetTicks("USDT-BTC", "fiveMin")
	if err != nil {
		fmt.Println("ERROR OCCURRED: ", err)
	}
	fmt.Println("Ticks collected: ", len(candleSticks))
	
	var res PlotPoints
	for _, candle := range candleSticks {
		res = append(res, PlotPoint{time.Time(candle.Timestamp).String(), strconv.FormatFloat(candle.Close, 'f', 6, 64)})
	}

	jsonResponse, _ := json.Marshal(res)
	fmt.Fprintf(w, string(jsonResponse))
}


func handleEmaBtcUsd(w http.ResponseWriter, r *http.Request) {
	
	err := bittrex.IsAPIAlive()
	if err != nil {
		fmt.Println("Can not reach Bittrex API servers: ", err)
	}
	
	candleSticks, err := bittrex.GetTicks("USDT-BTC", "thirtyMin")
	if err != nil {
		fmt.Println("ERROR OCCURRED: ", err)
	}
	
	var closes []float64
	for _, candle := range candleSticks {
		closes = append(closes, candle.Close)
	}
	
	interval, err := strconv.Atoi(r.URL.Query().Get("interval"))
	if err != nil || interval < 5  {
		interval = 5
	}
	fmt.Println("Getting EMA for USDT-BTC (", interval, ")")
	emaData := talib.Ema(closes, interval)

	var res PlotPoints
	for i, emaValue := range emaData {
		res = append(res, PlotPoint{time.Time(candleSticks[i].Timestamp).String(), strconv.FormatFloat(emaValue, 'f', 6, 64)})
	}

	jsonResponse, _ := json.Marshal(res)
	fmt.Fprintf(w, string(jsonResponse))
}


func handleIndicatorChart(w http.ResponseWriter, r *http.Request) {
	indicator := r.URL.Query().Get("name")
	if !stringInSlice(indicator, []string{"ema", "wma", "trima"}) {
		panic("indicator is not recognized")
	}
	market := r.URL.Query().Get("market") //"USDT-BTC"
	interval, err := strconv.Atoi(r.URL.Query().Get("interval"))
	if err != nil || interval < 5  {
		interval = 5
	}
	
	err = bittrex.IsAPIAlive()
	if err != nil {
		fmt.Println("Can not reach Bittrex API servers: ", err)
		panic(err)
	}
		
	candleSticks, err := bittrex.GetTicks(market, "fiveMin")
	if err != nil {
		panic(err)
	}
	
	var closes []float64
	for _, candle := range candleSticks {
		closes = append(closes, candle.Close)
	}	
	
	
	var indicatorData []float64

	fmt.Println("Indicator: ", indicator, market, interval)
	
	switch indicator {
	case "ema": indicatorData = talib.Ema(closes, interval)
	case "wma": indicatorData = talib.Wma(closes, interval)
	case "trima": indicatorData = talib.Trima(closes, interval)
	}
	

	var res PlotPoints
	for i, indicatorValue := range indicatorData {
		res = append(res, PlotPoint{time.Time(candleSticks[i].Timestamp).String(), strconv.FormatFloat(indicatorValue, 'f', 6, 64)})
	}

	jsonResponse, _ := json.Marshal(res)
	fmt.Fprintf(w, string(jsonResponse))
}


func handleTraderStart(w http.ResponseWriter, r *http.Request) {
	market := r.URL.Query().Get("market")

	err := bittrex.IsAPIAlive()
	if err != nil {
		fmt.Println("Can not reach Bittrex API servers: ", err)
		panic(err)
	}
	
	// periods -> ["oneMin", "fiveMin", "thirtyMin", "hour", "day"]
	candleSticks, err := bittrex.GetTicks(market, "fiveMin")
	if err != nil {
		fmt.Println("ERROR OCCURRED: ", err)
		panic(err)
	}

	// listen to ticks
	// spot wma cross
	// buy/sell

	fmt.Println("Trading started at", time.Now().String())
	tickSource := make(chan bittrex.CandleStick)
	actionSource := make(chan MarketAction)
	go func(market string, candles *bittrex.CandleSticks) {
		actionSource <- *strategyWma(market, candles)
		for {
			select {
				case <-time.After(30 * time.Second):
					fmt.Println("Tick", market, time.Now().String())
					go nextTick(market, candles, &tickSource)
				case <-tickSource:
					actionSource <- *strategyWma(market, candles)
				case marketAction := <-actionSource:
					performMarketAction(marketAction)
			}
		}
	}(market, &candleSticks)
	jsonResponse, _ := json.Marshal("OK")
	fmt.Fprintf(w, string(jsonResponse))
}

func handleStrategyTest(w http.ResponseWriter, r *http.Request) {
	market := r.URL.Query().Get("market")
	
	err := bittrex.IsAPIAlive()
	if err != nil {
		fmt.Println("Can not reach Bittrex API servers: ", err)
		panic(err)
	}

	// get data
	candleSticks, err := bittrex.GetTicks(market, "fiveMin")
	if err != nil {
		fmt.Println("ERROR OCCURRED: ", err)
		panic(err)
	}
	
	// test through it
	var result TestingResult
	var lastPrice float64 = 0
	var bottomLine float64 = 0

	for i := 0; i <= len(candleSticks) - 1000; i++ {
		t := candleSticks[0:1000+i]
		marketAction := strategyWma(market, &t)
		if (marketAction != nil) {
			result.Actions = append(result.Actions, *marketAction)
			if (marketAction.Action == MarketActionBuy) {
				lastPrice = marketAction.Price
			} else if marketAction.Action == MarketActionSell {
				if lastPrice > 0 {
					bottomLine = bottomLine + marketAction.Price - lastPrice
					lastPrice = 0
					result.Balances = append(result.Balances, PlotPoint{time.Time(marketAction.Time).String(), strconv.FormatFloat(bottomLine, 'f', 6, 64)})
				}
			}
		}
	}

	result.FinalBalance = bottomLine
	jsonResponse, _ := json.Marshal(result)
	fmt.Fprintf(w, string(jsonResponse))
}
//Strategies
// Floor finder
// Pump resolver
	