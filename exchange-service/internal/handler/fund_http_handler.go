package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
)

type FundHTTPHandler struct {
	cfg *config.Config
	svc *service.FundService
}

func NewFundHTTPHandler(cfg *config.Config, svc *service.FundService) *FundHTTPHandler {
	return &FundHTTPHandler{cfg: cfg, svc: svc}
}

// FundRoutes dispatches /api/v1/funds/... requests.
//
//	GET    /api/v1/funds                            list all funds
//	POST   /api/v1/funds                            create fund (supervisor)
//	GET    /api/v1/funds/{id}                       fund summary + holdings
//	GET    /api/v1/funds/{id}/performance           daily performance series
//	GET    /api/v1/funds/{id}/holdings              holdings list
//	POST   /api/v1/funds/{id}/invest                invest in fund
//	POST   /api/v1/funds/{id}/withdraw              withdraw from fund
//	GET    /api/v1/funds/positions/mine             positions of caller
//	POST   /api/v1/funds/{id}/orders                supervisor places buy-for-fund order metadata (validation only)
//
// Note: manager reassignment on supervisor demotion is owned by employee-service
// (writes investment_funds.manager_id directly via shared DB). No HTTP endpoint
// is exposed here because there are no external callers.
func (h *FundHTTPHandler) FundRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/funds"), "/")
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			h.listFunds(w, r)
		case http.MethodPost:
			h.createFund(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	parts := strings.Split(path, "/")
	switch {
	case len(parts) == 2 && parts[0] == "positions" && parts[1] == "mine":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.listMyPositions(w, r)
	case len(parts) == 1 && parts[0] == "benchmark":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.getBenchmark(w, r)
	case len(parts) == 2 && parts[0] == "dividends" && parts[1] == "run":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.runFundDividends(w, r)
	case len(parts) == 1:
		id, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid fund id"})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.getFund(w, r, uint(id))
	case len(parts) == 2:
		id, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid fund id"})
			return
		}
		switch parts[1] {
		case "performance":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.getPerformance(w, r, uint(id))
		case "holdings":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.listHoldings(w, r, uint(id))
		case "invest":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.investInFund(w, r, uint(id))
		case "withdraw":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.withdrawFromFund(w, r, uint(id))
		case "validate-order":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.validateFundOrder(w, r, uint(id))
		case "statistics":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.getStatistics(w, r, uint(id))
		case "dividends":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.listFundDividends(w, r, uint(id))
		case "dividend-policy":
			if r.Method != http.MethodPut {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.setDividendPolicy(w, r, uint(id))
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

// --- handlers ---

func (h *FundHTTPHandler) listFunds(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuthenticatedHTTP(w, r, h.cfg); !ok {
		return
	}

	funds, err := h.svc.ListFunds()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "neuspesno citanje fondova"})
		return
	}

	items := make([]map[string]interface{}, 0, len(funds))
	for i := range funds {
		summary, err := h.svc.SummariseFund(&funds[i])
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "neuspesan izracun fonda"})
			return
		}
		item := summariseToJSON(summary)
		// Statistics columns for the discovery page (Celina 4). Best-effort: a
		// failure leaves the fund without stats rather than failing the list.
		if stats, err := h.svc.ComputeStatistics(funds[i].ID); err == nil {
			item["statistics"] = statisticsToJSON(stats)
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"funds": items, "count": len(items)})
}

