package handler_test

import (
	"context"
	"errors"
	"testing"

	paymentv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/gen/proto/payment/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- mock service ---

type mockPaymentSvc struct {
	created   *models.Payment
	verified  *models.Payment
	got       *models.Payment
	byAccount []models.Payment
	byClient  []models.Payment
	total     int64
	createErr error
	verifyErr error
	getErr    error
	listErr   error
}

func (m *mockPaymentSvc) CreatePayment(input service.CreatePaymentInput) (*models.Payment, error) {
	return m.created, m.createErr
}
func (m *mockPaymentSvc) VerifyPayment(id uint, code string) (*models.Payment, error) {
	return m.verified, m.verifyErr
}
func (m *mockPaymentSvc) GetPayment(id uint) (*models.Payment, error) {
	return m.got, m.getErr
}
func (m *mockPaymentSvc) ListPaymentsByAccount(accountID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	return m.byAccount, m.total, m.listErr
}
func (m *mockPaymentSvc) ListPaymentsByClient(clientID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	return m.byClient, m.total, m.listErr
}

// --- helpers ---

func makePayment(id uint) *models.Payment {
	return &models.Payment{
		ID:                id,
		RacunPosiljaocaID: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             500,
		Status:            "u_obradi",
		VerifikacioniKod:  "123456",
	}
}

// --- tests ---

func TestCreatePayment_Success(t *testing.T) {
	svc := &mockPaymentSvc{created: makePayment(1)}
	h := handler.NewPaymentHandlerWithService(svc)

	resp, err := h.CreatePayment(context.Background(), &paymentv1.CreatePaymentRequest{
		RacunPosiljaocaId: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             500,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Payment.Id != 1 {
		t.Errorf("expected ID=1, got %d", resp.Payment.Id)
	}
	if resp.VerificationCode != "" {
		t.Errorf("expected empty VerificationCode, got %s", resp.VerificationCode)
	}
	if resp.Payment.Status != "u_obradi" {
		t.Errorf("expected status=u_obradi, got %s", resp.Payment.Status)
	}
}

func TestCreatePayment_MissingAccountID_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockPaymentSvc{}
	h := handler.NewPaymentHandlerWithService(svc)

	_, err := h.CreatePayment(context.Background(), &paymentv1.CreatePaymentRequest{
		RacunPosiljaocaId: 0,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             100,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestCreatePayment_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockPaymentSvc{createErr: errors.New("insufficient balance")}
	h := handler.NewPaymentHandlerWithService(svc)

	_, err := h.CreatePayment(context.Background(), &paymentv1.CreatePaymentRequest{
		RacunPosiljaocaId: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             99999,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestVerifyPayment_Success(t *testing.T) {
	verified := makePayment(2)
	verified.Status = "uspesno"
	svc := &mockPaymentSvc{verified: verified}
	h := handler.NewPaymentHandlerWithService(svc)

	resp, err := h.VerifyPayment(context.Background(), &paymentv1.VerifyPaymentRequest{
		Id: 2, VerificationCode: "123456",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Payment.Status != "uspesno" {
		t.Errorf("expected status=uspesno, got %s", resp.Payment.Status)
	}
}

func TestVerifyPayment_WrongCode_ReturnsInvalidArgument(t *testing.T) {
	svc := &mockPaymentSvc{verifyErr: &service.PaymentVerificationError{
		Code:    "invalid_verification_code",
		Message: "invalid verification code",
		Status:  "u_obradi",
	}}
	h := handler.NewPaymentHandlerWithService(svc)

	_, err := h.VerifyPayment(context.Background(), &paymentv1.VerifyPaymentRequest{
		Id: 1, VerificationCode: "000000",
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestVerifyPayment_InsufficientBalance_ReturnsFailedPrecondition(t *testing.T) {
	svc := &mockPaymentSvc{verifyErr: &service.PaymentVerificationError{
		Code:    "insufficient_balance",
		Message: "insufficient balance",
		Status:  "stornirano",
	}}
	h := handler.NewPaymentHandlerWithService(svc)

	_, err := h.VerifyPayment(context.Background(), &paymentv1.VerifyPaymentRequest{
		Id: 1, VerificationCode: "123456",
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", st.Code())
	}
}

func TestGetPayment_Success(t *testing.T) {
	svc := &mockPaymentSvc{got: makePayment(5)}
	h := handler.NewPaymentHandlerWithService(svc)

	resp, err := h.GetPayment(context.Background(), &paymentv1.GetPaymentRequest{Id: 5})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Payment.Id != 5 {
		t.Errorf("expected ID=5, got %d", resp.Payment.Id)
	}
}

func TestGetPayment_NotFound_ReturnsNotFound(t *testing.T) {
	svc := &mockPaymentSvc{getErr: errors.New("not found")}
	h := handler.NewPaymentHandlerWithService(svc)

	_, err := h.GetPayment(context.Background(), &paymentv1.GetPaymentRequest{Id: 99})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %v", st.Code())
	}
}

func TestListPaymentsByAccount_Success(t *testing.T) {
	payments := []models.Payment{*makePayment(1), *makePayment(2)}
	svc := &mockPaymentSvc{byAccount: payments, total: 2}
	h := handler.NewPaymentHandlerWithService(svc)

	resp, err := h.ListPaymentsByAccount(context.Background(), &paymentv1.ListPaymentsByAccountRequest{
		AccountId: 10, Page: 1, PageSize: 20,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Payments) != 2 {
		t.Errorf("expected 2 payments, got %d", len(resp.Payments))
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
}

func TestListPaymentsByClient_Success(t *testing.T) {
	payments := []models.Payment{*makePayment(3), *makePayment(4), *makePayment(5)}
	svc := &mockPaymentSvc{byClient: payments, total: 3}
	h := handler.NewPaymentHandlerWithService(svc)

	resp, err := h.ListPaymentsByClient(context.Background(), &paymentv1.ListPaymentsByClientRequest{
		ClientId: 7, Page: 1, PageSize: 20,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Payments) != 3 {
		t.Errorf("expected 3 payments, got %d", len(resp.Payments))
	}
}

func TestListPaymentsByAccount_ServiceError_ReturnsInternal(t *testing.T) {
	svc := &mockPaymentSvc{listErr: errors.New("db error")}
	h := handler.NewPaymentHandlerWithService(svc)

	_, err := h.ListPaymentsByAccount(context.Background(), &paymentv1.ListPaymentsByAccountRequest{AccountId: 1})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", st.Code())
	}
}
