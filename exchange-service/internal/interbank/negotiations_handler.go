package interbank

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// NegotiationsHandler hosts the cross-bank OTC negotiation REST surface
// from spec §3 — POST /negotiations and the
// /negotiations/{routingNumber}/{id} family. The handler assumes
// AuthMiddleware has already attached a *PartnerBank to the request
// context.
//
// /accept is a method on this handler too, but lives in
// negotiations_accept.go because it owns the outbound NEW_TX dispatch
// that the other verbs don't need.
type NegotiationsHandler struct {
	registry   *Registry
	repo       *repository.InterbankOtcRepository
	client     *Client
	db         *gorm.DB
	walletRepo *repository.InterbankWalletRepository
}

// NewNegotiationsHandler wires up the negotiation routes. db and
// walletRepo are used by runAcceptDispatch to credit the local
// seller's wallet after a successful COMMIT_TX — they're left
// nil-safe-checked in the dispatch path so older test setups that
// don't need a real wallet can still construct the handler.
func NewNegotiationsHandler(
	registry *Registry,
	repo *repository.InterbankOtcRepository,
	client *Client,
	db *gorm.DB,
	walletRepo *repository.InterbankWalletRepository,
) *NegotiationsHandler {
	return &NegotiationsHandler{
		registry:   registry,
		repo:       repo,
		client:     client,
		db:         db,
		walletRepo: walletRepo,
	}
}

// Collection routes POST /negotiations — the create endpoint that a
// buyer's bank calls on the seller's bank to start a negotiation.
func (h *NegotiationsHandler) Collection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.create(w, r)
}

