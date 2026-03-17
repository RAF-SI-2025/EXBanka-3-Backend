package handler

import (
	"context"
	"fmt"
	"time"

	transferv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/gen/proto/transfer/v1"
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
	ListTransfersByAccount(accountID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
	ListTransfersByClient(clientID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
}

type TransferHandler struct {
	transferv1.UnimplementedTransferServiceServer
	svc TransferServiceInterface
}

func NewTransferHandler(db *gorm.DB) *TransferHandler {
	accountRepo := repository.NewAccountRepository(db)
	transferRepo := repository.NewTransferRepository(db)
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, nil)
	return &TransferHandler{svc: svc}
}

func NewTransferHandlerWithService(svc TransferServiceInterface) *TransferHandler {
	return &TransferHandler{svc: svc}
}

func toTransferProto(t *models.Transfer) *transferv1.TransferProto {
	return &transferv1.TransferProto{
		Id:                 uint64(t.ID),
		RacunPosiljaocaId:  uint64(t.RacunPosiljaocaID),
		RacunPrimaocaId:    uint64(t.RacunPrimaocaID),
		Iznos:              t.Iznos,
		ValutaIznosa:       t.ValutaIznosa,
		KonvertovaniIznos:  t.KonvertovaniIznos,
		Kurs:               t.Kurs,
		Svrha:              t.Svrha,
		Status:             t.Status,
		VremeTransakcije:   t.VremeTransakcije.Format(time.RFC3339),
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

func (h *TransferHandler) CreateTransfer(ctx context.Context, req *transferv1.CreateTransferRequest) (*transferv1.CreateTransferResponse, error) {
	if req.RacunPosiljaocaId == 0 || req.RacunPrimaocaId == 0 {
		return nil, status.Error(codes.InvalidArgument, "sender and receiver account IDs are required")
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
		Message:  fmt.Sprintf("Transfer of %.2f %s created successfully", tr.Iznos, tr.ValutaIznosa),
	}, nil
}

func (h *TransferHandler) ListTransfersByAccount(ctx context.Context, req *transferv1.ListTransfersByAccountRequest) (*transferv1.ListTransfersResponse, error) {
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
