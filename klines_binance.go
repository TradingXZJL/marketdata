package marketdata

import (
	"errors"
	"fmt"
	"github.com/Hongssd/mybinanceapi"
	"github.com/robfig/cron/v3"
	"sync/atomic"
	"time"
)

type BinanceKline struct {
	parent      *BinanceMarketData
	SpotKline   *binanceKlineBase
	FutureKline *binanceKlineBase
	SwapKline   *binanceKlineBase
}

type binanceKlineBase struct {
	serverTimeDelta int64
	parent          *BinanceKline
	perConnSubNum   int64
	Exchange        Exchange
	AccountType     BinanceAccountType
	KlineMap        *MySyncMap[string, *Kline]                                           //symbol_interval->last kline
	WsClientListMap *MySyncMap[*mybinanceapi.WsStreamClient, *int64]                     //ws client->subscribe count
	WsClientMap     *MySyncMap[string, *mybinanceapi.WsStreamClient]                     //symbol_interval->ws client
	SubMap          *MySyncMap[string, *mybinanceapi.Subscription[mybinanceapi.WsKline]] //symbol_interval->subscribe
	CallBackMap     *MySyncMap[string, func(kline *Kline, err error)]                    //symbol_interval->callback
}

func (b *BinanceKline) getBaseMapFromAccountType(accountType BinanceAccountType) (*binanceKlineBase, error) {
	switch accountType {
	case BINANCE_SPOT:
		return b.SpotKline, nil
	case BINANCE_FUTURE:
		return b.FutureKline, nil
	case BINANCE_SWAP:
		return b.SwapKline, nil
	}
	return nil, ErrorAccountType
}

func (b *BinanceKline) newBinanceKlineBase(config BinanceKlineConfigBase) *binanceKlineBase {
	return &binanceKlineBase{
		Exchange:        BINANCE,
		perConnSubNum:   config.PerConnSubNum,
		KlineMap:        GetPointer(NewMySyncMap[string, *Kline]()),
		WsClientListMap: GetPointer(NewMySyncMap[*mybinanceapi.WsStreamClient, *int64]()),
		WsClientMap:     GetPointer(NewMySyncMap[string, *mybinanceapi.WsStreamClient]()),
		SubMap:          GetPointer(NewMySyncMap[string, *mybinanceapi.Subscription[mybinanceapi.WsKline]]()),
		CallBackMap:     GetPointer(NewMySyncMap[string, func(kline *Kline, err error)]()),
	}
}

// 封装好的获取K线方法
func (b *BinanceKline) GetLastKline(BinanceAccountType BinanceAccountType, symbol, interval string) (*Kline, error) {
	symbolKey := symbol + "_" + interval

	bmap, err := b.getBaseMapFromAccountType(BinanceAccountType)
	if err != nil {
		return nil, err
	}
	kline, ok := bmap.KlineMap.Load(symbolKey)
	if !ok {
		err := fmt.Errorf("%s symbol:%s kline not found", BinanceAccountType, symbol)
		return nil, err
	}
	return kline, nil
}

// 获取当前或新建ws客户端
func (b *BinanceKline) GetCurrentOrNewWsClient(accountType BinanceAccountType) (*mybinanceapi.WsStreamClient, error) {
	var WsClientListMap *MySyncMap[*mybinanceapi.WsStreamClient, *int64]
	perConnSubNum := int64(0)
	switch accountType {
	case BINANCE_SPOT:
		WsClientListMap = b.SpotKline.WsClientListMap
		perConnSubNum = b.SpotKline.perConnSubNum
	case BINANCE_FUTURE:
		WsClientListMap = b.FutureKline.WsClientListMap
		perConnSubNum = b.FutureKline.perConnSubNum
	case BINANCE_SWAP:
		WsClientListMap = b.SwapKline.WsClientListMap
		perConnSubNum = b.SwapKline.perConnSubNum
	default:
		return nil, ErrorAccountType
	}

	// log.Info(WsClientList)

	var wsClient *mybinanceapi.WsStreamClient
	var err error
	WsClientListMap.Range(func(k *mybinanceapi.WsStreamClient, v *int64) bool {
		if *v < perConnSubNum {
			wsClient = k
			return false
		}

		return true
	})

	if wsClient == nil {
		//新建链接
		switch accountType {
		case BINANCE_SPOT:
			wsClient = &binance.NewSpotWsStreamClient().WsStreamClient
		case BINANCE_FUTURE:
			wsClient = &binance.NewFutureWsStreamClient().WsStreamClient
		case BINANCE_SWAP:
			wsClient = &binance.NewSwapWsStreamClient().WsStreamClient
		}
		err = wsClient.OpenConn()
		if err != nil {
			return nil, err
		}
		initCount := int64(0)
		WsClientListMap.Store(wsClient, &initCount)
		if WsClientListMap.Length() > 1 {
			log.Infof("当前链接订阅权重已用完，建立新的Ws链接，当前链接数:%d ...", WsClientListMap.Length())
		} else {
			log.Info("首次建立新的Ws链接...")
		}
	}
	return wsClient, nil
}
func (b *binanceKlineBase) subscribeBinanceKline(binanceWsClient *mybinanceapi.WsStreamClient, symbol, interval string, callback func(kline *Kline, err error)) error {
	return b.subscribeBinanceKlineMultiple(binanceWsClient, []string{symbol}, []string{interval}, callback)
}

