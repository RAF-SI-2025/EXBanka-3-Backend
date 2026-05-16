package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

var (
	ErrOtcOfferNotFound    = errors.New("otc offer not found")
	ErrOtcContractNotFound = errors.New("otc contract not found")
)

type OtcService struct {
	portfolioRepo *repository.PortfolioRepository
	otcRepo       *repository.OtcRepository
	orchestrator  *SagaOrchestrator
}

func NewOtcService(portfolioRepo *repository.PortfolioRepository, otcRepo *repository.OtcRepository) *OtcService {
	return &OtcService{portfolioRepo: portfolioRepo, otcRepo: otcRepo}
}

func (s *OtcService) WithOrchestrator(orchestrator *SagaOrchestrator) *OtcService {
	s.orchestrator = orchestrator
	return s
}

type PublicOtcStock struct {
	HoldingID         uint
	SellerID          uint
	SellerType        string
	AssetID           uint
	Ticker            string
	Name              string
	Exchange          string
	Currency          string
	Price             float64
	Ask               float64
	Bid               float64
	PublicQuantity    float64
	ReservedQuantity  float64
	AvailableQuantity float64
	LastRefresh       time.Time
}

func (s *OtcService) ListPublicStocks(requesterID uint, requesterType string) ([]PublicOtcStock, error) {
	holdings, err := s.portfolioRepo.ListPublicOTCHoldings(requesterID, requesterType)
	if err != nil {
		return nil, err
	}

	stocks := make([]PublicOtcStock, 0, len(holdings))
	for _, holding := range holdings {
		available := holding.AvailableForOTC()
		if available <= 0 || holding.Asset.Type != string(models.ListingTypeStock) {
			continue
		}
		stocks = append(stocks, PublicOtcStock{
			HoldingID:         holding.ID,
			SellerID:          holding.UserID,
			SellerType:        holding.UserType,
			AssetID:           holding.AssetID,
			Ticker:            holding.Asset.Ticker,
			Name:              holding.Asset.Name,
			Exchange:          holding.Asset.Exchange.Acronym,
			Currency:          holding.Asset.Exchange.Currency,
			Price:             holding.Asset.Price,
			Ask:               holding.Asset.Ask,
			Bid:               holding.Asset.Bid,
			PublicQuantity:    holding.EffectivePublicQuantity(),
			ReservedQuantity:  holding.ReservedQuantity,
			AvailableQuantity: available,
			LastRefresh:       holding.Asset.LastRefresh,
		})
	}

	return stocks, nil
}

type CreateOtcOfferInput struct {
	BuyerID         uint
	BuyerType       string
	BuyerAccountID  uint
	SellerHoldingID uint
	Amount          float64
	PricePerStock   float64
	SettlementDate  time.Time
	Premium         float64
}

