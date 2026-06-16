package repository

import (
	"fmt"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Notification{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestNotificationRepository_CRUD(t *testing.T) {
	db := openTestDB(t, "notif_repo_crud")
	repo := NewNotificationRepository(db)

	// Create three for client 100, one read; one for employee 5.
	for i := 0; i < 3; i++ {
		if err := repo.Create(&models.Notification{UserID: 100, UserType: "client", Type: "ORDER_CREATED", Title: fmt.Sprintf("t%d", i)}); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	if err := repo.Create(&models.Notification{UserID: 5, UserType: "employee", Type: "X", Title: "emp"}); err != nil {
		t.Fatalf("create employee: %v", err)
	}

	// ListByUser scopes to the user.
	all, err := repo.ListByUser(100, "client", false, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 client notifications, got %d", len(all))
	}

	// UnreadCount = 3 before any are read.
	if c, _ := repo.UnreadCount(100, "client"); c != 3 {
		t.Errorf("unread count: want 3, got %d", c)
	}

	// Mark the first read; unread filter + count drop.
	first := all[len(all)-1] // oldest (created_at DESC ordering)
	if err := repo.MarkRead(first.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	unread, _ := repo.ListByUser(100, "client", true, 0)
	if len(unread) != 2 {
		t.Errorf("unread list: want 2, got %d", len(unread))
	}
	if c, _ := repo.UnreadCount(100, "client"); c != 2 {
		t.Errorf("unread count after one read: want 2, got %d", c)
	}

	// GetByID found + missing.
	got, err := repo.GetByID(first.ID)
	if err != nil || got == nil {
		t.Fatalf("get by id: %v / %v", got, err)
	}
	if missing, err := repo.GetByID(99999); err != nil || missing != nil {
		t.Errorf("get missing: expected nil,nil got %v,%v", missing, err)
	}

	// MarkAllRead zeroes the unread count.
	if err := repo.MarkAllRead(100, "client"); err != nil {
		t.Fatalf("mark all read: %v", err)
	}
	if c, _ := repo.UnreadCount(100, "client"); c != 0 {
		t.Errorf("unread after mark-all: want 0, got %d", c)
	}

	// Delete removes a row.
	if err := repo.Delete(first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if after, _ := repo.ListByUser(100, "client", false, 0); len(after) != 2 {
		t.Errorf("after delete: want 2, got %d", len(after))
	}
}

func TestNotificationRepository_ListLimitClamp(t *testing.T) {
	db := openTestDB(t, "notif_repo_limit")
	repo := NewNotificationRepository(db)
	for i := 0; i < 5; i++ {
		_ = repo.Create(&models.Notification{UserID: 1, UserType: "client", Type: "T", Title: "x"})
	}
	// An out-of-range limit (>200) is clamped to the default 50, not rejected.
	out, err := repo.ListByUser(1, "client", false, 9999)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 5 {
		t.Errorf("expected all 5 with clamped limit, got %d", len(out))
	}
	// An explicit small limit is honoured.
	if out, _ := repo.ListByUser(1, "client", false, 2); len(out) != 2 {
		t.Errorf("expected 2 with limit=2, got %d", len(out))
	}
}
