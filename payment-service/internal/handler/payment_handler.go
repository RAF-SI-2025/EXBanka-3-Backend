package handler

import (
	"context"
	"time"

	paymentv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/gen/proto/payment/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// PaymentServiceInterface allows handler tests to inject a mock service.
type PaymentServiceInterface interface {
	CreatePayment(input service.CreatePaymentInput) (*models.Payment, error)
	VerifyPayment(paymentID uint, verificationCode string) (*models.Payment, error)
	GetPayment(id uint) (*models.Payment, error)
	ListPaymentsByAccount(accountID uint, filter models.PaymentFilter) ([]models.Payment, int64, error)
	ListPaymentsByClient(clientID uint, filter models.PaymentFilter) ([]models.Payment, int64, error)
}

type PaymentHandler struct {
	paymentv1.UnimplementedPaymentServiceServer
	svc PaymentServiceInterface
}

func NewPaymentHandler(db *gorm.DB) *PaymentHandler {
	accountRepo := repository.NewAccountRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	recipientRepo := repository.NewPaymentRecipientRepository(db)
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, recipientRepo)
	return &PaymentHandler{svc: svc}
}

func NewPaymentHandlerWithService(svc PaymentServiceInterface) *PaymentHandler {
	return &PaymentHandler{svc: svc}
}

func toPaymentProto(p *models.Payment) *paymentv1.PaymentProto {
	proto := &paymentv1.PaymentProto{
		Id:                uint64(p.ID),
		RacunPosiljaocaId: uint64(p.RacunPosiljaocaID),
		RacunPrimaocaBroj: p.RacunPrimaocaBroj,
		Iznos:             p.Iznos,
		SifraPlacanja:     p.SifraPlacanja,
		PozivNaBroj:       p.PozivNaBroj,
		Svrha:             p.Svrha,
		Status:            p.Status,
		VremeTransakcije:  p.VremeTransakcije.Format(time.RFC3339),
	}
	if p.RecipientID != nil {
		proto.RecipientId = uint64(*p.RecipientID)
	}
	return proto
}

func parsePaymentFilter(status string, dateFrom, dateTo string, minAmount, maxAmount float64, page, pageSize int32) models.PaymentFilter {
	f := models.PaymentFilter{
		Status:   status,
		Page:     int(page),
		PageSize: int(pageSize),
	}
	if minAmount != 0 {
		v := minAmount
		f.MinAmount = &v
	}
	if maxAmount != 0 {
		v := maxAmount
		f.MaxAmount = &v
	}
	if dateFrom != "" {
		if t, err := time.Parse(time.RFC3339, dateFrom); err == nil {
			f.DateFrom = &t
		}
	}
	if dateTo != "" {
		if t, err := time.Parse(time.RFC3339, dateTo); err == nil {
			f.DateTo = &t
		}
	}
	return f
}

func (h *PaymentHandler) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.CreatePaymentResponse, error) {
	if req.RacunPosiljaocaId == 0 {
		return nil, status.Error(codes.InvalidArgument, "racun_posiljaoca_id is required")
	}
	if req.RacunPrimaocaBroj == "" {
		return nil, status.Error(codes.InvalidArgument, "racun_primaoca_broj is required")
	}

	input := service.CreatePaymentInput{
		RacunPosiljaocaID: uint(req.RacunPosiljaocaId),
		RacunPrimaocaBroj: req.RacunPrimaocaBroj,
		Iznos:             req.Iznos,
		SifraPlacanja:     req.SifraPlacanja,
		PozivNaBroj:       req.PozivNaBroj,
		Svrha:             req.Svrha,
		AddRecipient:      req.AddRecipient,
		RecipientNaziv:    req.RecipientNaziv,
	}
	if req.RecipientId != 0 {
		id := uint(req.RecipientId)
		input.RecipientID = &id
	}

	p, err := h.svc.CreatePayment(input)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &paymentv1.CreatePaymentResponse{
		Payment:          toPaymentProto(p),
		VerificationCode: p.VerifikacioniKod,
		Message:          "Payment created, awaiting verification",
	}, nil
}

func (h *PaymentHandler) VerifyPayment(ctx context.Context, req *paymentv1.VerifyPaymentRequest) (*paymentv1.VerifyPaymentResponse, error) {
	p, err := h.svc.VerifyPayment(uint(req.Id), req.VerificationCode)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &paymentv1.VerifyPaymentResponse{
		Payment: toPaymentProto(p),
		Message: "Payment verified successfully",
	}, nil
}

func (h *PaymentHandler) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.GetPaymentResponse, error) {
	p, err := h.svc.GetPayment(uint(req.Id))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "payment not found")
	}

	return &paymentv1.GetPaymentResponse{Payment: toPaymentProto(p)}, nil
}

func (h *PaymentHandler) ListPaymentsByAccount(ctx context.Context, req *paymentv1.ListPaymentsByAccountRequest) (*paymentv1.ListPaymentsResponse, error) {
	filter := parsePaymentFilter(req.Status, req.DateFrom, req.DateTo, req.MinAmount, req.MaxAmount, req.Page, req.PageSize)

	payments, total, err := h.svc.ListPaymentsByAccount(uint(req.AccountId), filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list payments")
	}

	items := make([]*paymentv1.PaymentProto, 0, len(payments))
	for i := range payments {
		items = append(items, toPaymentProto(&payments[i]))
	}

	return &paymentv1.ListPaymentsResponse{
		Payments: items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

func (h *PaymentHandler) ListPaymentsByClient(ctx context.Context, req *paymentv1.ListPaymentsByClientRequest) (*paymentv1.ListPaymentsResponse, error) {
	filter := parsePaymentFilter(req.Status, req.DateFrom, req.DateTo, req.MinAmount, req.MaxAmount, req.Page, req.PageSize)

	payments, total, err := h.svc.ListPaymentsByClient(uint(req.ClientId), filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list payments")
	}

	items := make([]*paymentv1.PaymentProto, 0, len(payments))
	for i := range payments {
		items = append(items, toPaymentProto(&payments[i]))
	}

	return &paymentv1.ListPaymentsResponse{
		Payments: items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