// Item routes /negotiations/{routingNumber}/{id} and
// /negotiations/{routingNumber}/{id}/accept. It dispatches on path
// suffix + method.
func (h *NegotiationsHandler) Item(w http.ResponseWriter, r *http.Request) {
	rest, ok := stripPrefix(r.URL.Path, "/negotiations/")
	if !ok {
		writeProblemJSON(w, http.StatusNotFound, "unknown path")
		return
	}
	parts := strings.Split(strings.TrimSuffix(rest, "/"), "/")

	switch len(parts) {
	case 2:
		// /negotiations/{routingNumber}/{id}
		routing, id, ok := parsePathKey(parts[0], parts[1])
		if !ok {
			writeProblemJSON(w, http.StatusBadRequest, "expected /negotiations/{routingNumber:int}/{id}")
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.read(w, r, routing, id)
		case http.MethodPut:
			h.update(w, r, routing, id)
		case http.MethodDelete:
			h.delete(w, r, routing, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case 3:
		// /negotiations/{routingNumber}/{id}/{verb}
		if parts[2] != "accept" {
			writeProblemJSON(w, http.StatusNotFound, fmt.Sprintf("unknown action %q", parts[2]))
			return
		}
		routing, id, ok := parsePathKey(parts[0], parts[1])
		if !ok {
			writeProblemJSON(w, http.StatusBadRequest, "expected /negotiations/{routingNumber:int}/{id}/accept")
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.accept(w, r, routing, id)
	default:
		writeProblemJSON(w, http.StatusNotFound, "expected /negotiations/{routingNumber}/{id}[/accept]")
	}
}

// create handles POST /negotiations. The body is an OtcOffer
// representing the buyer's initial bid; we mint a fresh negotiation
// id, persist our local copy with LocalRole=seller, and return the
// ForeignBankId the buyer's bank should use for subsequent calls.
func (h *NegotiationsHandler) create(w http.ResponseWriter, r *http.Request) {
	partner := PartnerFromContext(r.Context())
	if partner == nil {
		writeProblemJSON(w, http.StatusUnauthorized, "no partner in context — AuthMiddleware not wired?")
		return
	}

	var offer OtcOffer
	if err := decodeJSONBody(r, &offer); err != nil {
		writeProblemJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	own := h.registry.OwnRoutingNumber()
	if offer.SellerID.RoutingNumber != own {
		writeProblemJSON(w, http.StatusBadRequest,
			fmt.Sprintf("offer.sellerId.routingNumber %d does not match this bank (%d) — POST to the seller's bank",
				offer.SellerID.RoutingNumber, own))
		return
	}
	if offer.BuyerID.RoutingNumber != partner.Code {
		writeProblemJSON(w, http.StatusForbidden,
			fmt.Sprintf("offer.buyerId.routingNumber %d does not match the X-Api-Key partner (%d)",
				offer.BuyerID.RoutingNumber, partner.Code))
		return
	}
	if offer.LastModifiedBy.RoutingNumber != partner.Code {
		writeProblemJSON(w, http.StatusForbidden,
			"offer.lastModifiedBy must belong to the calling bank on the initial offer")
		return
	}

	negID := uuid.NewString()
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: int(own),
		NegotiationID:            negID,
		LocalRole:                models.InterbankNegotiationRoleSeller,
		CounterpartyRoutingNumber: int(partner.Code),

		BuyerRoutingNumber:  int(offer.BuyerID.RoutingNumber),
		BuyerID:             offer.BuyerID.ID,
		SellerRoutingNumber: int(offer.SellerID.RoutingNumber),
		SellerID:            offer.SellerID.ID,

		StockTicker: offer.Stock.Ticker,
		Amount:      offer.Amount,

		PricePerUnitCurrency: string(offer.PricePerUnit.Currency),
		PricePerUnitAmount:   offer.PricePerUnit.Amount,
		PremiumCurrency:      string(offer.Premium.Currency),
		PremiumAmount:        offer.Premium.Amount,

		SettlementDate: offer.SettlementDate,

		LastModifiedByRoutingNumber: int(offer.LastModifiedBy.RoutingNumber),
		LastModifiedByID:            offer.LastModifiedBy.ID,
		IsOngoing:                   true,
	}

	if err := h.repo.Create(neg); err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, fmt.Sprintf("persisting negotiation: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, ForeignBankId{
		RoutingNumber: own,
		ID:            negID,
	})
}

// read handles GET /negotiations/{routingNumber}/{id}.
func (h *NegotiationsHandler) read(w http.ResponseWriter, r *http.Request, routing RoutingNumber, id string) {
	partner := PartnerFromContext(r.Context())
	if partner == nil {
		writeProblemJSON(w, http.StatusUnauthorized, "no partner in context")
		return
	}

	neg, err := h.repo.Get(int(routing), id)
	if err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, fmt.Sprintf("loading negotiation: %v", err))
		return
	}
	if neg == nil {
		writeProblemJSON(w, http.StatusNotFound, "no such negotiation")
		return
	}
	if !partnerMayAccess(neg, partner) {
		writeProblemJSON(w, http.StatusForbidden, "this X-Api-Key is not a party to that negotiation")
		return
	}

	writeJSON(w, http.StatusOK, negotiationToWire(neg))
}

// update handles PUT /negotiations/{routingNumber}/{id} — a counter-offer.
// The body is an OtcOffer carrying the new terms; we trust the
// LastModifiedBy field within it to identify who's countering, but we
// re-check that it belongs to the calling partner.
func (h *NegotiationsHandler) update(w http.ResponseWriter, r *http.Request, routing RoutingNumber, id string) {
	partner := PartnerFromContext(r.Context())
	if partner == nil {
		writeProblemJSON(w, http.StatusUnauthorized, "no partner in context")
		return
	}

	neg, err := h.repo.Get(int(routing), id)
	if err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, fmt.Sprintf("loading negotiation: %v", err))
		return
	}
	if neg == nil {
		writeProblemJSON(w, http.StatusNotFound, "no such negotiation")
		return
	}
	if !partnerMayAccess(neg, partner) {
		writeProblemJSON(w, http.StatusForbidden, "this X-Api-Key is not a party to that negotiation")
		return
	}
	if !neg.IsOngoing {
		writeProblemJSON(w, http.StatusConflict, "negotiation is no longer ongoing")
		return
	}

	var offer OtcOffer
	if err := decodeJSONBody(r, &offer); err != nil {
		writeProblemJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if offer.LastModifiedBy.RoutingNumber != partner.Code {
		writeProblemJSON(w, http.StatusForbidden,
			"offer.lastModifiedBy.routingNumber must match the calling bank")
		return
	}

	if err := h.repo.UpdateTerms(
		int(routing), id,
		offer.Amount,
		string(offer.PricePerUnit.Currency), offer.PricePerUnit.Amount,
		string(offer.Premium.Currency), offer.Premium.Amount,
		offer.SettlementDate,
		int(offer.LastModifiedBy.RoutingNumber), offer.LastModifiedBy.ID,
	); err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, fmt.Sprintf("updating negotiation: %v", err))
		return
	}

	neg, err = h.repo.Get(int(routing), id)
	if err != nil || neg == nil {
		writeProblemJSON(w, http.StatusInternalServerError, "negotiation disappeared after update")
		return
	}
	writeJSON(w, http.StatusOK, negotiationToWire(neg))
}

