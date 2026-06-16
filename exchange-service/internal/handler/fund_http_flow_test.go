package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// TestFundHTTP_FullFlow drives a client invest -> reads -> withdraw through the
// HTTP layer, covering the success paths of investInFund / withdrawFromFund /
// getPerformance / getStatistics / getBenchmark / listHoldings / listMyPositions.
func TestFundHTTP_FullFlow(t *testing.T) {
	db := newFundTestDB(t, "fh_full_flow")
	h, svc := setupFundHandler(t, db)
	fund, err := svc.CreateFund(service.CreateFundInput{Naziv: "Flow", MinimalniUlog: 100, ManagerID: 6})
	if err != nil {
		t.Fatalf("create fund: %v", err)
	}
	now := time.Now().UTC()
	// Client 100 (clientToken) RSD account, funded.
	db.Exec(`INSERT INTO accounts (id, broj_racuna, currency_id, stanje, raspolozivo_stanje, status, client_id, created_at, updated_at) VALUES (50, 'CLI100', 1, 5000, 5000, 'aktivan', 100, ?, ?)`, now, now)

	tok := clientToken(t)
	base := fmt.Sprintf("/api/v1/funds/%d", fund.ID)

	// Invest 1000.
	if rec := do(t, h.FundRoutes, http.MethodPost, base+"/invest", tok, `{"amount":1000,"sourceAccountId":50}`); rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("invest status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Reads.
	for _, p := range []string{base + "/performance?granularity=monthly", base + "/statistics", "/api/v1/funds/benchmark", base + "/holdings", "/api/v1/funds/positions/mine"} {
		if rec := do(t, h.FundRoutes, http.MethodGet, p, tok, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s status=%d body=%s", p, rec.Code, rec.Body.String())
		}
	}

	// Withdraw a cash-covered amount.
	if rec := do(t, h.FundRoutes, http.MethodPost, base+"/withdraw", tok, `{"amount":400,"destinationAccountId":50}`); rec.Code != http.StatusOK {
		t.Errorf("withdraw status=%d body=%s", rec.Code, rec.Body.String())
	}
}
