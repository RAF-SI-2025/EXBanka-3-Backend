package handler

import (
	"context"

	exchangev1 "github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/gen/proto/exchange/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/provider"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ExchangeServiceInterface allows handler tests to inject a mock service.
type ExchangeServiceInterface interface {
	GetRateList() []service.ExchangeRate
	CalculateExchange(fromCurrency, toCurrency string, amount float64) (*service.ExchangeResult, error)
}

type ExchangeHandler struct {
	exchangev1.UnimplementedExchangeServiceServer
	svc ExchangeServiceInterface
}

func NewExchangeHandler() *ExchangeHandler {
	p := provider.NewStaticRateProvider()
	svc := service.NewExchangeServiceWithProvider(p)
	return &ExchangeHandler{svc: svc}
}

func NewExchangeHandlerWithService(svc ExchangeServiceInterface) *ExchangeHandler {
	return &ExchangeHandler{svc: svc}
}

func (h *ExchangeHandler) GetRateList(ctx context.Context, req *exchangev1.GetRateListRequest) (*exchangev1.GetRateListResponse, error) {
	rates := h.svc.GetRateList()

	items := make([]*exchangev1.ExchangeRateProto, 0, len(rates))
	for _, r := range rates {
		items = append(items, &exchangev1.ExchangeRateProto{
			From: r.From,
			To:   r.To,
			Rate: r.Rate,
		})
	}

	return &exchangev1.GetRateListResponse{Rates: items}, nil
}

func (h *ExchangeHandler) CalculateExchange(ctx context.Context, req *exchangev1.CalculateExchangeRequest) (*exchangev1.CalculateExchangeResponse, error) {
	if req.FromCurrency == "" || req.ToCurrency == "" {
		return nil, status.Error(codes.InvalidArgument, "from_currency and to_currency are required")
	}

	result, err := h.svc.CalculateExchange(req.FromCurrency, req.ToCurrency, req.Amount)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &exchangev1.CalculateExchangeResponse{
		FromCurrency: result.FromCurrency,
		ToCurrency:   result.ToCurrency,
		InputAmount:  result.InputAmount,
		OutputAmount: result.OutputAmount,
		Rate:         result.Rate,
	}, nil
}