// delete handles DELETE /negotiations/{routingNumber}/{id}. Closes
// the negotiation; idempotent on already-closed rows.
func (h *NegotiationsHandler) delete(w http.ResponseWriter, r *http.Request, routing RoutingNumber, id string) {
	partner := PartnerFromContext(r.Context())
	if partner == nil {
		writeProblemJSON(w, http.StatusUnauthorized, "no partner in context")
		return
	}

	neg, err := h.repo.Get(int(routing), id)
	if err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, fmt.Sprintf("loading negotiation: %v", err))
		return
	}
	if neg == nil {
		writeProblemJSON(w, http.StatusNotFound, "no such negotiation")
		return
	}
	if !partnerMayAccess(neg, partner) {
		writeProblemJSON(w, http.StatusForbidden, "this X-Api-Key is not a party to that negotiation")
		return
	}

	if neg.IsOngoing {
		if err := h.repo.MarkClosed(int(routing), id); err != nil {
			writeProblemJSON(w, http.StatusInternalServerError, fmt.Sprintf("closing negotiation: %v", err))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// negotiationToWire converts a model row to the protocol's
// OtcNegotiation shape (= OtcOffer + isOngoing).
func negotiationToWire(neg *models.InterbankOtcNegotiation) OtcNegotiation {
	return OtcNegotiation{
		OtcOffer: OtcOffer{
			Stock:          StockDescription{Ticker: neg.StockTicker},
			SettlementDate: neg.SettlementDate,
			PricePerUnit: MonetaryValue{
				Currency: CurrencyCode(neg.PricePerUnitCurrency),
				Amount:   neg.PricePerUnitAmount,
			},
			Premium: MonetaryValue{
				Currency: CurrencyCode(neg.PremiumCurrency),
				Amount:   neg.PremiumAmount,
			},
			BuyerID:  ForeignBankId{RoutingNumber: RoutingNumber(neg.BuyerRoutingNumber), ID: neg.BuyerID},
			SellerID: ForeignBankId{RoutingNumber: RoutingNumber(neg.SellerRoutingNumber), ID: neg.SellerID},
			Amount:   neg.Amount,
			LastModifiedBy: ForeignBankId{
				RoutingNumber: RoutingNumber(neg.LastModifiedByRoutingNumber),
				ID:            neg.LastModifiedByID,
			},
		},
		IsOngoing: neg.IsOngoing,
	}
}

// partnerMayAccess returns true when the calling X-Api-Key
// corresponds to either of the negotiation's counterparties. Cross-
// negotiation tampering is blocked here.
func partnerMayAccess(neg *models.InterbankOtcNegotiation, partner *PartnerBank) bool {
	code := int(partner.Code)
	return neg.BuyerRoutingNumber == code || neg.SellerRoutingNumber == code
}

func decodeJSONBody(r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading body: %w", err)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("malformed JSON body: %w", err)
	}
	return nil
}

func stripPrefix(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	return strings.TrimPrefix(path, prefix), true
}

func parsePathKey(routingStr, id string) (RoutingNumber, string, bool) {
	if routingStr == "" || id == "" {
		return 0, "", false
	}
	n, err := strconv.Atoi(routingStr)
	if err != nil {
		return 0, "", false
	}
	return RoutingNumber(n), id, true
}