// 订阅币安K线底层执行
func (b *binanceKlineBase) subscribeBinanceKlineMultiple(binanceWsClient *mybinanceapi.WsStreamClient, symbols, intervals []string, callback func(kline *Kline, err error)) error {

	binanceSub, err := binanceWsClient.SubscribeKlineMultiple(symbols, intervals)
	if err != nil {
		log.Error(err)
		return err
	}

	for _, symbol := range symbols {
		for _, interval := range intervals {
			symbolKey := symbol + "_" + interval
			b.WsClientMap.Store(symbolKey, binanceWsClient)
			b.SubMap.Store(symbolKey, binanceSub)
			b.CallBackMap.Store(symbolKey, callback)
		}
	}

	go func() {
		for {
			select {
			case err := <-binanceSub.ErrChan():
				log.Error(err)
				if callback != nil {
					callback(nil, err)
				}
			case result := <-binanceSub.ResultChan():
				symbolKey := result.Symbol + "_" + result.Interval
				//保存至Kline
				kline := &Kline{
					Timestamp:            result.Timestamp + b.serverTimeDelta,
					Exchange:             b.Exchange.String(),
					AccountType:          b.AccountType.String(),
					Symbol:               result.Symbol,
					Interval:             result.Interval,
					StartTime:            result.StartTime,
					Open:                 result.Open,
					High:                 result.High,
					Low:                  result.Low,
					Close:                result.Close,
					Volume:               result.Volume,
					CloseTime:            result.CloseTime,
					TransactionVolume:    result.TransactionVolume,
					TransactionNumber:    result.TransactionNumber,
					BuyTransactionVolume: result.BuyTransactionVolume,
					BuyTransactionAmount: result.BuyTransactionAmount,
					Confirm:              result.Confirm,
				}
				b.KlineMap.Store(symbolKey, kline)
				if callback != nil {
					callback(kline, nil)
				}
			case <-binanceSub.CloseChan():
				log.Info("订阅已关闭: ", binanceSub.Params)
				return
			}
		}
	}()

	currentCount := int64(len(symbols) * len(intervals))
	count, ok := b.WsClientListMap.Load(binanceWsClient)
	if !ok {
		initCount := currentCount
		b.WsClientListMap.Store(binanceWsClient, &initCount)
	}
	atomic.AddInt64(count, currentCount)

	return nil
}

// 取消订阅币安K线
func (b *binanceKlineBase) UnSubscribeBinanceKline(symbol, interval string) error {
	symbolKey := symbol + "_" + interval
	binanceSub, ok := b.SubMap.Load(symbolKey)
	if !ok {
		return nil
	}
	return binanceSub.Unsubscribe()
}

// 订阅K线
func (b *BinanceKline) SubscribeKline(accountType BinanceAccountType, symbol, interval string) error {
	return b.SubscribeKlineWithCallBack(accountType, symbol, interval, nil)
}

// 批量订阅K线
func (b *BinanceKline) SubscribeKlines(accountType BinanceAccountType, symbols, intervals []string) error {
	return b.SubscribeKlinesWithCallBack(accountType, symbols, intervals, nil)
}

