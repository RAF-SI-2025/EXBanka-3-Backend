package models_test

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
)

func TestValidPeriods_HasEntryForEachLoanType(t *testing.T) {
	periods := models.ValidPeriods()
	for _, vrsta := range models.ValidLoanTypes() {
		if _, ok := periods[vrsta]; !ok {
			t.Errorf("expected period entry for loan type %q", vrsta)
		}
	}
}

func TestValidPeriods_StambeniHasLongerPeriods(t *testing.T) {
	periods := models.ValidPeriods()
	stambeni := periods["stambeni"]
	found := false
	for _, p := range stambeni {
		if p == 360 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected stambeni to allow 360 months, got %v", stambeni)
	}
}

func TestValidEmploymentStatuses_ContainsExpected(t *testing.T) {
	statuses := models.ValidEmploymentStatuses()
	expected := []string{"stalno", "privremeno", "nezaposlen"}
	for _, e := range expected {
		assertContains(t, statuses, e)
	}
}