func (h *FundHTTPHandler) createFund(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireSupervisorHTTP(w, claims) {
		return
	}

	var body struct {
		Naziv         string  `json:"naziv"`
		Opis          string  `json:"opis"`
		MinimalniUlog float64 `json:"minimalniUlog"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "neispravan zahtev"})
		return
	}

	fund, err := h.svc.CreateFund(service.CreateFundInput{
		Naziv:         body.Naziv,
		Opis:          body.Opis,
		MinimalniUlog: body.MinimalniUlog,
		ManagerID:     claims.EmployeeID,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	summary, err := h.svc.SummariseFund(fund)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "neuspesan izracun fonda"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"fund": summariseToJSON(summary)})
}

func (h *FundHTTPHandler) getFund(w http.ResponseWriter, r *http.Request, fundID uint) {
	if _, ok := requireAuthenticatedHTTP(w, r, h.cfg); !ok {
		return
	}
	fund, err := h.svc.GetFund(fundID)
	if err != nil {
		if errors.Is(err, service.ErrFundNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "fond nije pronadjen"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	summary, err := h.svc.SummariseFund(fund)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"fund": summariseToJSON(summary)})
}

func (h *FundHTTPHandler) getPerformance(w http.ResponseWriter, r *http.Request, fundID uint) {
	if _, ok := requireAuthenticatedHTTP(w, r, h.cfg); !ok {
		return
	}
	granularity := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("granularity")))
	records, err := h.svc.GetPerformance(fundID, granularity)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	items := make([]map[string]interface{}, 0, len(records))
	for _, rec := range records {
		items = append(items, map[string]interface{}{
			"date":      rec.Date.Format("2006-01-02"),
			"fundValue": rec.FundValue,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"performance": items,
		"count":       len(items),
		"granularity": granularity,
	})
}

func (h *FundHTTPHandler) listHoldings(w http.ResponseWriter, r *http.Request, fundID uint) {
	if _, ok := requireAuthenticatedHTTP(w, r, h.cfg); !ok {
		return
	}
	if _, err := h.svc.GetFund(fundID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "fond nije pronadjen"})
		return
	}
	holdings, err := h.svc.ListFundHoldings(fundID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	items := make([]map[string]interface{}, 0, len(holdings))
	for _, h := range holdings {
		items = append(items, map[string]interface{}{
			"id":                 h.ID,
			"assetId":            h.AssetID,
			"ticker":             h.Asset.Ticker,
			"name":               h.Asset.Name,
			"price":              h.Asset.Price,
			"change":             0.0,
			"volume":             h.Asset.Volume,
			"quantity":           h.Quantity,
			"avgBuyPrice":        h.AvgBuyPrice,
			"acquisitionDate":    h.CreatedAt.Format("2006-01-02"),
			"initialMarginCost":  0.0,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"holdings": items, "count": len(items)})
}

func (h *FundHTTPHandler) investInFund(w http.ResponseWriter, r *http.Request, fundID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	var body struct {
		SourceAccountID uint    `json:"sourceAccountId"`
		Amount          float64 `json:"amount"`
		// Optional: when a supervisor invests on the bank's behalf they may
		// supply this flag; otherwise the request is treated as the caller's
		// own contribution.
		AsBank bool `json:"asBank"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "neispravan zahtev"})
		return
	}

	clientID, clientType, err := fundParticipantIdentity(claims, body.AsBank)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": err.Error()})
		return
	}

	txRec, err := h.svc.InvestInFund(service.InvestInFundInput{
		FundID:          fundID,
		ClientID:        clientID,
		ClientType:      clientType,
		SourceAccountID: body.SourceAccountID,
		Amount:          body.Amount,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"transaction": map[string]interface{}{
			"id":         txRec.ID,
			"fundId":     txRec.FundID,
			"amount":     txRec.Iznos,
			"isInflow":   txRec.IsInflow,
			"timestamp":  txRec.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			"clientType": txRec.ClientType,
		},
	})
}

