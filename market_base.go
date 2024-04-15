package marketdata

import (
	"github.com/Hongssd/mybinanceapi"
	"github.com/Hongssd/myokxapi"
	jsoniter "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var log = logrus.New()
var binance = mybinanceapi.MyBinance{}
var okx = myokxapi.MyOkx{}

func SetLogger(logger *logrus.Logger) {
	log = logger
}

type BinanceMarketData struct {
	mybinanceapi.Client
	*BinanceOrderBook
	*BinanceKline
}

func NewBinanceMarketDataDefault() *BinanceMarketData {
	return NewBinanceMarketData("", "")
}
func NewBinanceMarketData(apiKey, apiSecrey string) *BinanceMarketData {
	return &BinanceMarketData{
		Client: mybinanceapi.Client{
			ApiKey:    apiKey,
			ApiSecret: apiSecrey,
		},
	}
}
func (bm *BinanceMarketData) InitBinanceOrderBook(config BinanceOrderBookConfig) error {
	b := &BinanceOrderBook{}
	b.SpotOrderBook = b.newBinanceOrderBookBase(config.SpotConfig)
	b.SpotOrderBook.AccountType = BINANCE_SPOT
	b.SpotOrderBook.parent = b
	b.FutureOrderBook = b.newBinanceOrderBookBase(config.FutureConfig)
	b.FutureOrderBook.AccountType = BINANCE_FUTURE
	b.FutureOrderBook.parent = b
	b.SwapOrderBook = b.newBinanceOrderBookBase(config.SwapConfig)
	b.SwapOrderBook.AccountType = BINANCE_SWAP
	b.SwapOrderBook.parent = b
	b.init()
	bm.BinanceOrderBook = b
	b.parent = bm
	return nil
}

func (bm *BinanceMarketData) InitBinanceKline(config BinanceKlineConfig) error {
	b := &BinanceKline{}
	b.SpotKline = b.newBinanceKlineBase(config.SpotConfig)
	b.SpotKline.AccountType = BINANCE_SPOT
	b.SpotKline.parent = b
	b.FutureKline = b.newBinanceKlineBase(config.FutureConfig)
	b.FutureKline.AccountType = BINANCE_FUTURE
	b.FutureKline.parent = b
	b.SwapKline = b.newBinanceKlineBase(config.SwapConfig)
	b.SwapKline.AccountType = BINANCE_SWAP
	b.SwapKline.parent = b
	bm.BinanceKline = b
	b.parent = bm
	b.init()
	return nil
}

type OkxMarketData struct {
	myokxapi.Client
	*OkxOrderBook
}

func NewOkxMarketDataDefault() *OkxMarketData {
	return NewOkxMarketData("", "", "")
}
func NewOkxMarketData(APIKey, SecretKey, Passphrase string) *OkxMarketData {
	return &OkxMarketData{
		Client: myokxapi.Client{
			APIKey:     APIKey,
			SecretKey:  SecretKey,
			Passphrase: Passphrase,
		},
	}
}
func (om *OkxMarketData) InitOkxOrderBook(config OkxOrderBookConfig) error {
	o := om.newOkxOrderBook(config)
	om.OkxOrderBook = o
	o.init()
	return nil
}
