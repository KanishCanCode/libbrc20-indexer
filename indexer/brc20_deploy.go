package indexer

import (
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/unisat-wallet/libbrc20-indexer/conf"
	"github.com/unisat-wallet/libbrc20-indexer/constant"
	"github.com/unisat-wallet/libbrc20-indexer/decimal"
	"github.com/unisat-wallet/libbrc20-indexer/model"
	"github.com/unisat-wallet/libbrc20-indexer/utils"
)

func (g *BRC20ModuleIndexer) ProcessDeploy(data *model.InscriptionBRC20Data) error {
	body := new(model.InscriptionBRC20DeployContent)
	if err := body.Unmarshal(data.ContentBody); err != nil {
		return nil
	}

	uniqueLowerTicker, err := utils.GetValidUniqueLowerTickerTicker(body.BRC20Tick)
	if err != nil {
		return nil
	}

	if err := g.validateTicker(body, uniqueLowerTicker, data); err != nil {
		return err
	}

	tinfo := g.initializeTickInfo(body, data)
	if err := g.validateTickDetails(tinfo, body); err != nil {
		return err
	}

	tokenInfo := g.storeTokenInfo(uniqueLowerTicker, tinfo)
	g.updateUserAndTokenData(data, uniqueLowerTicker, tokenInfo)

	return nil
}

func (g *BRC20ModuleIndexer) validateTicker(body *model.InscriptionBRC20DeployContent, uniqueLowerTicker string, data *model.InscriptionBRC20Data) error {
	if len(body.BRC20Tick) == 5 {
		if body.BRC20SelfMint != "true" || data.Height < conf.ENABLE_SELF_MINT_HEIGHT {
			return nil
		}
	}

	if conf.TICKS_ENABLED != "" {
		if strings.Contains(uniqueLowerTicker, " ") || !strings.Contains(conf.TICKS_ENABLED, uniqueLowerTicker) {
			return nil
		}
	}

	if _, exists := g.InscriptionsTickerInfoMap[uniqueLowerTicker]; exists {
		return nil
	}

	if body.BRC20Max == "" {
		log.Printf("deploy, but max missing. ticker: %s", uniqueLowerTicker)
		return errors.New("deploy, but max missing")
	}

	return nil
}

func (g *BRC20ModuleIndexer) initializeTickInfo(body *model.InscriptionBRC20DeployContent, data *model.InscriptionBRC20Data) *model.InscriptionBRC20TickInfo {
	tinfo := model.NewInscriptionBRC20TickInfo(body.BRC20Tick, body.Operation, data)
	tinfo.Data.BRC20Max = body.BRC20Max
	tinfo.Data.BRC20Limit = body.BRC20Limit
	tinfo.Data.BRC20Decimal = body.BRC20Decimal
	tinfo.Data.BRC20Minted = "0"
	tinfo.InscriptionNumberStart = data.InscriptionNumber

	if len(body.BRC20Tick) == 5 && body.BRC20SelfMint == "true" {
		tinfo.SelfMint = true
		tinfo.Data.BRC20SelfMint = "true"
	}

	return tinfo
}

func (g *BRC20ModuleIndexer) validateTickDetails(tinfo *model.InscriptionBRC20TickInfo, body *model.InscriptionBRC20DeployContent) error {
	decimalValue, err := strconv.ParseUint(tinfo.Data.BRC20Decimal, 10, 64)
	if err != nil || decimalValue > 18 {
		log.Printf("deploy, but dec invalid. ticker: %s, dec: %s", tinfo.Ticker, tinfo.Data.BRC20Decimal)
		return errors.New("deploy, but dec invalid")
	}
	tinfo.Decimal = uint8(decimalValue)

	max, err := decimal.NewDecimalFromString(body.BRC20Max, int(tinfo.Decimal))
	if err != nil || max.Sign() < 0 || max.IsOverflowUint64() {
		log.Printf("deploy, but max invalid. ticker: %s, max: '%s'", tinfo.Ticker, body.BRC20Max)
		return errors.New("deploy, but max invalid")
	}

	if max.Sign() == 0 && !tinfo.SelfMint {
		return errors.New("deploy, but max invalid (0)")
	}

	if max.Sign() == 0 {
		tinfo.Max = max.GetMaxUint64()
	} else {
		tinfo.Max = max
	}

	lim, err := decimal.NewDecimalFromString(tinfo.Data.BRC20Limit, int(tinfo.Decimal))
	if err != nil || lim.Sign() < 0 || lim.IsOverflowUint64() {
		log.Printf("deploy, but limit invalid. ticker: %s, limit: '%s'", tinfo.Ticker, tinfo.Data.BRC20Limit)
		return errors.New("deploy, but limit invalid")
	}

	if lim.Sign() == 0 && !tinfo.SelfMint {
		return errors.New("deploy, but limit invalid (0)")
	}

	if lim.Sign() == 0 {
		tinfo.Limit = lim.GetMaxUint64()
	} else {
		tinfo.Limit = lim
	}

	return nil
}

func (g *BRC20ModuleIndexer) storeTokenInfo(uniqueLowerTicker string, tinfo *model.InscriptionBRC20TickInfo) *model.BRC20TokenInfo {
	tokenInfo := &model.BRC20TokenInfo{
		Ticker: body.BRC20Tick,
		Deploy: tinfo,
	}

	g.InscriptionsTickerInfoMap[uniqueLowerTicker] = tokenInfo

	return tokenInfo
}

func (g *BRC20ModuleIndexer) updateUserAndTokenData(data *model.InscriptionBRC20Data, uniqueLowerTicker string, tokenInfo *model.BRC20TokenInfo) {
	tokenBalance := &model.BRC20TokenBalance{
		Ticker:  body.BRC20Tick,
		PkScript: data.PkScript,
	}

	if g.EnableHistory {
		historyObj := model.NewBRC20History(constant.BRC20_HISTORY_TYPE_N_INSCRIBE_DEPLOY, true, false, tinfo, nil, data)
		history := g.UpdateHistoryHeightAndGetHistoryIndex(historyObj)

		tokenBalance.History = append(tokenBalance.History, history)
		tokenInfo.History = append(tokenInfo.History, history)

		userHistory := g.GetBRC20HistoryByUser(string(data.PkScript))
		userHistory.History = append(userHistory.History, history)

		g.AllHistory = append(g.AllHistory, history)
	}

	tokenInfo.UpdateHeight = data.Height

	userTokens := g.getUserTokens(string(data.PkScript))
	userTokens[uniqueLowerTicker] = tokenBalance

	tokenUsers := make(map[string]*model.BRC20TokenBalance)
	tokenUsers[string(data.PkScript)] = tokenBalance
	g.TokenUsersBalanceData[uniqueLowerTicker] = tokenUsers

	g.InscriptionsValidBRC20DataMap[data.CreateIdxKey] = tinfo.Data
}

func (g *BRC20ModuleIndexer) getUserTokens(pkScript string) map[string]*model.BRC20TokenBalance {
	if tokens, exists := g.UserTokensBalanceData[pkScript]; exists {
		return tokens
	}

	userTokens := make(map[string]*model.BRC20TokenBalance)
	g.UserTokensBalanceData[pkScript] = userTokens
	return userTokens
}
