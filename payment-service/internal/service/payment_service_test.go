package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
)

type mockAccountRepo struct {
	accounts map[uint]*models.Account
	byBroj   map[string]*models.Account
	updated  map[uint]map[string]interface{}
	findErr  error
}

func newMockAccountRepo(accounts ...*models.Account) *mockAccountRepo {
	repo := &mockAccountRepo{
		accounts: make(map[uint]*models.Account),
		byBroj:   make(map[string]*models.Account),
		updated:  make(map[uint]map[string]interface{}),
	}
	for _, account := range accounts {
		repo.accounts[account.ID] = account
		repo.byBroj[account.BrojRacuna] = account
	}
	return repo
}

func (m *mockAccountRepo) FindByID(id uint) (*models.Account, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	if account, ok := m.accounts[id]; ok {
		return account, nil
	}
	return nil, errors.New("account not found")
}

func (m *mockAccountRepo) FindByBrojRacuna(brojRacuna string) (*models.Account, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	if account, ok := m.byBroj[brojRacuna]; ok {
		return account, nil
	}
	return nil, errors.New("account not found")
}

func (m *mockAccountRepo) UpdateFields(id uint, fields map[string]interface{}) error {
	copyFields := make(map[string]interface{}, len(fields))
	for key, value := range fields {
		copyFields[key] = value
	}
	m.updated[id] = copyFields

	account, ok := m.accounts[id]
	if !ok {
		return nil
	}

	if value, ok := fields["stanje"].(float64); ok {
		account.Stanje = value
	}
	if value, ok := fields["raspolozivo_stanje"].(float64); ok {
		account.RaspolozivoStanje = value
	}
	if value, ok := fields["dnevna_potrosnja"].(float64); ok {
		account.DnevnaPotrosnja = value
	}
	if value, ok := fields["mesecna_potrosnja"].(float64); ok {
		account.MesecnaPotrosnja = value
	}

	return nil
}

type mockPaymentRepo struct {
	created   *models.Payment
	saved     *models.Payment
	findByID  map[uint]*models.Payment
	nextID    uint
	createErr error
	saveErr   error
	findErr   error
}

func newMockPaymentRepo() *mockPaymentRepo {
	return &mockPaymentRepo{
		findByID: make(map[uint]*models.Payment),
		nextID:   1,
	}
}

func (m *mockPaymentRepo) Create(p *models.Payment) error {
	if m.createErr != nil {
		return m.createErr
	}
	p.ID = m.nextID
	m.nextID++
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	m.created = p
	m.findByID[p.ID] = p
	return nil
}

func (m *mockPaymentRepo) FindByID(id uint) (*models.Payment, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	if payment, ok := m.findByID[id]; ok {
		return payment, nil
	}
	return nil, errors.New("payment not found")
}

func (m *mockPaymentRepo) Save(p *models.Payment) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = p
	m.findByID[p.ID] = p
	return nil
}

func (m *mockPaymentRepo) ListByAccountID(accountID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	return nil, 0, nil
}

func (m *mockPaymentRepo) ListByClientID(clientID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	return nil, 0, nil
}

type mockCreateRecipientRepo struct {
	created *models.PaymentRecipient
	nextID  uint
}

func (m *mockCreateRecipientRepo) Create(r *models.PaymentRecipient) error {
	m.nextID++
	r.ID = m.nextID
	m.created = r
	return nil
}

type mockNotifier struct {
	sent    bool
	to      string
	name    string
	code    string
	sendErr error
}

func (m *mockNotifier) SendVerificationCode(toEmail, clientName, code string, iznos float64, svrha, primaocRacun string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = true
	m.to = toEmail
	m.name = clientName
	m.code = code
	return nil
}

func rsdAccount(id uint, brojRacuna string, balance float64, clientID *uint) *models.Account {
	return &models.Account{
		ID:                id,
		BrojRacuna:        brojRacuna,
		ClientID:          clientID,
		CurrencyID:        1,
		CurrencyKod:       "RSD",
		RaspolozivoStanje: balance,
		Stanje:            balance,
		DnevniLimit:       100000,
		MesecniLimit:      1000000,
		Status:            "aktivan",
	}
}

func eurAccount(id uint, brojRacuna string, balance float64, clientID *uint) *models.Account {
	account := rsdAccount(id, brojRacuna, balance, clientID)
	account.CurrencyID = 2
	account.CurrencyKod = "EUR"
	return account
}

func TestCreatePayment_Success_StatusPendingAndVerificationSent(t *testing.T) {
	clientID := uint(7)
	sender := rsdAccount(1, "111111111111111111", 5000, &clientID)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	notifier := &mockNotifier{}
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, notifier)

	payment, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             500,
		Svrha:             "Test",
		ClientEmail:       "client@example.com",
		ClientName:        "Test Client",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payment.Status != "u_obradi" {
		t.Fatalf("expected pending status, got %s", payment.Status)
	}
	if payment.RacunPrimaocaID == nil || *payment.RacunPrimaocaID != receiver.ID {
		t.Fatalf("expected receiver account id %d, got %+v", receiver.ID, payment.RacunPrimaocaID)
	}
	if payment.VerificationExpiresAt == nil {
		t.Fatal("expected verification expiry to be set")
	}
	if !notifier.sent {
		t.Fatal("expected verification code to be sent")
	}
}