// 订阅K线并带上回调
func (b *BinanceKline) SubscribeKlineWithCallBack(accountType BinanceAccountType, symbol, interval string, callback func(kline *Kline, err error)) error {
	client, err := b.GetCurrentOrNewWsClient(accountType)
	if err != nil {
		return err
	}

	switch accountType {
	case BINANCE_SPOT:
		err = b.SpotKline.subscribeBinanceKline(client, symbol, interval, callback)
	case BINANCE_FUTURE:
		err = b.FutureKline.subscribeBinanceKline(client, symbol, interval, callback)
	case BINANCE_SWAP:
		err = b.SwapKline.subscribeBinanceKline(client, symbol, interval, callback)
	}

	return err
}

// 批量订阅K线并带上回调
func (b *BinanceKline) SubscribeKlinesWithCallBack(accountType BinanceAccountType, symbols, intervals []string, callback func(kline *Kline, err error)) error {
	log.Infof("开始订阅%s，交易对数:%d, 周期数:%d, 总订阅数:%d", accountType, len(symbols), len(intervals), len(symbols)*len(intervals))

	var currentBinanceKlineBase *binanceKlineBase

	switch accountType {
	case BINANCE_SPOT:
		currentBinanceKlineBase = b.SpotKline
	case BINANCE_FUTURE:
		currentBinanceKlineBase = b.FutureKline
	case BINANCE_SWAP:
		currentBinanceKlineBase = b.SwapKline
	default:
		return ErrorAccountType
	}

	//订阅总数超过LEN次，分批订阅
	LEN := 100
	if len(symbols)*len(intervals) > LEN {
		for i := 0; i < len(symbols); i += LEN / len(intervals) {
			end := i + LEN/len(intervals)
			if end > len(symbols) {
				end = len(symbols)
			}
			tempSymbols := symbols[i:end]
			client, err := b.GetCurrentOrNewWsClient(accountType)
			if err != nil {
				return err
			}
			err = currentBinanceKlineBase.subscribeBinanceKlineMultiple(client, tempSymbols, intervals, callback)
			if err != nil {
				return err
			}

			currentCount, ok := currentBinanceKlineBase.WsClientListMap.Load(client)
			if !ok {
				return errors.New("WsClientListMap Load error")
			}
			log.Infof("K线%s分批订阅成功，此次订阅交易对:%v, 总数:%d，当前链接总订阅数:%d, 等待1秒后继续订阅...", accountType, tempSymbols, len(tempSymbols)*len(intervals), *currentCount)

			time.Sleep(1000 * time.Millisecond)
		}

	} else {
		client, err := b.GetCurrentOrNewWsClient(accountType)
		if err != nil {
			return err
		}
		err = currentBinanceKlineBase.subscribeBinanceKlineMultiple(client, symbols, intervals, callback)
		if err != nil {
			return err
		}
	}

	log.Infof("订阅结束，交易对数:%d, 周期数:%d, 总订阅数:%d", len(symbols), len(intervals), len(symbols)*len(intervals))

	return nil
}

// 获取当前服务器时间差
func (b *BinanceKline) GetServerTimeDelta(accountType BinanceAccountType) int64 {
	switch accountType {
	case BINANCE_SPOT:
		return b.SpotKline.serverTimeDelta
	case BINANCE_FUTURE:
		return b.FutureKline.serverTimeDelta
	case BINANCE_SWAP:
		return b.SwapKline.serverTimeDelta
	}
	return 0
}

// 初始化
func (b *BinanceKline) init() {
	mybinanceapi.SetLogger(log)
	c := cron.New(cron.WithSeconds())
	refresh := func() {
		b.SpotKline.RefreshDelta()
		b.FutureKline.RefreshDelta()
		b.SwapKline.RefreshDelta()
	}
	refresh()

	//每隔5秒更新一次服务器时间
	_, err := c.AddFunc("*/5 * * * * *", refresh)
	if err != nil {
		log.Error(err)
		return
	}
	c.Start()
}

func (b *binanceKlineBase) RefreshDelta() {
	switch b.AccountType {
	case BINANCE_SPOT:
		b.serverTimeDelta = b.parent.parent.spotServerTimeDelta
	case BINANCE_FUTURE:
		b.serverTimeDelta = b.parent.parent.futureServerTimeDelta
	case BINANCE_SWAP:
		b.serverTimeDelta = b.parent.parent.swapServerTimeDelta
	}
}