func (s *OtcService) CreateOffer(input CreateOtcOfferInput) (*models.OtcOfferRecord, error) {
	if !hasParticipantIdentity(input.BuyerID, input.BuyerType) {
		return nil, fmt.Errorf("buyer identity is required")
	}
	if input.SellerHoldingID == 0 {
		return nil, fmt.Errorf("seller holding is required")
	}
	if input.BuyerAccountID == 0 {
		return nil, fmt.Errorf("buyer account is required")
	}
	if err := validateOtcTerms(input.Amount, input.PricePerStock, input.SettlementDate, input.Premium); err != nil {
		return nil, err
	}

	holding, err := s.portfolioRepo.GetHoldingByID(input.SellerHoldingID)
	if err != nil {
		return nil, err
	}
	if holding == nil {
		return nil, fmt.Errorf("seller holding not found")
	}
	if holding.UserID == input.BuyerID && holding.UserType == input.BuyerType {
		return nil, fmt.Errorf("buyer cannot create an OTC offer for their own holding")
	}
	if holding.Asset.Type != string(models.ListingTypeStock) {
		return nil, fmt.Errorf("only stock holdings can be offered through OTC")
	}
	if input.Amount > holding.AvailableForOTC() {
		return nil, fmt.Errorf("amount exceeds available OTC quantity")
	}
	buyerAccount, err := s.otcRepo.GetAccountReference(input.BuyerAccountID)
	if err != nil {
		return nil, err
	}
	if err := validateOtcAccountForParticipant(buyerAccount, input.BuyerID, input.BuyerType, holding.Asset.Exchange.Currency, "buyer"); err != nil {
		return nil, err
	}
	if holding.AccountID == 0 {
		return nil, fmt.Errorf("seller holding account is required")
	}
	sellerAccount, err := s.otcRepo.GetAccountReference(holding.AccountID)
	if err != nil {
		return nil, err
	}
	if err := validateOtcAccountForParticipant(sellerAccount, holding.UserID, holding.UserType, holding.Asset.Exchange.Currency, "seller"); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	offer := &models.OtcOfferRecord{
		StockListingID:  holding.AssetID,
		SellerHoldingID: holding.ID,
		Amount:          input.Amount,
		PricePerStock:   input.PricePerStock,
		SettlementDate:  input.SettlementDate.UTC(),
		Premium:         input.Premium,
		LastModified:    now,
		ModifiedByID:    input.BuyerID,
		ModifiedByType:  input.BuyerType,
		Status:          models.OtcOfferStatusPending,
		BuyerID:         input.BuyerID,
		BuyerType:       input.BuyerType,
		BuyerAccountID:  input.BuyerAccountID,
		SellerID:        holding.UserID,
		SellerType:      holding.UserType,
		SellerAccountID: holding.AccountID,
	}
	if err := s.otcRepo.CreateOffer(offer); err != nil {
		return nil, err
	}

	created, err := s.otcRepo.GetOfferByID(offer.ID)
	if err != nil {
		return nil, err
	}
	return created, nil
}

type CounterOtcOfferInput struct {
	OfferID        uint
	ModifiedByID   uint
	ModifiedByType string
	Amount         float64
	PricePerStock  float64
	SettlementDate time.Time
	Premium        float64
}

func (s *OtcService) CounterOffer(input CounterOtcOfferInput) (*models.OtcOfferRecord, error) {
	if !hasParticipantIdentity(input.ModifiedByID, input.ModifiedByType) {
		return nil, fmt.Errorf("modifier identity is required")
	}
	if input.OfferID == 0 {
		return nil, fmt.Errorf("offer id is required")
	}
	if err := validateOtcTerms(input.Amount, input.PricePerStock, input.SettlementDate, input.Premium); err != nil {
		return nil, err
	}

	offer, err := s.otcRepo.GetOfferByID(input.OfferID)
	if err != nil {
		return nil, err
	}
	if offer == nil || !isOfferParticipant(*offer, input.ModifiedByID, input.ModifiedByType) {
		return nil, ErrOtcOfferNotFound
	}
	if offer.Status != models.OtcOfferStatusPending {
		return nil, fmt.Errorf("only pending offers can be countered")
	}

	holding, err := s.portfolioRepo.GetHoldingByID(offer.SellerHoldingID)
	if err != nil {
		return nil, err
	}
	if holding == nil {
		return nil, fmt.Errorf("seller holding not found")
	}
	if holding.Asset.Type != string(models.ListingTypeStock) {
		return nil, fmt.Errorf("only stock holdings can be offered through OTC")
	}
	if input.Amount > holding.AvailableForOTC() {
		return nil, fmt.Errorf("amount exceeds available OTC quantity")
	}

	if err := s.otcRepo.UpdateOfferTerms(
		offer.ID,
		input.Amount,
		input.PricePerStock,
		input.SettlementDate.UTC(),
		input.Premium,
		input.ModifiedByID,
		input.ModifiedByType,
	); err != nil {
		return nil, err
	}

	updated, err := s.otcRepo.GetOfferByID(offer.ID)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *OtcService) DeclineOffer(offerID uint, sellerID uint, sellerType string) (*models.OtcOfferRecord, error) {
	return s.updateOfferStatusForParticipant(offerID, sellerID, sellerType, models.OtcOfferStatusDeclined)
}

func (s *OtcService) CancelOffer(offerID uint, buyerID uint, buyerType string) (*models.OtcOfferRecord, error) {
	return s.updateOfferStatusForParticipant(offerID, buyerID, buyerType, models.OtcOfferStatusCancelled)
}

