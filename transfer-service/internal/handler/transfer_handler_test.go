package handler_test

import (
	"context"
	"errors"
	"testing"

	transferv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/gen/proto/transfer/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- mock service ---

type mockTransferSvc struct {
	created         *models.Transfer
	createErr       error
	byAccountResult []models.Transfer
	byAccountTotal  int64
	byClientResult  []models.Transfer
	byClientTotal   int64
	listErr         error
}

func (m *mockTransferSvc) CreateTransfer(input service.CreateTransferInput) (*models.Transfer, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.created, nil
}

func (m *mockTransferSvc) ListTransfersByAccount(accountID uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	return m.byAccountResult, m.byAccountTotal, m.listErr
}

func (m *mockTransferSvc) ListTransfersByClient(clientID uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	return m.byClientResult, m.byClientTotal, m.listErr
}

// --- helpers ---

func makeTransfer(id uint) *models.Transfer {
	return &models.Transfer{
		ID:                id,
		RacunPosiljaocaID: 1,
		RacunPrimaocaID:   2,
		Iznos:             500,
		ValutaIznosa:      "RSD",
		KonvertovaniIznos: 500,
		Kurs:              1.0,
		Status:            "uspesno",
	}
}

// --- tests ---

func TestCreateTransfer_Success(t *testing.T) {
	svc := &mockTransferSvc{created: makeTransfer(10)}
	h := handler.NewTransferHandlerWithService(svc)

	resp, err := h.CreateTransfer(context.Background(), &transferv1.CreateTransferRequest{
		RacunPosiljaocaId: 1,
		RacunPrimaocaId:   2,
		Iznos:             500,
		Svrha:             "Test",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Transfer.Id != 10 {
		t.Errorf("expected transfer ID=10, got %d", resp.Transfer.Id)
	}
	if resp.Transfer.Iznos != 500 {
		t.Errorf("expected Iznos=500, got %f", resp.Transfer.Iznos)
	}
}

func TestCreateTransfer_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockTransferSvc{createErr: errors.New("insufficient balance")}
	h := handler.NewTransferHandlerWithService(svc)

	_, err := h.CreateTransfer(context.Background(), &transferv1.CreateTransferRequest{
		RacunPosiljaocaId: 1,
		RacunPrimaocaId:   2,
		Iznos:             9999,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestCreateTransfer_MissingAccountIDs_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockTransferSvc{created: makeTransfer(1)}
	h := handler.NewTransferHandlerWithService(svc)

	_, err := h.CreateTransfer(context.Background(), &transferv1.CreateTransferRequest{
		RacunPosiljaocaId: 0,
		RacunPrimaocaId:   0,
		Iznos:             100,
	})

	if err == nil {
		t.Fatal("expected error for zero account IDs, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListTransfersByAccount_Success(t *testing.T) {
	transfers := []models.Transfer{*makeTransfer(1), *makeTransfer(2)}
	svc := &mockTransferSvc{byAccountResult: transfers, byAccountTotal: 2}
	h := handler.NewTransferHandlerWithService(svc)

	resp, err := h.ListTransfersByAccount(context.Background(), &transferv1.ListTransfersByAccountRequest{
		AccountId: 5,
		Page:      1,
		PageSize:  10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Transfers) != 2 {
		t.Errorf("expected 2 transfers, got %d", len(resp.Transfers))
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
}

func TestListTransfersByAccount_ServiceError_ReturnsInternal(t *testing.T) {
	svc := &mockTransferSvc{listErr: errors.New("db error")}
	h := handler.NewTransferHandlerWithService(svc)

	_, err := h.ListTransfersByAccount(context.Background(), &transferv1.ListTransfersByAccountRequest{AccountId: 1})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestListTransfersByClient_Success(t *testing.T) {
	transfers := []models.Transfer{*makeTransfer(10), *makeTransfer(11), *makeTransfer(12)}
	svc := &mockTransferSvc{byClientResult: transfers, byClientTotal: 3}
	h := handler.NewTransferHandlerWithService(svc)

	resp, err := h.ListTransfersByClient(context.Background(), &transferv1.ListTransfersByClientRequest{
		ClientId: 7,
		Page:     1,
		PageSize: 20,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Transfers) != 3 {
		t.Errorf("expected 3 transfers, got %d", len(resp.Transfers))
	}
	if resp.Total != 3 {
		t.Errorf("expected total=3, got %d", resp.Total)
	}
}

func TestListTransfersByClient_ServiceError_ReturnsInternal(t *testing.T) {
	svc := &mockTransferSvc{listErr: errors.New("db error")}
	h := handler.NewTransferHandlerWithService(svc)

	_, err := h.ListTransfersByClient(context.Background(), &transferv1.ListTransfersByClientRequest{ClientId: 7})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestCreateTransfer_ResponseContainsMessage(t *testing.T) {
	svc := &mockTransferSvc{created: makeTransfer(5)}
	h := handler.NewTransferHandlerWithService(svc)

	resp, err := h.CreateTransfer(context.Background(), &transferv1.CreateTransferRequest{
		RacunPosiljaocaId: 1,
		RacunPrimaocaId:   2,
		Iznos:             500,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestListTransfersByAccount_PaginationPassedThrough(t *testing.T) {
	svc := &mockTransferSvc{}
	h := handler.NewTransferHandlerWithService(svc)

	resp, err := h.ListTransfersByAccount(context.Background(), &transferv1.ListTransfersByAccountRequest{
		AccountId: 1,
		Page:      3,
		PageSize:  15,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Page != 3 {
		t.Errorf("expected Page=3, got %d", resp.Page)
	}
	if resp.PageSize != 15 {
		t.Errorf("expected PageSize=15, got %d", resp.PageSize)
	}
}
