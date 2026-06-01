package interbank

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// TestCallerMayCounter pins the §3.3 turn rule: a counter-offer may only
// be placed by the party who did NOT make the most recent offer.
func TestCallerMayCounter(t *testing.T) {
	const (
		buyerBank  = 444
		sellerBank = 333
	)
	neg := &models.InterbankOtcNegotiation{
		BuyerRoutingNumber:  buyerBank,
		BuyerID:             "client-1",
		SellerRoutingNumber: sellerBank,
		SellerID:            "client-9",
	}

	cases := []struct {
		name           string
		lastModifiedBy int
		caller         RoutingNumber
		want           bool
	}{
		{"buyer moved last, seller may counter", buyerBank, sellerBank, true},
		{"buyer moved last, buyer may NOT counter again", buyerBank, buyerBank, false},
		{"seller moved last, buyer may counter", sellerBank, buyerBank, true},
		{"seller moved last, seller may NOT counter again", sellerBank, sellerBank, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			neg.LastModifiedByRoutingNumber = tc.lastModifiedBy
			got := callerMayCounter(neg, &PartnerBank{Code: tc.caller})
			if got != tc.want {
				t.Fatalf("callerMayCounter(lastModified=%d, caller=%d) = %v, want %v",
					tc.lastModifiedBy, tc.caller, got, tc.want)
			}
		})
	}
}