func (s *OtcService) AcceptOffer(offerID uint, userID uint, userType string) (*models.OtcContractRecord, error) {
	if !hasParticipantIdentity(userID, userType) {
		return nil, fmt.Errorf("participant identity is required")
	}
	if offerID == 0 {
		return nil, fmt.Errorf("offer id is required")
	}

	offer, err := s.otcRepo.GetOfferByID(offerID)
	if err != nil {
		return nil, err
	}
	if offer == nil || !isOfferParticipant(*offer, userID, userType) {
		return nil, ErrOtcOfferNotFound
	}
	if offer.Status != models.OtcOfferStatusPending {
		return nil, fmt.Errorf("only pending offers can be accepted")
	}
	if offer.ModifiedByID == userID && offer.ModifiedByType == userType {
		return nil, fmt.Errorf("cannot accept your own counter offer")
	}

	contract, err := s.otcRepo.AcceptOfferAndCreateContract(offerID, userID, userType)
	if err != nil {
		if err.Error() == "offer not found" {
			return nil, ErrOtcOfferNotFound
		}
		return nil, err
	}
	return contract, nil
}

func (s *OtcService) ListOffersForParticipant(userID uint, userType string, status string) ([]models.OtcOfferRecord, error) {
	if err := validateOfferStatusFilter(status); err != nil {
		return nil, err
	}
	return s.otcRepo.ListOffersForParticipant(userID, userType, status)
}

func (s *OtcService) ListContractsForParticipant(userID uint, userType string, status string) ([]models.OtcContractRecord, error) {
	if err := validateContractStatusFilter(status); err != nil {
		return nil, err
	}
	return s.otcRepo.ListContractsForParticipant(userID, userType, status)
}

// GetContractForParticipant returns the contract iff the caller is buyer or
// seller. Mirrors GetOfferForParticipant — used by GET /api/v1/otc/contracts/{id}.
func (s *OtcService) GetContractForParticipant(contractID, userID uint, userType string) (*models.OtcContractRecord, error) {
	contract, err := s.otcRepo.GetContractByID(contractID)
	if err != nil {
		return nil, err
	}
	if contract == nil {
		return nil, ErrOtcContractNotFound
	}
	isBuyer := contract.BuyerID == userID && contract.BuyerType == userType
	isSeller := contract.SellerID == userID && contract.SellerType == userType
	if !isBuyer && !isSeller {
		return nil, ErrOtcContractNotFound
	}
	return contract, nil
}

func (s *OtcService) ExpireDueContracts(referenceTime time.Time) (int, error) {
	if referenceTime.IsZero() {
		referenceTime = time.Now().UTC()
	}
	return s.otcRepo.ExpireValidContracts(referenceTime.UTC())
}

type ExerciseContractResult struct {
	SagaID   uint
	Contract *models.OtcContractRecord
}

// ExerciseContract validates that the caller is the buyer on a still-valid,
// not-yet-expired OTC contract and runs the 5-step exercise saga. Returns the
// saga ID so the caller can poll progress.
func (s *OtcService) ExerciseContract(contractID, buyerID uint, buyerType string) (*ExerciseContractResult, error) {
	if s.orchestrator == nil {
		return nil, fmt.Errorf("saga orchestrator is not configured")
	}
	if !hasParticipantIdentity(buyerID, buyerType) {
		return nil, fmt.Errorf("buyer identity is required")
	}
	if contractID == 0 {
		return nil, fmt.Errorf("contract id is required")
	}

	contract, err := s.otcRepo.GetContractByID(contractID)
	if err != nil {
		return nil, err
	}
	if contract == nil {
		return nil, ErrOtcContractNotFound
	}
	if contract.BuyerID != buyerID || contract.BuyerType != buyerType {
		return nil, ErrOtcContractNotFound
	}
	if contract.Status != models.OtcContractStatusValid {
		return nil, fmt.Errorf("contract is not in a valid state")
	}
	now := time.Now().UTC()
	settlementEnd := contract.SettlementDate.Add(24 * time.Hour)
	if !settlementEnd.After(now) {
		return nil, fmt.Errorf("contract settlement window has passed")
	}

	payload, err := MarshalOtcExercisePayload(contract)
	if err != nil {
		return nil, err
	}

	steps := BuildOtcExerciseSteps(contract)
	sagaID, runErr := s.orchestrator.Run(models.SagaTypeOtcExercise, payload, steps)
	if runErr != nil {
		return &ExerciseContractResult{SagaID: sagaID, Contract: contract}, runErr
	}

	finalContract, err := s.otcRepo.GetContractByID(contractID)
	if err != nil {
		return &ExerciseContractResult{SagaID: sagaID, Contract: contract}, err
	}
	return &ExerciseContractResult{SagaID: sagaID, Contract: finalContract}, nil
}

