package service_test

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
)

// --- TransferVerificationError ---

func TestTransferVerificationError_ImplementsError(t *testing.T) {
	e := &service.TransferVerificationError{
		Code:    "x",
		Message: "boom",
	}
	if e.Error() != "boom" {
		t.Errorf("expected 'boom', got %q", e.Error())
	}
}

// --- WithDB ---

func TestWithDB_Chains(t *testing.T) {
	svc := service.NewTransferServiceWithRepos(
		&mockAccountRepo{accounts: map[uint]*models.Account{}},
		&mockTransferRepo{},
		&mockExchangeRateService{},
	)
	if got := svc.WithDB(nil); got != svc {
		t.Error("expected WithDB to return same service for chaining")
	}
}

// --- PreviewTransfer ---

func TestPreviewTransfer_SameCurrency(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 0),
	}}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	preview, err := svc.PreviewTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 750,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.KonvertovaniIznos != 750 || preview.Kurs != 1.0 || preview.Provizija != 0 {
		t.Errorf("unexpected preview: %+v", preview)
	}
}

func TestPreviewTransfer_CrossCurrency_ReturnsCommission(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: eurAccount(1, 1000),
		2: rsdAccount(2, 0),
	}}
	rateSvc := &mockExchangeRateService{rate: 117.0}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, rateSvc)

	preview, err := svc.PreviewTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.ProvizijaProcent != 0.5 {
		t.Errorf("expected 0.5%% commission, got %v", preview.ProvizijaProcent)
	}
	// 200 * 0.5 / 100 = 1.0
	if preview.Provizija != 1.0 {
		t.Errorf("expected commission=1.0, got %v", preview.Provizija)
	}
	if preview.KonvertovaniIznos != 23400 {
		t.Errorf("expected 200*117=23400 KonvertovaniIznos, got %v", preview.KonvertovaniIznos)
	}
}

func TestPreviewTransfer_BadInput_PropagatesError(t *testing.T) {
	svc := service.NewTransferServiceWithRepos(
		&mockAccountRepo{accounts: map[uint]*models.Account{}},
		&mockTransferRepo{},
		&mockExchangeRateService{},
	)

	if _, err := svc.PreviewTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 1, Iznos: 100,
	}); err == nil {
		t.Fatal("expected same-account error")
	}
}

// --- CreateAndSettleTransfer (non-tx path) ---

func TestCreateAndSettleTransfer_SameCurrency_UpdatesBothBalances(t *testing.T) {
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 100),
	})
	transferRepo := &mockTransferRepo{}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	tr, err := svc.CreateAndSettleTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != "uspesno" {
		t.Errorf("expected uspesno status, got %s", tr.Status)
	}
	if accountRepo.updates[1]["raspolozivo_stanje"].(float64) != 4000 {
		t.Errorf("sender balance: expected 4000, got %v", accountRepo.updates[1]["raspolozivo_stanje"])
	}
	if accountRepo.updates[2]["raspolozivo_stanje"].(float64) != 1100 {
		t.Errorf("receiver balance: expected 1100, got %v", accountRepo.updates[2]["raspolozivo_stanje"])
	}
}

func TestCreateAndSettleTransfer_CrossCurrency_UpdatesBankAccounts(t *testing.T) {
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: eurAccount(1, 1000),
		2: rsdAccount(2, 0),
	})
	transferRepo := &mockTransferRepo{}
	rateSvc := &mockExchangeRateService{rate: 117.0}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, rateSvc)

	tr, err := svc.CreateAndSettleTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != "uspesno" {
		t.Errorf("expected uspesno, got %s", tr.Status)
	}
	// Cross-currency settlement should have touched the bank-account intermediary (id=9000).
	if _, ok := accountRepo.updates[9000]; !ok {
		t.Errorf("expected bank account 9000 to be updated, got %v", keysOf(accountRepo.updates))
	}
}

func TestCreateAndSettleTransfer_PrepareError(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 100),
		2: rsdAccount(2, 0),
	}}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	if _, err := svc.CreateAndSettleTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 99999,
	}); err == nil {
		t.Fatal("expected insufficient balance error before settlement")
	}
}

func keysOf(m map[uint]map[string]interface{}) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- error-injecting account repo for hitting settleTransferNonTx error branches ---

