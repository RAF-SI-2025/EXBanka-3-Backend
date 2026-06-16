package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestOtcService_CreateOffer_ValidationErrors(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_validation")
	base := func() service.CreateOtcOfferInput {
		return service.CreateOtcOfferInput{
			BuyerID: 100, BuyerType: "client", BuyerAccountID: 2,
			SellerHoldingID: holding.ID, Amount: 3, PricePerStock: 105,
			SettlementDate: time.Now().UTC().AddDate(0, 1, 0), Premium: 25,
		}
	}
	cases := map[string]func(*service.CreateOtcOfferInput){
		"amount<=0":         func(in *service.CreateOtcOfferInput) { in.Amount = 0 },
		"price<=0":          func(in *service.CreateOtcOfferInput) { in.PricePerStock = 0 },
		"premium<0":         func(in *service.CreateOtcOfferInput) { in.Premium = -1 },
		"settlement zero":   func(in *service.CreateOtcOfferInput) { in.SettlementDate = time.Time{} },
		"settlement past":   func(in *service.CreateOtcOfferInput) { in.SettlementDate = time.Now().UTC().Add(-time.Hour) },
		"no buyer identity": func(in *service.CreateOtcOfferInput) { in.BuyerID = 0 },
		"no buyer account":  func(in *service.CreateOtcOfferInput) { in.BuyerAccountID = 0 },
		"no seller holding": func(in *service.CreateOtcOfferInput) { in.SellerHoldingID = 0 },
	}
	for name, mut := range cases {
		in := base()
		mut(&in)
		if _, err := svc.CreateOffer(in); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestOtcService_GetContractForParticipant(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_getcontract")
	offerID := makeOtcOffer(t, svc, holding.ID)
	contract, err := svc.AcceptOffer(offerID, 200, "client")
	if err != nil {
		t.Fatalf("AcceptOffer: %v", err)
	}

	// Buyer (participant) can read it.
	if _, err := svc.GetContractForParticipant(contract.ID, 100, "client"); err != nil {
		t.Errorf("participant should read contract: %v", err)
	}
	// Stranger cannot.
	if _, err := svc.GetContractForParticipant(contract.ID, 999, "client"); !errors.Is(err, service.ErrOtcContractNotFound) {
		t.Errorf("stranger: want ErrOtcContractNotFound, got %v", err)
	}
	// Unknown contract.
	if _, err := svc.GetContractForParticipant(99999, 100, "client"); !errors.Is(err, service.ErrOtcContractNotFound) {
		t.Errorf("unknown: want ErrOtcContractNotFound, got %v", err)
	}
}