func (h *FundHTTPHandler) withdrawFromFund(w http.ResponseWriter, r *http.Request, fundID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	var body struct {
		DestinationAccountID uint    `json:"destinationAccountId"`
		Amount               float64 `json:"amount"`
		WithdrawAll          bool    `json:"withdrawAll"`
		AsBank               bool    `json:"asBank"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "neispravan zahtev"})
		return
	}

	clientID, clientType, err := fundParticipantIdentity(claims, body.AsBank)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": err.Error()})
		return
	}

	result, err := h.svc.WithdrawFromFund(service.WithdrawFromFundInput{
		FundID:               fundID,
		ClientID:             clientID,
		ClientType:           clientType,
		DestinationAccountID: body.DestinationAccountID,
		Amount:               body.Amount,
		WithdrawAll:          body.WithdrawAll,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	liquidated := make([]map[string]interface{}, 0, len(result.LiquidatedItems))
	for _, item := range result.LiquidatedItems {
		liquidated = append(liquidated, map[string]interface{}{
			"assetId":     item.AssetID,
			"ticker":      item.Ticker,
			"quantity":    item.Quantity,
			"pricePerRSD": item.PricePerRSD,
			"totalRSD":    item.TotalRSD,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"grossWithdrawn":   result.GrossWithdrawn,
		"commission":       result.Commission,
		"netToAccount":     result.NetToAccount,
		"liquidated":       result.Liquidated,
		"liquidatedItems":  liquidated,
	})
}

func (h *FundHTTPHandler) listMyPositions(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	if claims.TokenSource == "employee" {
		// Supervisors view the funds they manage; agents/basic employees see
		// nothing here.
		if !util.HasPermission(claims, models.PermEmployeeSupervisor) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"funds": []interface{}{}, "count": 0})
			return
		}
		funds, err := h.svc.ListFundsByManager(claims.EmployeeID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		items := make([]map[string]interface{}, 0, len(funds))
		for i := range funds {
			summary, err := h.svc.SummariseFund(&funds[i])
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
				return
			}
			items = append(items, summariseToJSON(summary))
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"funds": items, "count": len(items), "role": "supervisor"})
		return
	}

	// Client view: own positions.
	views, err := h.svc.ListClientFundPositions(claims.ClientID, "client")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	items := make([]map[string]interface{}, 0, len(views))
	for _, v := range views {
		items = append(items, map[string]interface{}{
			"fundId":             v.FundID,
			"naziv":              v.FundNaziv,
			"ukupanUlozeniRSD":   v.UkupanUlozeniRSD,
			"udeoProcenat":       v.UdeoProcenat,
			"trenutnaVrednost":   v.TrenutnaVrednost,
			"profitRSD":          v.ProfitRSD,
			"fundValueRSD":       v.FundValueRSD,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"positions": items, "count": len(items), "role": "client"})
}

func (h *FundHTTPHandler) validateFundOrder(w http.ResponseWriter, r *http.Request, fundID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireSupervisorHTTP(w, claims) {
		return
	}
	fund, err := h.svc.GetFund(fundID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "fond nije pronadjen"})
		return
	}
	if _, err := h.svc.ValidateFundBuyOrder(fundID, claims.EmployeeID, util.HasPermission(claims, models.PermEmployeeAdmin), fund.AccountID); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"fundId":    fund.ID,
		"accountId": fund.AccountID,
		"ok":        true,
	})
}

// fundParticipantIdentity maps a JWT into (clientID, clientType) for the
// fund domain. Clients always invest as themselves; supervisors can pass
// `asBank=true` to invest on behalf of the bank (clientID=0, type="bank").
func (h *FundHTTPHandler) getStatistics(w http.ResponseWriter, r *http.Request, fundID uint) {
	if _, ok := requireAuthenticatedHTTP(w, r, h.cfg); !ok {
		return
	}
	if _, err := h.svc.GetFund(fundID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "fond nije pronadjen"})
		return
	}
	stats, err := h.svc.ComputeStatistics(fundID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"statistics": statisticsToJSON(stats)})
}

func (h *FundHTTPHandler) getBenchmark(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuthenticatedHTTP(w, r, h.cfg); !ok {
		return
	}
	points, err := h.svc.AverageFundBenchmark()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"benchmark": points, "count": len(points)})
}

func (h *FundHTTPHandler) listFundDividends(w http.ResponseWriter, r *http.Request, fundID uint) {
	if _, ok := requireAuthenticatedHTTP(w, r, h.cfg); !ok {
		return
	}
	dividends, err := h.svc.ListFundDividends(fundID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	items := make([]map[string]interface{}, 0, len(dividends))
	for _, d := range dividends {
		items = append(items, map[string]interface{}{
			"id":               d.ID,
			"assetId":          d.AssetID,
			"ticker":           d.Ticker,
			"period":           d.Period,
			"quantity":         d.Quantity,
			"grossRSD":         d.GrossRSD,
			"policy":           d.Policy,
			"reinvestedShares": d.ReinvestedShares,
			"reinvestedRSD":    d.ReinvestedRSD,
			"distributedRSD":   d.DistributedRSD,
			"paidAt":           d.PaidAt.UTC().Format(time.RFC3339),
		})
	}

	// Per-participant payout breakdown (payout-policy distributions). Best-effort:
	// a read failure leaves the payouts list empty rather than failing the call.
	payoutItems := make([]map[string]interface{}, 0)
	if payouts, perr := h.svc.ListFundDividendPayouts(fundID); perr == nil {
		for _, p := range payouts {
			payoutItems = append(payoutItems, map[string]interface{}{
				"id":         p.ID,
				"assetId":    p.AssetID,
				"ticker":     p.Ticker,
				"period":     p.Period,
				"clientId":   p.ClientID,
				"clientType": p.ClientType,
				"accountId":  p.AccountID,
				"amountRSD":  p.AmountRSD,
				"paidAt":     p.PaidAt.UTC().Format(time.RFC3339),
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"dividends": items,
		"count":     len(items),
		"payouts":   payoutItems,
	})
}

func (h *FundHTTPHandler) setDividendPolicy(w http.ResponseWriter, r *http.Request, fundID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireSupervisorHTTP(w, claims) {
		return
	}
	var body struct {
		Policy string `json:"policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "neispravan zahtev"})
		return
	}
	isAdmin := util.HasPermission(claims, models.PermEmployeeAdmin)
	fund, err := h.svc.SetDividendPolicy(fundID, claims.EmployeeID, isAdmin, body.Policy)
	if err != nil {
		if errors.Is(err, service.ErrFundNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "fond nije pronadjen"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"fundId": fund.ID, "dividendPolicy": fund.DividendPolicy})
}