type errAccountRepo struct {
	*captureAccountRepo
	bankErr            error
	updateBankErr      error
	receiverUpdateErr  error
	senderUpdateErr    error
	failOnAccountID    uint  // returns error on UpdateFields for this id
	failBankAfterFirst bool  // return error on second bank-account update
	bankCalls          int
}

func newErrAccountRepo(accounts map[uint]*models.Account) *errAccountRepo {
	return &errAccountRepo{captureAccountRepo: newCaptureRepo(accounts)}
}

func (r *errAccountRepo) FindBankAccountByCurrency(currencyKod string) (*models.Account, error) {
	r.bankCalls++
	if r.bankErr != nil {
		return nil, r.bankErr
	}
	if r.failBankAfterFirst && r.bankCalls > 1 {
		return nil, errInjected
	}
	return r.captureAccountRepo.FindBankAccountByCurrency(currencyKod)
}

func (r *errAccountRepo) UpdateFields(id uint, fields map[string]interface{}) error {
	if id == r.failOnAccountID {
		return errInjected
	}
	if r.updateBankErr != nil && id == 9000 {
		return r.updateBankErr
	}
	return r.captureAccountRepo.UpdateFields(id, fields)
}

var errInjected = errInjectedErr{}

type errInjectedErr struct{}

func (errInjectedErr) Error() string { return "injected error" }

// --- settleTransferNonTx branch coverage via CreateAndSettleTransfer ---

func TestCreateAndSettleTransfer_SenderUpdateFails(t *testing.T) {
	repo := newErrAccountRepo(map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 0),
	})
	repo.failOnAccountID = 1 // fail when updating sender

	svc := service.NewTransferServiceWithRepos(repo, &mockTransferRepo{}, &mockExchangeRateService{})
	if _, err := svc.CreateAndSettleTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 100,
	}); err == nil {
		t.Fatal("expected sender update error")
	}
}

func TestCreateAndSettleTransfer_ReceiverUpdateFails(t *testing.T) {
	repo := newErrAccountRepo(map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 0),
	})
	repo.failOnAccountID = 2 // fail when updating receiver

	svc := service.NewTransferServiceWithRepos(repo, &mockTransferRepo{}, &mockExchangeRateService{})
	if _, err := svc.CreateAndSettleTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 100,
	}); err == nil {
		t.Fatal("expected receiver update error")
	}
}

func TestCreateAndSettleTransfer_BankFromLookupFails(t *testing.T) {
	repo := newErrAccountRepo(map[uint]*models.Account{
		1: eurAccount(1, 1000),
		2: rsdAccount(2, 0),
	})
	repo.bankErr = errInjected

	svc := service.NewTransferServiceWithRepos(repo, &mockTransferRepo{}, &mockExchangeRateService{rate: 117.0})
	if _, err := svc.CreateAndSettleTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 100,
	}); err == nil {
		t.Fatal("expected bank lookup error")
	}
}

func TestCreateAndSettleTransfer_NonRSDToNonRSD_TouchesAllBankAccounts(t *testing.T) {
	// EUR -> USD: should trigger the IznosRSD>0 branch and update bankFrom (EUR), bankRSD, bankTo (USD).
	clientID := uint(1)
	usdAccount := &models.Account{
		ID: 3, RaspolozivoStanje: 0, Stanje: 0,
		DnevniLimit: 10000, MesecniLimit: 100000, CurrencyID: 3,
		ClientID: &clientID,
		Currency: models.Currency{ID: 3, Kod: "USD"},
		Client:   &models.Client{ID: 1},
	}
	repo := newCaptureRepo(map[uint]*models.Account{
		1: eurAccount(1, 1000),
		3: usdAccount,
	})
	svc := service.NewTransferServiceWithRepos(repo, &mockTransferRepo{}, &mockExchangeRateService{rate: 117.0})

	tr, err := svc.CreateAndSettleTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 3, Iznos: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != "uspesno" {
		t.Errorf("expected uspesno, got %s", tr.Status)
	}
	// IznosRSD branch was taken (non-RSD → non-RSD), so we expect at least one
	// update at bank account id 9000 (mock returns same id for all currencies).
	if _, ok := repo.updates[9000]; !ok {
		t.Error("expected bank account updates for cross-currency settlement")
	}
}