func (s *OtcService) GetOfferForParticipant(offerID uint, userID uint, userType string) (*models.OtcOfferRecord, error) {
	offer, err := s.otcRepo.GetOfferByID(offerID)
	if err != nil {
		return nil, err
	}
	if offer == nil {
		return nil, ErrOtcOfferNotFound
	}
	if !isOfferParticipant(*offer, userID, userType) {
		return nil, ErrOtcOfferNotFound
	}
	return offer, nil
}

func isOfferParticipant(offer models.OtcOfferRecord, userID uint, userType string) bool {
	return (offer.BuyerID == userID && offer.BuyerType == userType) ||
		(offer.SellerID == userID && offer.SellerType == userType)
}

func validateOfferStatusFilter(status string) error {
	switch status {
	case "", models.OtcOfferStatusPending, models.OtcOfferStatusAccepted, models.OtcOfferStatusDeclined, models.OtcOfferStatusCancelled:
		return nil
	default:
		return fmt.Errorf("invalid offer status")
	}
}

func validateContractStatusFilter(status string) error {
	switch status {
	case "", models.OtcContractStatusValid, models.OtcContractStatusExercised, models.OtcContractStatusExpired:
		return nil
	default:
		return fmt.Errorf("invalid contract status")
	}
}

func (s *OtcService) updateOfferStatusForParticipant(offerID uint, userID uint, userType string, status string) (*models.OtcOfferRecord, error) {
	if !hasParticipantIdentity(userID, userType) {
		return nil, fmt.Errorf("participant identity is required")
	}
	if offerID == 0 {
		return nil, fmt.Errorf("offer id is required")
	}

	offer, err := s.otcRepo.GetOfferByID(offerID)
	if err != nil {
		return nil, err
	}
	if offer == nil || !isOfferParticipant(*offer, userID, userType) {
		return nil, ErrOtcOfferNotFound
	}
	if offer.Status != models.OtcOfferStatusPending {
		return nil, fmt.Errorf("only pending offers can be updated")
	}

	switch status {
	case models.OtcOfferStatusDeclined:
		if offer.SellerID != userID || offer.SellerType != userType {
			return nil, fmt.Errorf("only seller can decline an offer")
		}
	case models.OtcOfferStatusCancelled:
		if offer.BuyerID != userID || offer.BuyerType != userType {
			return nil, fmt.Errorf("only buyer can cancel an offer")
		}
	default:
		return nil, fmt.Errorf("unsupported offer status update")
	}

	if err := s.otcRepo.UpdateOfferStatus(offer.ID, status, userID, userType); err != nil {
		return nil, err
	}
	updated, err := s.otcRepo.GetOfferByID(offer.ID)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func validateOtcTerms(amount, pricePerStock float64, settlementDate time.Time, premium float64) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if pricePerStock <= 0 {
		return fmt.Errorf("price per stock must be positive")
	}
	if premium < 0 {
		return fmt.Errorf("premium cannot be negative")
	}
	if settlementDate.IsZero() {
		return fmt.Errorf("settlement date is required")
	}
	if !settlementDate.After(time.Now().UTC()) {
		return fmt.Errorf("settlement date must be in the future")
	}
	return nil
}

func validateOtcAccountForParticipant(account *repository.OtcAccountReference, userID uint, userType string, expectedCurrency string, role string) error {
	if account == nil {
		return fmt.Errorf("%s account not found", role)
	}
	if account.Status != "aktivan" {
		return fmt.Errorf("%s account is not active", role)
	}
	if account.CurrencyCode != expectedCurrency {
		return fmt.Errorf("%s account currency must match stock currency", role)
	}
	if !account.IsOwnedBy(userID, userType) {
		return fmt.Errorf("%s account does not belong to participant", role)
	}
	return nil
}

func hasParticipantIdentity(userID uint, userType string) bool {
	if userType == "" {
		return false
	}
	return userType != string(models.PortfolioOwnerTypeClient) || userID != 0
}