func TestCreatePayment_RejectsUnsupportedCurrency(t *testing.T) {
	clientID := uint(7)
	sender := eurAccount(1, "111111111111111111", 5000, &clientID)
	receiver := eurAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	svc := service.NewPaymentServiceWithRepos(accountRepo, newMockPaymentRepo(), nil, nil)

	_, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             100,
	})

	if err == nil {
		t.Fatal("expected unsupported currency error, got nil")
	}
}

func TestCreatePayment_SendFailure_CancelsPayment(t *testing.T) {
	clientID := uint(7)
	sender := rsdAccount(1, "111111111111111111", 5000, &clientID)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	notifier := &mockNotifier{sendErr: errors.New("smtp down")}
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, notifier)

	_, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             100,
		ClientEmail:       "client@example.com",
		ClientName:        "Test Client",
	})

	if err == nil {
		t.Fatal("expected notification failure, got nil")
	}
	if paymentRepo.saved == nil || paymentRepo.saved.Status != "stornirano" {
		t.Fatalf("expected cancelled payment after send failure, got %+v", paymentRepo.saved)
	}
}

func TestVerifyPayment_Success_SettlesSenderReceiverAndSpending(t *testing.T) {
	clientID := uint(7)
	sender := rsdAccount(1, "111111111111111111", 5000, &clientID)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, nil)

	created, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             300,
		Svrha:             "Settlement",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	verified, err := svc.VerifyPayment(created.ID, created.VerifikacioniKod)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	if verified.Status != "uspesno" {
		t.Fatalf("expected successful payment, got %s", verified.Status)
	}
	if verified.VerifikacioniKod != "" {
		t.Fatal("expected verification code to be cleared after success")
	}
	if verified.VerificationExpiresAt != nil {
		t.Fatal("expected verification expiry to be cleared after success")
	}

	senderUpdate := accountRepo.updated[sender.ID]
	if senderUpdate["raspolozivo_stanje"].(float64) != 4700 {
		t.Fatalf("expected sender available balance 4700, got %v", senderUpdate["raspolozivo_stanje"])
	}
	if senderUpdate["dnevna_potrosnja"].(float64) != 300 {
		t.Fatalf("expected sender daily spending 300, got %v", senderUpdate["dnevna_potrosnja"])
	}
	if senderUpdate["mesecna_potrosnja"].(float64) != 300 {
		t.Fatalf("expected sender monthly spending 300, got %v", senderUpdate["mesecna_potrosnja"])
	}

	receiverUpdate := accountRepo.updated[receiver.ID]
	if receiverUpdate["raspolozivo_stanje"].(float64) != 400 {
		t.Fatalf("expected receiver available balance 400, got %v", receiverUpdate["raspolozivo_stanje"])
	}
	if receiverUpdate["stanje"].(float64) != 400 {
		t.Fatalf("expected receiver balance 400, got %v", receiverUpdate["stanje"])
	}
}

func TestVerifyPayment_WrongCode_IncrementsAttempts(t *testing.T) {
	sender := rsdAccount(1, "111111111111111111", 5000, nil)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, nil)

	created, _ := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             100,
	})

	_, err := svc.VerifyPayment(created.ID, "000000")
	if err == nil {
		t.Fatal("expected wrong code error, got nil")
	}
	if paymentRepo.saved == nil || paymentRepo.saved.BrojPokusaja != 1 {
		t.Fatalf("expected one failed attempt, got %+v", paymentRepo.saved)
	}
}

func TestVerifyPayment_ThreeWrongCodes_CancelsPayment(t *testing.T) {
	sender := rsdAccount(1, "111111111111111111", 5000, nil)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, nil)

	created, _ := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             100,
	})

	for i := 0; i < 3; i++ {
		_, _ = svc.VerifyPayment(created.ID, "000000")
	}

	if paymentRepo.saved == nil || paymentRepo.saved.Status != "stornirano" {
		t.Fatalf("expected cancelled payment after 3 wrong codes, got %+v", paymentRepo.saved)
	}
}

func TestVerifyPayment_ExpiredCode_CancelsPayment(t *testing.T) {
	sender := rsdAccount(1, "111111111111111111", 5000, nil)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, nil)

	created, _ := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             100,
	})
	expired := time.Now().UTC().Add(-time.Minute)
	created.VerificationExpiresAt = &expired

	_, err := svc.VerifyPayment(created.ID, created.VerifikacioniKod)
	if err == nil {
		t.Fatal("expected expired verification code error, got nil")
	}
	if paymentRepo.saved == nil || paymentRepo.saved.Status != "stornirano" {
		t.Fatalf("expected cancelled payment after expiry, got %+v", paymentRepo.saved)
	}
}