// --- ApproveTransferMobile / RejectTransfer error paths ---

func TestApproveTransferMobile_TransferNotFound(t *testing.T) {
	repo := newCaptureRepo(map[uint]*models.Account{})
	svc := service.NewTransferServiceWithRepos(repo, &mockTransferRepo{}, &mockExchangeRateService{})

	if _, _, _, err := svc.ApproveTransferMobile(999, "confirm"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestApproveTransferMobile_AlreadyCompleted_ReturnsSuccess(t *testing.T) {
	// Pre-load a completed transfer in the mock repo.
	completed := &models.Transfer{
		ID: 50, Status: "uspesno", RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
	}
	transferRepo := &mockTransferRepo{created: completed}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{}),
		transferRepo,
		&mockExchangeRateService{},
	)
	tr, _, _, err := svc.ApproveTransferMobile(50, "confirm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil || tr.Status != "uspesno" {
		t.Errorf("expected completed transfer returned, got %+v", tr)
	}
}

func TestRejectTransfer_NotFound(t *testing.T) {
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{}),
		&mockTransferRepo{},
		&mockExchangeRateService{},
	)
	if _, err := svc.RejectTransfer(123); err == nil {
		t.Fatal("expected not-found error")
	}
}

// --- prepareTransfer error branches ---

func TestCreateTransfer_SenderNotFound(t *testing.T) {
	svc := service.NewTransferServiceWithRepos(
		&mockAccountRepo{accounts: map[uint]*models.Account{2: rsdAccount(2, 0)}},
		&mockTransferRepo{},
		&mockExchangeRateService{},
	)
	if _, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 999, RacunPrimaocaID: 2, Iznos: 100,
	}); err == nil {
		t.Fatal("expected sender-not-found error")
	}
}

func TestCreateTransfer_ReceiverNotFound(t *testing.T) {
	svc := service.NewTransferServiceWithRepos(
		&mockAccountRepo{accounts: map[uint]*models.Account{1: rsdAccount(1, 5000)}},
		&mockTransferRepo{},
		&mockExchangeRateService{},
	)
	if _, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 999, Iznos: 100,
	}); err == nil {
		t.Fatal("expected receiver-not-found error")
	}
}

func TestCreateTransfer_DifferentClientOwners_ReturnsError(t *testing.T) {
	clientA := uint(1)
	clientB := uint(2)
	a := rsdAccount(1, 5000)
	b := rsdAccount(2, 0)
	a.ClientID = &clientA
	b.ClientID = &clientB
	repo := &mockAccountRepo{accounts: map[uint]*models.Account{1: a, 2: b}}

	svc := service.NewTransferServiceWithRepos(repo, &mockTransferRepo{}, &mockExchangeRateService{})
	if _, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 100,
	}); err == nil {
		t.Fatal("expected ownership-mismatch error")
	}
}

func TestCreateTransfer_AmountAboveDailyLimit_ReturnsError(t *testing.T) {
	clientID := uint(1)
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: {ID: 1, ClientID: &clientID, RaspolozivoStanje: 200000, Stanje: 200000, DnevniLimit: 100000, MesecniLimit: 1_000_000, CurrencyID: 1, Currency: models.Currency{Kod: "RSD"}},
		2: rsdAccount(2, 0),
	}}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	if _, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 150_000,
	}); err == nil {
		t.Fatal("expected daily-limit error")
	}
}

func TestCreateTransfer_CrossCurrency_ExchangeRateError(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: eurAccount(1, 1000),
		2: rsdAccount(2, 0),
	}}
	rateSvc := &mockExchangeRateService{err: errInjected}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, rateSvc)

	if _, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 100,
	}); err == nil {
		t.Fatal("expected exchange-rate error")
	}
}

// --- pendingTransferForMobile not-pending branch ---

func TestApproveTransferMobile_TransferCancelled_ReturnsVerificationError(t *testing.T) {
	cancelled := &models.Transfer{ID: 60, Status: "stornirano"}
	transferRepo := &mockTransferRepo{created: cancelled}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{}),
		transferRepo,
		&mockExchangeRateService{},
	)
	if _, _, _, err := svc.ApproveTransferMobile(60, "confirm"); err == nil {
		t.Fatal("expected verification error for non-pending transfer")
	}
}