// runFundDividends triggers a fund-dividend distribution on demand (supervisor
// only). Mirrors the client/bank manual trigger; idempotent per quarter.
func (h *FundHTTPHandler) runFundDividends(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireSupervisorHTTP(w, claims) {
		return
	}
	res, err := h.svc.DistributeFundDividends(time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"period":    res.Period,
		"eligible":  res.Eligible,
		"processed": res.Processed,
		"skipped":   res.Skipped,
		"failed":    res.Failed,
	})
}

func fundParticipantIdentity(claims *util.Claims, asBank bool) (uint, string, error) {
	if asBank {
		if claims.TokenSource != "employee" {
			return 0, "", errors.New("samo zaposleni mogu investirati u ime banke")
		}
		if !util.HasPermission(claims, models.PermEmployeeSupervisor) {
			return 0, "", errors.New("samo supervisor moze investirati u ime banke")
		}
		return 0, "bank", nil
	}
	if claims.TokenSource == "client" {
		if !util.HasPermission(claims, models.PermClientTrading) {
			return 0, "", errors.New("klijent nema canTrade permisiju")
		}
		return claims.ClientID, "client", nil
	}
	return 0, "", errors.New("nepoznat ucesnik")
}

func summariseToJSON(s *service.FundSummary) map[string]interface{} {
	if s == nil || s.Fund == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"id":                 s.Fund.ID,
		"naziv":              s.Fund.Naziv,
		"opis":               s.Fund.Opis,
		"minimalniUlog":      s.Fund.MinimalniUlog,
		"managerId":          s.Fund.ManagerID,
		"accountId":          s.Fund.AccountID,
		"datumKreiranja":     s.Fund.DatumKreiranja.Format("2006-01-02"),
		"dividendPolicy":     s.Fund.DividendPolicy,
		"fundValueRSD":       s.FundValueRSD,
		"liquidCashRSD":      s.LiquidCashRSD,
		"holdingsValueRSD":   s.HoldingsValueRSD,
		"totalInvestedRSD":   s.TotalInvestedRSD,
		"profitRSD":          s.ProfitRSD,
		"participantsCount":  s.ParticipantsCount,
		"withdrawalCommRate": s.WithdrawalCommRate,
	}
}

func statisticsToJSON(st service.FundStatistics) map[string]interface{} {
	return map[string]interface{}{
		"available":           st.Available,
		"monthsOfData":        st.MonthsOfData,
		"annualizedReturn":    st.AnnualizedReturn,
		"volatility":          st.Volatility,
		"rewardToVariability": st.RewardToVariability,
		"maxDrawdown":         st.MaxDrawdown,
	}
}
