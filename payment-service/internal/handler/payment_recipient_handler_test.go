package handler_test

import (
	"context"
	"errors"
	"testing"

	prv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/gen/proto/payment_recipient/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- mock service ---

type mockRecipientSvc struct {
	created    *models.PaymentRecipient
	listed     []models.PaymentRecipient
	updated    *models.PaymentRecipient
	createErr  error
	listErr    error
	updateErr  error
	deleteErr  error
}

func (m *mockRecipientSvc) CreateRecipient(input service.CreateRecipientInput) (*models.PaymentRecipient, error) {
	return m.created, m.createErr
}

func (m *mockRecipientSvc) ListRecipientsByClient(clientID uint) ([]models.PaymentRecipient, error) {
	return m.listed, m.listErr
}

func (m *mockRecipientSvc) UpdateRecipient(id, clientID uint, input service.UpdateRecipientInput) (*models.PaymentRecipient, error) {
	return m.updated, m.updateErr
}

func (m *mockRecipientSvc) DeleteRecipient(id, clientID uint) error {
	return m.deleteErr
}

// --- helpers ---

func makeRecipient(id, clientID uint) *models.PaymentRecipient {
	return &models.PaymentRecipient{
		ID:         id,
		ClientID:   clientID,
		Naziv:      "Test Recipient",
		BrojRacuna: "000000000000000098",
	}
}

// --- tests ---

func TestCreateRecipient_Success(t *testing.T) {
	svc := &mockRecipientSvc{created: makeRecipient(1, 10)}
	h := handler.NewPaymentRecipientHandlerWithService(svc)

	resp, err := h.CreateRecipient(context.Background(), &prv1.CreateRecipientRequest{
		ClientId:   10,
		Naziv:      "Test Recipient",
		BrojRacuna: "000000000000000098",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Recipient.Id != 1 {
		t.Errorf("expected ID=1, got %d", resp.Recipient.Id)
	}
	if resp.Recipient.ClientId != 10 {
		t.Errorf("expected ClientId=10, got %d", resp.Recipient.ClientId)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCreateRecipient_MissingClientID_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockRecipientSvc{created: makeRecipient(1, 0)}
	h := handler.NewPaymentRecipientHandlerWithService(svc)

	_, err := h.CreateRecipient(context.Background(), &prv1.CreateRecipientRequest{
		ClientId:   0,
		Naziv:      "Test",
		BrojRacuna: "000000000000000098",
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestCreateRecipient_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockRecipientSvc{createErr: errors.New("invalid account number")}
	h := handler.NewPaymentRecipientHandlerWithService(svc)

	_, err := h.CreateRecipient(context.Background(), &prv1.CreateRecipientRequest{
		ClientId:   1,
		Naziv:      "Test",
		BrojRacuna: "bad",
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListRecipients_Success(t *testing.T) {
	recipients := []models.PaymentRecipient{*makeRecipient(1, 5), *makeRecipient(2, 5)}
	svc := &mockRecipientSvc{listed: recipients}
	h := handler.NewPaymentRecipientHandlerWithService(svc)

	resp, err := h.ListRecipients(context.Background(), &prv1.ListRecipientsRequest{ClientId: 5})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Recipients) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(resp.Recipients))
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
}

func TestListRecipients_ServiceError_ReturnsInternal(t *testing.T) {
	svc := &mockRecipientSvc{listErr: errors.New("db error")}
	h := handler.NewPaymentRecipientHandlerWithService(svc)

	_, err := h.ListRecipients(context.Background(), &prv1.ListRecipientsRequest{ClientId: 1})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestUpdateRecipient_Success(t *testing.T) {
	updated := makeRecipient(3, 7)
	updated.Naziv = "Updated Name"
	svc := &mockRecipientSvc{updated: updated}
	h := handler.NewPaymentRecipientHandlerWithService(svc)

	resp, err := h.UpdateRecipient(context.Background(), &prv1.UpdateRecipientRequest{
		Id:       3,
		ClientId: 7,
		Naziv:    "Updated Name",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Recipient.Naziv != "Updated Name" {
		t.Errorf("expected Naziv=Updated Name, got %s", resp.Recipient.Naziv)
	}
}

func TestUpdateRecipient_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockRecipientSvc{updateErr: errors.New("access denied")}
	h := handler.NewPaymentRecipientHandlerWithService(svc)

	_, err := h.UpdateRecipient(context.Background(), &prv1.UpdateRecipientRequest{
		Id: 1, ClientId: 99,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestDeleteRecipient_Success(t *testing.T) {
	svc := &mockRecipientSvc{}
	h := handler.NewPaymentRecipientHandlerWithService(svc)

	resp, err := h.DeleteRecipient(context.Background(), &prv1.DeleteRecipientRequest{
		Id: 1, ClientId: 5,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestDeleteRecipient_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockRecipientSvc{deleteErr: errors.New("access denied")}
	h := handler.NewPaymentRecipientHandlerWithService(svc)

	_, err := h.DeleteRecipient(context.Background(), &prv1.DeleteRecipientRequest{
		Id: 1, ClientId: 99,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}
