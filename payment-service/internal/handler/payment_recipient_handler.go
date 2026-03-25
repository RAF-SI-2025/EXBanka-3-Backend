package handler

import (
	"context"

	prv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/gen/proto/payment_recipient/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// PaymentRecipientServiceInterface allows handler tests to inject a mock service.
type PaymentRecipientServiceInterface interface {
	CreateRecipient(input service.CreateRecipientInput) (*models.PaymentRecipient, error)
	ListRecipientsByClient(clientID uint) ([]models.PaymentRecipient, error)
	UpdateRecipient(id, clientID uint, input service.UpdateRecipientInput) (*models.PaymentRecipient, error)
	DeleteRecipient(id, clientID uint) error
}

type PaymentRecipientHandler struct {
	prv1.UnimplementedPaymentRecipientServiceServer
	svc PaymentRecipientServiceInterface
}

func NewPaymentRecipientHandler(db *gorm.DB) *PaymentRecipientHandler {
	repo := repository.NewPaymentRecipientRepository(db)
	svc := service.NewPaymentRecipientServiceWithRepo(repo)
	return &PaymentRecipientHandler{svc: svc}
}

func NewPaymentRecipientHandlerWithService(svc PaymentRecipientServiceInterface) *PaymentRecipientHandler {
	return &PaymentRecipientHandler{svc: svc}
}

func toRecipientProto(r *models.PaymentRecipient) *prv1.RecipientProto {
	return &prv1.RecipientProto{
		Id:         uint64(r.ID),
		ClientId:   uint64(r.ClientID),
		Naziv:      r.Naziv,
		BrojRacuna: r.BrojRacuna,
	}
}

func (h *PaymentRecipientHandler) CreateRecipient(ctx context.Context, req *prv1.CreateRecipientRequest) (*prv1.RecipientResponse, error) {
	clientID := uint(req.ClientId)
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok {
		if claims.ClientID == 0 {
			return nil, status.Error(codes.PermissionDenied, "client access required")
		}
		if req.ClientId != 0 && uint(req.ClientId) != claims.ClientID {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
		clientID = claims.ClientID
	}
	if clientID == 0 {
		return nil, status.Error(codes.InvalidArgument, "client_id is required")
	}

	r, err := h.svc.CreateRecipient(service.CreateRecipientInput{
		ClientID:   clientID,
		Naziv:      req.Naziv,
		BrojRacuna: req.BrojRacuna,
	})
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &prv1.RecipientResponse{
		Recipient: toRecipientProto(r),
		Message:   "Recipient created",
	}, nil
}

func (h *PaymentRecipientHandler) ListRecipients(ctx context.Context, req *prv1.ListRecipientsRequest) (*prv1.ListRecipientsResponse, error) {
	clientID := uint(req.ClientId)
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok {
		if claims.ClientID == 0 {
			return nil, status.Error(codes.PermissionDenied, "client access required")
		}
		if req.ClientId != 0 && uint(req.ClientId) != claims.ClientID {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
		clientID = claims.ClientID
	}

	recipients, err := h.svc.ListRecipientsByClient(clientID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list recipients")
	}

	items := make([]*prv1.RecipientProto, 0, len(recipients))
	for i := range recipients {
		items = append(items, toRecipientProto(&recipients[i]))
	}

	return &prv1.ListRecipientsResponse{
		Recipients: items,
		Total:      int64(len(recipients)),
	}, nil
}

func (h *PaymentRecipientHandler) UpdateRecipient(ctx context.Context, req *prv1.UpdateRecipientRequest) (*prv1.RecipientResponse, error) {
	clientID := uint(req.ClientId)
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok {
		if claims.ClientID == 0 {
			return nil, status.Error(codes.PermissionDenied, "client access required")
		}
		if req.ClientId != 0 && uint(req.ClientId) != claims.ClientID {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
		clientID = claims.ClientID
	}

	r, err := h.svc.UpdateRecipient(uint(req.Id), clientID, service.UpdateRecipientInput{
		Naziv:      req.Naziv,
		BrojRacuna: req.BrojRacuna,
	})
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &prv1.RecipientResponse{
		Recipient: toRecipientProto(r),
		Message:   "Recipient updated",
	}, nil
}

func (h *PaymentRecipientHandler) DeleteRecipient(ctx context.Context, req *prv1.DeleteRecipientRequest) (*prv1.DeleteRecipientResponse, error) {
	clientID := uint(req.ClientId)
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok {
		if claims.ClientID == 0 {
			return nil, status.Error(codes.PermissionDenied, "client access required")
		}
		if req.ClientId != 0 && uint(req.ClientId) != claims.ClientID {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
		clientID = claims.ClientID
	}

	if err := h.svc.DeleteRecipient(uint(req.Id), clientID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &prv1.DeleteRecipientResponse{Message: "Recipient deleted"}, nil
}
