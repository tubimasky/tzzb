package api

// Position 是一条持仓明细。
type Position struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Count      Num    `json:"count"`
	Cost       Num    `json:"cost"`
	Price      Num    `json:"price"`
	Value      Num    `json:"value"`
	HoldRate   Num    `json:"hold_rate"`   // 持仓收益率(小数)
	HoldProfit Num    `json:"hold_profit"` // 持仓盈亏
	WProfit    Num    `json:"w_profit"`    // 本周盈亏（不是当日！当日看账户级 card.now_profit）
	Market     string `json:"market"`      // 1=深 2=沪（股票/ETF 有；基金常空）
}

// LiveView 按插件 stockCalc 重算后的展示字段。
type LiveView struct {
	Price      float64
	Value      float64
	HoldProfit float64
	HoldRate   float64 // 持仓收益率 %，对应插件 浮动盈亏比 ccykb
	DayProfit  float64
	DayRate    float64 // 今日涨幅 %，对应插件 涨幅 zf = (现价−昨收)/昨收
	DayYKRate  float64 // 当日盈亏比 %，对应插件 drykb（内部/合计用）
	SellAmount float64 // 当日卖出金额，合计盈亏比分母用
	DayOK      bool
	Live       bool
}

// RecomputeLive 对齐插件 stockCalc（auto 账户口径）：
//
//	现价       = 行情 xianjia（无则回落接口价）
//	市值       = 数量 × 现价
//	今日盈亏   = 当日买卖收益 + (现价−昨收)×(持仓−今日买入−正 remainchg) − 费用 + 速记
//	今日涨幅   = (现价 − 昨收) / 昨收          ← 插件 hq.zf，不是盈亏比
//	当日盈亏比 = 今日盈亏 / (市值 − 今日盈亏 + 卖出额)  ← 插件 drykb；合计行同口径
//	持仓盈亏   = 接口 hold_profit + (现价 − 接口价) × 数量
//	收益率     = 持仓盈亏 / (成本价 × 数量)   ← 插件 浮动盈亏比
func RecomputeLive(count, cost, apiPrice, apiValue, apiHold, apiHoldRate float64, q *Quote, trades []DayTrade) LiveView {
	v := LiveView{
		Price:      apiPrice,
		Value:      apiValue,
		HoldProfit: apiHold,
		HoldRate:   apiHoldRate * 100,
	}
	if count == 0 {
		return v
	}
	if v.Value == 0 && apiPrice > 0 {
		v.Value = apiPrice * count
	}

	price := apiPrice
	prev := 0.0
	if q != nil {
		if q.Price > 0 {
			price = q.Price
		}
		if q.PrevClose > 0 {
			prev = q.PrevClose
		}
	}
	v.Price = price
	if price > 0 {
		v.Value = count * price
		v.Live = apiPrice == 0 || price != apiPrice
	}

	// 持仓盈亏：接口 + 价差修正（插件 auto）
	if apiPrice > 0 && price > 0 {
		v.HoldProfit = apiHold + (price-apiPrice)*count
	} else if price > 0 && cost > 0 {
		v.HoldProfit = (price - cost) * count
		v.Live = true
	}
	// 浮动盈亏比 = 持仓盈亏 / 成本(成本价×数量)；与插件 Z=I/N 一致
	costBasis := cost * count
	if costBasis != 0 {
		v.HoldRate = v.HoldProfit / costBasis * 100
	} else if apiHoldRate != 0 {
		v.HoldRate = apiHoldRate * 100
	}

	// 今日涨幅（价格涨跌幅）：有昨收即可，与是否成交无关
	if price > 0 && prev > 0 {
		v.DayRate = (price - prev) / prev * 100
		v.DayOK = true
	}

	// 今日盈亏：无昨收且无成交则无法算
	if price <= 0 {
		return v
	}
	if prev <= 0 && len(trades) == 0 {
		return v
	}
	prevForPnL := prev
	if prevForPnL <= 0 {
		// 有成交但无昨收：昨收差价段为 0
		prevForPnL = price
	}

	var h, m, feeSum, buyCount, flashG, remainC float64
	var hasFlash bool
	var costChg float64
	for _, t := range trades {
		feeSum += t.Fee
		n := t.Price
		o := t.Count
		switch t.OpType {
		case OpBuy:
			buyCount += o
			h += o * (price - n) // 买入收益 = 数量×(现价−买入价)
		case OpSell:
			h += o * (n - prevForPnL) // 卖出收益 = 数量×(卖出价−昨收)
			m += o * n
		case OpCollateralIn:
			buyCount += o
		case OpCollateralOut:
			h += o * (price - prevForPnL)
			m += o * price
		case OpFlash:
			hasFlash = true
			costChg = t.CostChange
			remainC = t.RemainChange
		}
	}
	if hasFlash {
		if costChg < 0 {
			flashG = absFloat(remainC)*price + costChg
		} else {
			flashG = costChg - absFloat(remainC)*prevForPnL
		}
	}
	// 今日盈亏 = 买卖收益 + (现价−昨收)×(持仓−买入−正remain) − 费用 + 速记
	adjRemain := 0.0
	if remainC > 0 {
		adjRemain = remainC
	}
	h += (price-prevForPnL)*(count-buyCount-adjRemain) - feeSum + flashG
	v.DayProfit = h
	v.SellAmount = m
	v.DayOK = true

	// 当日盈亏比（插件 drykb，合计行用）
	den := v.Value - h + m
	if den != 0 {
		v.DayYKRate = h / den * 100
	}
	return v
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
