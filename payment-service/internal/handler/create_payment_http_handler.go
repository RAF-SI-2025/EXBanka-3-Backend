package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
	"gorm.io/gorm"
)

type paymentCreateService interface {
	CreatePayment(input service.CreatePaymentInput) (*models.Payment, error)
}

type CreatePaymentHTTPHandler struct {
	svc paymentCreateService
	db  *gorm.DB
	cfg *config.Config
}

func NewCreatePaymentHTTPHandler(db *gorm.DB, cfg *config.Config) *CreatePaymentHTTPHandler {
	accountRepo := repository.NewAccountRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	recipientRepo := repository.NewPaymentRecipientRepository(db)
	notifSvc := service.NewNotificationService(cfg)
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, recipientRepo, notifSvc)
	return &CreatePaymentHTTPHandler{svc: svc, db: db, cfg: cfg}
}

func NewCreatePaymentHTTPHandlerWithService(svc paymentCreateService, db *gorm.DB, cfg *config.Config) *CreatePaymentHTTPHandler {
	return &CreatePaymentHTTPHandler{svc: svc, db: db, cfg: cfg}
}

type createPaymentHTTPRequest struct {
	RacunPosiljaocaIDSnake uint    `json:"racun_posiljaoca_id"`
	RacunPosiljaocaIDCamel uint    `json:"racunPosiljaocaId"`
	RacunPrimaocaBrojSnake string  `json:"racun_primaoca_broj"`
	RacunPrimaocaBrojCamel string  `json:"racunPrimaocaBroj"`
	Iznos                  float64 `json:"iznos"`
	SifraPlacanjaSnake     string  `json:"sifra_placanja"`
	SifraPlacanjaCamel     string  `json:"sifraPlacanja"`
	PozivNaBrojSnake       string  `json:"poziv_na_broj"`
	PozivNaBrojCamel       string  `json:"pozivNaBroj"`
	Svrha                  string  `json:"svrha"`
	RecipientIDSnake       uint    `json:"recipient_id"`
	RecipientIDCamel       uint    `json:"recipientId"`
	AddRecipientSnake      bool    `json:"add_recipient"`
	AddRecipientCamel      bool    `json:"addRecipient"`
	RecipientNazivSnake    string  `json:"recipient_naziv"`
	RecipientNazivCamel    string  `json:"recipientNaziv"`
}

type paymentHTTPJSON struct {
	ID                string  `json:"id"`
	RacunPosiljaocaID string  `json:"racunPosiljaocaId"`
	RacunPrimaocaBroj string  `json:"racunPrimaocaBroj"`
	Iznos             float64 `json:"iznos"`
	SifraPlacanja     string  `json:"sifraPlacanja"`
	PozivNaBroj       string  `json:"pozivNaBroj"`
	Svrha             string  `json:"svrha"`
	Status            string  `json:"status"`
	RecipientID       string  `json:"recipientId"`
	VremeTransakcije  string  `json:"vremeTransakcije"`
}

func toPaymentHTTPJSON(payment *models.Payment) paymentHTTPJSON {
	result := paymentHTTPJSON{
		ID:                uintToString(payment.ID),
		RacunPosiljaocaID: uintToString(payment.RacunPosiljaocaID),
		RacunPrimaocaBroj: payment.RacunPrimaocaBroj,
		Iznos:             payment.Iznos,
		SifraPlacanja:     payment.SifraPlacanja,
		PozivNaBroj:       payment.PozivNaBroj,
		Svrha:             payment.Svrha,
		Status:            payment.Status,
		RecipientID:       "0",
		VremeTransakcije:  payment.VremeTransakcije.UTC().Format(time.RFC3339),
	}
	if payment.RecipientID != nil {
		result.RecipientID = uintToString(*payment.RecipientID)
	}
	return result
}

func (h *CreatePaymentHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireClientPermissionHTTP(w, claims, models.PermClientBasic) {
		return
	}

	var req createPaymentHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	racunPosiljaocaID := req.RacunPosiljaocaIDSnake
	if racunPosiljaocaID == 0 {
		racunPosiljaocaID = req.RacunPosiljaocaIDCamel
	}
	racunPrimaocaBroj := req.RacunPrimaocaBrojSnake
	if racunPrimaocaBroj == "" {
		racunPrimaocaBroj = req.RacunPrimaocaBrojCamel
	}
	sifraPlacanja := req.SifraPlacanjaSnake
	if sifraPlacanja == "" {
		sifraPlacanja = req.SifraPlacanjaCamel
	}
	pozivNaBroj := req.PozivNaBrojSnake
	if pozivNaBroj == "" {
		pozivNaBroj = req.PozivNaBrojCamel
	}
	recipientID := req.RecipientIDSnake
	if recipientID == 0 {
		recipientID = req.RecipientIDCamel
	}
	addRecipient := req.AddRecipientSnake || req.AddRecipientCamel
	recipientNaziv := req.RecipientNazivSnake
	if recipientNaziv == "" {
		recipientNaziv = req.RecipientNazivCamel
	}

	owned, err := h.accountOwnedByClient(racunPosiljaocaID, claims.ClientID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to verify account ownership")
		return
	}
	if !owned {
		writeAuthError(w, http.StatusForbidden, "access denied")
		return
	}

	if recipientID != 0 {
		ownedRecipient, err := h.recipientOwnedByClient(recipientID, claims.ClientID)
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to verify recipient ownership")
			return
		}
		if !ownedRecipient {
			writeAuthError(w, http.StatusForbidden, "access denied")
			return
		}
	}

	input := service.CreatePaymentInput{
		RacunPosiljaocaID: racunPosiljaocaID,
		RacunPrimaocaBroj: racunPrimaocaBroj,
		Iznos:             req.Iznos,
		SifraPlacanja:     sifraPlacanja,
		PozivNaBroj:       pozivNaBroj,
		Svrha:             req.Svrha,
		AddRecipient:      addRecipient,
		RecipientNaziv:    recipientNaziv,
	}
	if recipientID != 0 {
		id := recipientID
		input.RecipientID = &id
	}

	if h.db != nil {
		var client models.Client
		if err := h.db.First(&client, claims.ClientID).Error; err == nil {
			input.ClientEmail = client.Email
			input.ClientName = client.Ime + " " + client.Prezime
		}
	}

	payment, err := h.svc.CreatePayment(input)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"payment": toPaymentHTTPJSON(payment),
		"message": "Payment created, please approve it via the mobile app",
	})
}

func (h *CreatePaymentHTTPHandler) accountOwnedByClient(accountID, clientID uint) (bool, error) {
	if h.db == nil {
		return true, nil
	}

	var account models.Account
	if err := h.db.First(&account, accountID).Error; err != nil {
		return false, err
	}

	return account.ClientID != nil && *account.ClientID == clientID, nil
}

func (h *CreatePaymentHTTPHandler) recipientOwnedByClient(recipientID, clientID uint) (bool, error) {
	if h.db == nil {
		return true, nil
	}

	var recipient models.PaymentRecipient
	if err := h.db.First(&recipient, recipientID).Error; err != nil {
		return false, err
	}

	return recipient.ClientID == clientID, nil
}

func uintToString(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
