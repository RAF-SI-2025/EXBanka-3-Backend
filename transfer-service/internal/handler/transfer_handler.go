package handler

import (
	"context"
	"fmt"
	"time"

	transferv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/gen/proto/transfer/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// TransferServiceInterface allows handler tests to inject a mock service.
type TransferServiceInterface interface {
	CreateTransfer(input service.CreateTransferInput) (*models.Transfer, error)
	VerifyTransfer(transferID uint, verificationCode string) (*models.Transfer, error)
	ListTransfersByAccount(accountID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
	ListTransfersByClient(clientID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
}

type TransferHandler struct {
	transferv1.UnimplementedTransferServiceServer
	svc TransferServiceInterface
	db  *gorm.DB
}

func NewTransferHandler(db *gorm.DB, exchangeServiceURL string, cfg *config.Config) *TransferHandler {
	accountRepo := repository.NewAccountRepository(db)
	transferRepo := repository.NewTransferRepository(db)
	exchangeSvc := service.NewHTTPExchangeRateService(exchangeServiceURL)
	notifier := service.NewNotificationService(cfg)
	svc := service.NewTransferServiceWithReposAndNotifier(accountRepo, transferRepo, exchangeSvc, notifier)
	return &TransferHandler{svc: svc, db: db}
}

func NewTransferHandlerWithService(svc TransferServiceInterface) *TransferHandler {
	return &TransferHandler{svc: svc}
}

func toTransferProto(t *models.Transfer) *transferv1.TransferProto {
	return &transferv1.TransferProto{
		Id:                uint64(t.ID),
		RacunPosiljaocaId: uint64(t.RacunPosiljaocaID),
		RacunPrimaocaId:   uint64(t.RacunPrimaocaID),
		Iznos:             t.Iznos,
		ValutaIznosa:      t.ValutaIznosa,
		KonvertovaniIznos: t.KonvertovaniIznos,
		Kurs:              t.Kurs,
		Svrha:             t.Svrha,
		Status:            t.Status,
		VremeTransakcije:  t.VremeTransakcije.Format(time.RFC3339),
	}
}

func parseFilterFromAccount(req *transferv1.ListTransfersByAccountRequest) models.TransferFilter {
	f := models.TransferFilter{
		Status:   req.Status,
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
	}
	if req.MinAmount != 0 {
		v := req.MinAmount
		f.MinAmount = &v
	}
	if req.MaxAmount != 0 {
		v := req.MaxAmount
		f.MaxAmount = &v
	}
	if req.DateFrom != "" {
		if t, err := time.Parse(time.RFC3339, req.DateFrom); err == nil {
			f.DateFrom = &t
		}
	}
	if req.DateTo != "" {
		if t, err := time.Parse(time.RFC3339, req.DateTo); err == nil {
			f.DateTo = &t
		}
	}
	return f
}

func parseFilterFromClient(req *transferv1.ListTransfersByClientRequest) models.TransferFilter {
	f := models.TransferFilter{
		Status:   req.Status,
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
	}
	if req.MinAmount != 0 {
		v := req.MinAmount
		f.MinAmount = &v
	}
	if req.MaxAmount != 0 {
		v := req.MaxAmount
		f.MaxAmount = &v
	}
	if req.DateFrom != "" {
		if t, err := time.Parse(time.RFC3339, req.DateFrom); err == nil {
			f.DateFrom = &t
		}
	}
	if req.DateTo != "" {
		if t, err := time.Parse(time.RFC3339, req.DateTo); err == nil {
			f.DateTo = &t
		}
	}
	return f
}

// VerifyTransfer delegates to the underlying service (used by the custom HTTP verify handler).
func (h *TransferHandler) VerifyTransfer(transferID uint, verificationCode string) (*models.Transfer, error) {
	return h.svc.VerifyTransfer(transferID, verificationCode)
}

func (h *TransferHandler) CreateTransfer(ctx context.Context, req *transferv1.CreateTransferRequest) (*transferv1.CreateTransferResponse, error) {
	if req.RacunPosiljaocaId == 0 || req.RacunPrimaocaId == 0 {
		return nil, status.Error(codes.InvalidArgument, "sender and receiver account IDs are required")
	}
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.ClientID != 0 {
		senderOwned, err := h.accountOwnedByClient(uint(req.RacunPosiljaocaId), claims.ClientID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to verify account ownership")
		}
		if !senderOwned {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
		receiverOwned, err := h.accountOwnedByClient(uint(req.RacunPrimaocaId), claims.ClientID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to verify account ownership")
		}
		if !receiverOwned {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
	}

	input := service.CreateTransferInput{
		RacunPosiljaocaID: uint(req.RacunPosiljaocaId),
		RacunPrimaocaID:   uint(req.RacunPrimaocaId),
		Iznos:             req.Iznos,
		Svrha:             req.Svrha,
	}

	tr, err := h.svc.CreateTransfer(input)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &transferv1.CreateTransferResponse{
		Transfer: toTransferProto(tr),
		Message:  fmt.Sprintf("Transfer of %.2f %s created successfully. Verification code sent to email.", tr.Iznos, tr.ValutaIznosa),
	}, nil
}

func (h *TransferHandler) ListTransfersByAccount(ctx context.Context, req *transferv1.ListTransfersByAccountRequest) (*transferv1.ListTransfersResponse, error) {
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.ClientID != 0 {
		owned, err := h.accountOwnedByClient(uint(req.AccountId), claims.ClientID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to verify account ownership")
		}
		if !owned {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
	}

	filter := parseFilterFromAccount(req)

	transfers, total, err := h.svc.ListTransfersByAccount(uint(req.AccountId), filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list transfers")
	}

	items := make([]*transferv1.TransferProto, 0, len(transfers))
	for i := range transfers {
		items = append(items, toTransferProto(&transfers[i]))
	}

	return &transferv1.ListTransfersResponse{
		Transfers: items,
		Total:     total,
		Page:      req.Page,
		PageSize:  req.PageSize,
	}, nil
}

func (h *TransferHandler) ListTransfersByClient(ctx context.Context, req *transferv1.ListTransfersByClientRequest) (*transferv1.ListTransfersResponse, error) {
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.ClientID != 0 && uint(req.ClientId) != claims.ClientID {
		return nil, status.Error(codes.PermissionDenied, "access denied")
	}

	filter := parseFilterFromClient(req)

	transfers, total, err := h.svc.ListTransfersByClient(uint(req.ClientId), filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list transfers")
	}

	items := make([]*transferv1.TransferProto, 0, len(transfers))
	for i := range transfers {
		items = append(items, toTransferProto(&transfers[i]))
	}

	return &transferv1.ListTransfersResponse{
		Transfers: items,
		Total:     total,
		Page:      req.Page,
		PageSize:  req.PageSize,
	}, nil
}

func (h *TransferHandler) accountOwnedByClient(accountID, clientID uint) (bool, error) {
	if h.db == nil {
		return true, nil
	}

	var account models.Account
	if err := h.db.First(&account, accountID).Error; err != nil {
		return false, err
	}

	return account.ClientID != nil && *account.ClientID == clientID, nil
}
