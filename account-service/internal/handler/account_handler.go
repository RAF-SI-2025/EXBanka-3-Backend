package handler

import (
	"context"

	accountv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/gen/proto/account/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// AccountServiceInterface allows handler tests to inject a mock service.
type AccountServiceInterface interface {
	CreateAccount(input service.CreateAccountInput) (*models.Account, error)
	GetAccount(id uint) (*models.Account, error)
	ListAccountsByClient(clientID uint) ([]models.Account, error)
	ListAllAccounts(filter models.AccountFilter) ([]models.Account, int64, error)
	UpdateAccountName(id uint, naziv string) error
	UpdateAccountLimits(id uint, clientID uint, dnevniLimit, mesecniLimit float64) error
}

type AccountHandler struct {
	accountv1.UnimplementedAccountServiceServer
	svc AccountServiceInterface
	db  *gorm.DB
}

func NewAccountHandler(db *gorm.DB, cfg *config.Config) *AccountHandler {
	accountRepo := repository.NewAccountRepository(db)
	currencyRepo := repository.NewCurrencyRepository(db)
	notifSvc := service.NewNotificationService(cfg)
	svc := service.NewAccountServiceWithRepos(accountRepo, currencyRepo, notifSvc)
	return &AccountHandler{svc: svc, db: db}
}

func NewAccountHandlerWithService(svc AccountServiceInterface) *AccountHandler {
	return &AccountHandler{svc: svc}
}

func toAccountProto(a *models.Account) *accountv1.AccountProto {
	proto := &accountv1.AccountProto{
		Id:                uint64(a.ID),
		BrojRacuna:        a.BrojRacuna,
		CurrencyId:        uint64(a.CurrencyID),
		Tip:               a.Tip,
		Vrsta:             a.Vrsta,
		Stanje:            a.Stanje,
		RaspolozivoStanje: a.RaspolozivoStanje,
		DnevniLimit:       a.DnevniLimit,
		MesecniLimit:      a.MesecniLimit,
		Naziv:             a.Naziv,
		Status:            a.Status,
	}
	if a.ClientID != nil {
		proto.ClientId = uint64(*a.ClientID)
	}
	if a.FirmaID != nil {
		proto.FirmaId = uint64(*a.FirmaID)
	}
	if a.Currency.Kod != "" {
		proto.CurrencyKod = a.Currency.Kod
	}
	return proto
}

func (h *AccountHandler) CreateAccount(ctx context.Context, req *accountv1.CreateAccountRequest) (*accountv1.CreateAccountResponse, error) {
	input := service.CreateAccountInput{
		CurrencyID: uint(req.CurrencyId),
		Tip:        req.Tip,
		Vrsta:      req.Vrsta,
		Naziv:      req.Naziv,
	}
	// Look up client email for notification
	if req.ClientId != 0 && h.db != nil {
		var client models.Client
		if err := h.db.First(&client, req.ClientId).Error; err == nil {
			input.ClientEmail = client.Email
			input.ClientName = client.Ime + " " + client.Prezime
		}
	}
	if req.ClientId != 0 {
		id := uint(req.ClientId)
		input.ClientID = &id
	}
	if req.FirmaId != 0 {
		id := uint(req.FirmaId)
		input.FirmaID = &id
	}
	// Extract employee ID from JWT claims
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.EmployeeID != 0 {
		empID := claims.EmployeeID
		input.ZaposleniID = &empID
	}

	acc, err := h.svc.CreateAccount(input)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &accountv1.CreateAccountResponse{
		Account: toAccountProto(acc),
		Message: "Account created",
	}, nil
}

func (h *AccountHandler) GetAccount(ctx context.Context, req *accountv1.GetAccountRequest) (*accountv1.GetAccountResponse, error) {
	acc, err := h.svc.GetAccount(uint(req.Id))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.ClientID != 0 {
		if acc.ClientID == nil || *acc.ClientID != claims.ClientID {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
	}

	return &accountv1.GetAccountResponse{Account: toAccountProto(acc)}, nil
}

func (h *AccountHandler) ListClientAccounts(ctx context.Context, req *accountv1.ListClientAccountsRequest) (*accountv1.ListAccountsResponse, error) {
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.ClientID != 0 && uint(req.ClientId) != claims.ClientID {
		return nil, status.Error(codes.PermissionDenied, "access denied")
	}

	accounts, err := h.svc.ListAccountsByClient(uint(req.ClientId))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list accounts")
	}

	items := make([]*accountv1.AccountProto, 0, len(accounts))
	for i := range accounts {
		items = append(items, toAccountProto(&accounts[i]))
	}

	return &accountv1.ListAccountsResponse{
		Accounts: items,
		Total:    int64(len(accounts)),
	}, nil
}

func (h *AccountHandler) ListAllAccounts(ctx context.Context, req *accountv1.ListAllAccountsRequest) (*accountv1.ListAccountsResponse, error) {
	filter := models.AccountFilter{
		ClientName: req.ClientName,
		Tip:        req.Tip,
		Vrsta:      req.Vrsta,
		Status:     req.Status,
		Page:       int(req.Page),
		PageSize:   int(req.PageSize),
	}
	if req.CurrencyId != 0 {
		id := uint(req.CurrencyId)
		filter.CurrencyID = &id
	}

	accounts, total, err := h.svc.ListAllAccounts(filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list accounts")
	}

	items := make([]*accountv1.AccountProto, 0, len(accounts))
	for i := range accounts {
		items = append(items, toAccountProto(&accounts[i]))
	}

	return &accountv1.ListAccountsResponse{
		Accounts: items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

func (h *AccountHandler) UpdateAccountName(ctx context.Context, req *accountv1.UpdateAccountNameRequest) (*accountv1.UpdateAccountNameResponse, error) {
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.ClientID != 0 {
		acc, err := h.svc.GetAccount(uint(req.Id))
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "account not found")
		}
		if acc.ClientID == nil || *acc.ClientID != claims.ClientID {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
	}

	if err := h.svc.UpdateAccountName(uint(req.Id), req.Naziv); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update account name")
	}

	return &accountv1.UpdateAccountNameResponse{Message: "Account name updated"}, nil
}

func (h *AccountHandler) UpdateAccountLimits(ctx context.Context, req *accountv1.UpdateAccountLimitsRequest) (*accountv1.UpdateAccountLimitsResponse, error) {
	var clientID uint
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.ClientID != 0 {
		clientID = claims.ClientID
	}
	if err := h.svc.UpdateAccountLimits(uint(req.Id), clientID, req.DnevniLimit, req.MesecniLimit); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &accountv1.UpdateAccountLimitsResponse{Message: "Account limits updated"}, nil
}
