//go:build integration

package integration_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func extractSetupToken(message string) string {
	const marker = "token: "
	idx := strings.LastIndex(message, marker)
	if idx == -1 {
		return ""
	}
	return strings.TrimSpace(message[idx+len(marker):])
}

func TestGatewaySmoke_ClientLifecycleLoanAndCard(t *testing.T) {
	adminToken := adminLogin(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientEmail := fmt.Sprintf("gateway.smoke.%s@example.com", suffix)

	createResp, createBody := postJSONWithToken(t, "/clients", adminToken, map[string]interface{}{
		"ime":           "Gateway",
		"prezime":       "Smoke",
		"datumRodjenja": 946684800,
		"pol":           "M",
		"email":         clientEmail,
		"brojTelefona":  "0611234567",
		"adresa":        "Smoke Test 1",
	})
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create client expected 200, got %d: %v", createResp.StatusCode, createBody)
	}
	clientObj, ok := createBody["client"].(map[string]interface{})
	if !ok {
		t.Fatalf("create client response missing client object: %v", createBody)
	}
	clientID := toNumericString(clientObj["id"])
	if clientID == "" {
		t.Fatalf("client id missing from response: %v", createBody)
	}
	message, _ := createBody["message"].(string)
	setupToken := extractSetupToken(message)
	if setupToken == "" {
		t.Fatalf("setup token missing from create response: %v", createBody)
	}

	activateResp, activateBody := postJSON(t, "/auth/client/activate", map[string]string{
		"token":           setupToken,
		"password":        "ClientPass12",
		"passwordConfirm": "ClientPass12",
	})
	if activateResp.StatusCode != http.StatusOK {
		t.Fatalf("client activate expected 200, got %d: %v", activateResp.StatusCode, activateBody)
	}

	clientLoginResp, clientLoginBody := postJSON(t, "/auth/client/login", map[string]string{
		"email":    clientEmail,
		"password": "ClientPass12",
	})
	if clientLoginResp.StatusCode != http.StatusOK {
		t.Fatalf("client login expected 200, got %d: %v", clientLoginResp.StatusCode, clientLoginBody)
	}
	clientToken, _ := clientLoginBody["accessToken"].(string)
	if clientToken == "" {
		t.Fatalf("client access token missing: %v", clientLoginBody)
	}

	accountResp, accountBody := postJSONWithToken(t, "/accounts/create", adminToken, map[string]interface{}{
		"clientId":      toNumber(clientID),
		"currencyId":    1,
		"tip":           "tekuci",
		"vrsta":         "licni",
		"podvrsta":      "standardni",
		"naziv":         "Smoke account",
		"pocetnoStanje": 10000,
	})
	if accountResp.StatusCode != http.StatusOK {
		t.Fatalf("create account expected 200, got %d: %v", accountResp.StatusCode, accountBody)
	}
	accountObj, ok := accountBody["account"].(map[string]interface{})
	if !ok {
		t.Fatalf("create account response missing account: %v", accountBody)
	}
	accountID := toNumericString(accountObj["id"])
	accountNumber, _ := accountObj["brojRacuna"].(string)
	if accountID == "" || accountNumber == "" {
		t.Fatalf("account response missing id/brojRacuna: %v", accountObj)
	}

	cardResp, cardBody := postJSONWithToken(t, "/cards", adminToken, map[string]interface{}{
		"accountId":    toNumber(accountID),
		"clientId":     toNumber(clientID),
		"vrstaKartice": "visa",
		"nazivKartice": "Smoke Visa",
		"clientEmail":  clientEmail,
		"clientName":   "Gateway Smoke",
	})
	if cardResp.StatusCode != http.StatusOK {
		t.Fatalf("create card expected 200, got %d: %v", cardResp.StatusCode, cardBody)
	}

	clientCardResp, clientCardBody := getWithToken(t, "/cards/client/"+clientID, clientToken)
	if clientCardResp.StatusCode != http.StatusOK {
		t.Fatalf("client cards expected 200, got %d: %v", clientCardResp.StatusCode, clientCardBody)
	}

	loanResp, loanBody := postJSONWithToken(t, "/loans/request", clientToken, map[string]interface{}{
		"vrsta":       "gotovinski",
		"broj_racuna": accountNumber,
		"iznos":       25000,
		"period":      12,
		"tip_kamate":  "fiksna",
		"client_id":   toNumber(clientID),
		"currency_id": 1,
	})
	if loanResp.StatusCode != http.StatusCreated {
		t.Fatalf("loan request expected 201, got %d: %v", loanResp.StatusCode, loanBody)
	}
	loanObj := loanBody
	loanIDValue, ok := loanObj["id"].(float64)
	if !ok {
		t.Fatalf("loan id missing: %v", loanObj)
	}
	loanID := int(loanIDValue)

	approveResp, approveBody := postJSONWithToken(t, fmt.Sprintf("/loans/%d/approve", loanID), adminToken, map[string]interface{}{
		"zaposleni_id": 1,
	})
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("loan approve expected 200, got %d: %v", approveResp.StatusCode, approveBody)
	}

	clientLoansResp, clientLoansBody := getWithToken(t, "/loans/client/"+clientID, clientToken)
	if clientLoansResp.StatusCode != http.StatusOK {
		t.Fatalf("client loans expected 200, got %d: %v", clientLoansResp.StatusCode, clientLoansBody)
	}
}

func toNumber(raw string) int {
	var value int
	fmt.Sscanf(raw, "%d", &value)
	return value
}

func toNumericString(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	case float64:
		return fmt.Sprintf("%.0f", value)
	default:
		return ""
	}
}
