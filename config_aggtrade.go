package marketdata

type BinanceAggTradeConfig struct {
	SpotConfig   BinanceAggTradeConfigBase
	FutureConfig BinanceAggTradeConfigBase
	SwapConfig   BinanceAggTradeConfigBase
}

type BinanceAggTradeConfigBase struct {
	PerConnSubNum int64 //每条链接订阅的数量
	PerSubMaxLen  int   //每条链接每次订阅的最大数量
}

type OkxAggTradeConfig struct {
	PerConnSubNum int64 //每条链接订阅的数量
	PerSubMaxLen  int   //每条链接每次订阅的最大数量
}