func TestVerifyPayment_InsufficientBalanceAtVerify_CancelsPayment(t *testing.T) {
	sender := rsdAccount(1, "111111111111111111", 5000, nil)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, nil)

	created, _ := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             200,
	})
	sender.Stanje = 0
	sender.RaspolozivoStanje = 0

	_, err := svc.VerifyPayment(created.ID, created.VerifikacioniKod)
	if err == nil {
		t.Fatal("expected insufficient balance error, got nil")
	}
	if paymentRepo.saved == nil || paymentRepo.saved.Status != "stornirano" {
		t.Fatalf("expected cancelled payment after verify-time insufficiency, got %+v", paymentRepo.saved)
	}
}

func TestVerifyPayment_CannotReuseSuccessfulCode(t *testing.T) {
	sender := rsdAccount(1, "111111111111111111", 5000, nil)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, nil)

	created, _ := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             50,
	})

	if _, err := svc.VerifyPayment(created.ID, created.VerifikacioniKod); err != nil {
		t.Fatalf("expected first verification to succeed, got %v", err)
	}
	if _, err := svc.VerifyPayment(created.ID, created.VerifikacioniKod); err == nil {
		t.Fatal("expected second verification to fail, got nil")
	}
}

func TestCreatePayment_WithAddRecipient_CreatesRecipient(t *testing.T) {
	clientID := uint(7)
	sender := rsdAccount(1, "111111111111111111", 5000, &clientID)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	recipientRepo := &mockCreateRecipientRepo{}
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, recipientRepo, nil)

	_, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             100,
		AddRecipient:      true,
		RecipientNaziv:    "Novi Primalac",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recipientRepo.created == nil {
		t.Fatal("expected recipient to be created, got nil")
	}
	if recipientRepo.created.ClientID != clientID {
		t.Fatalf("expected recipient client id %d, got %d", clientID, recipientRepo.created.ClientID)
	}
}

func TestApprovePaymentMobile_CodeReturnsExistingCodeWithoutSettlement(t *testing.T) {
	sender := rsdAccount(1, "111111111111111111", 5000, nil)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, nil)

	created, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             100,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	payment, code, expiresAt, err := svc.ApprovePaymentMobile(created.ID, "code")
	if err != nil {
		t.Fatalf("approve mobile failed: %v", err)
	}
	if payment.Status != "u_obradi" {
		t.Fatalf("expected payment to remain pending, got %s", payment.Status)
	}
	if code == "" {
		t.Fatal("expected verification code to be returned")
	}
	if expiresAt == nil {
		t.Fatal("expected verification expiry to be returned")
	}
	if len(accountRepo.updated) != 0 {
		t.Fatal("expected no balance updates in code mode")
	}
}

func TestApprovePaymentMobile_ConfirmExecutesSettlement(t *testing.T) {
	sender := rsdAccount(1, "111111111111111111", 5000, nil)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, nil)

	created, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             125,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	payment, code, expiresAt, err := svc.ApprovePaymentMobile(created.ID, "confirm")
	if err != nil {
		t.Fatalf("confirm mobile failed: %v", err)
	}
	if payment.Status != "uspesno" {
		t.Fatalf("expected completed payment, got %s", payment.Status)
	}
	if code != "" || expiresAt != nil {
		t.Fatal("expected confirm mode not to return verification code or expiry")
	}
	if accountRepo.updated[sender.ID]["raspolozivo_stanje"].(float64) != 4875 {
		t.Fatalf("expected sender balance 4875, got %v", accountRepo.updated[sender.ID]["raspolozivo_stanje"])
	}
	if accountRepo.updated[receiver.ID]["raspolozivo_stanje"].(float64) != 225 {
		t.Fatalf("expected receiver balance 225, got %v", accountRepo.updated[receiver.ID]["raspolozivo_stanje"])
	}
}

func TestRejectPayment_CancelsPendingPayment(t *testing.T) {
	sender := rsdAccount(1, "111111111111111111", 5000, nil)
	receiver := rsdAccount(2, "222222222222222222", 100, nil)
	accountRepo := newMockAccountRepo(sender, receiver)
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil, nil)

	created, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: sender.ID,
		RacunPrimaocaBroj: receiver.BrojRacuna,
		Iznos:             75,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	rejected, err := svc.RejectPayment(created.ID)
	if err != nil {
		t.Fatalf("reject mobile failed: %v", err)
	}
	if rejected.Status != "stornirano" {
		t.Fatalf("expected cancelled payment, got %s", rejected.Status)
	}
	if len(accountRepo.updated) != 0 {
		t.Fatal("expected reject not to touch balances")
	}
}
