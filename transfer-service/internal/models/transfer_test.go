package models_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
)

func TestTransfer_GormTags(t *testing.T) {
	typ := reflect.TypeOf(models.Transfer{})

	tests := []struct {
		field    string
		contains string
	}{
		{"RacunPosiljaocaID", "not null"},
		{"RacunPrimaocaID", "not null"},
		{"Iznos", "not null"},
		{"Status", "default:'uspesno'"},
	}

	for _, tt := range tests {
		f, ok := typ.FieldByName(tt.field)
		if !ok {
			t.Errorf("field %q not found on Transfer", tt.field)
			continue
		}
		tag := f.Tag.Get("gorm")
		if tag == "" {
			t.Errorf("field %q has no gorm tag", tt.field)
			continue
		}
		found := false
		for i := 0; i <= len(tag)-len(tt.contains); i++ {
			if tag[i:i+len(tt.contains)] == tt.contains {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("field %q gorm tag %q does not contain %q", tt.field, tag, tt.contains)
		}
	}
}

func TestTransfer_StatusValues(t *testing.T) {
	validStatuses := []string{"uspesno", "neuspesno", "u_obradi"}
	for _, s := range validStatuses {
		tr := models.Transfer{
			RacunPosiljaocaID: 1,
			RacunPrimaocaID:   2,
			Iznos:             1000,
			Status:            s,
			VremeTransakcije:  time.Now(),
		}
		if tr.Status != s {
			t.Errorf("expected Status=%q, got %q", s, tr.Status)
		}
	}
}

func TestTransfer_ForeignKeyRelations(t *testing.T) {
	tr := models.Transfer{
		RacunPosiljaocaID: 1,
		RacunPrimaocaID:   2,
		Iznos:             500,
		Status:            "uspesno",
		RacunPosiljaoca:   models.Account{ID: 1, BrojRacuna: "000100000000000001"},
		RacunPrimaoca:     models.Account{ID: 2, BrojRacuna: "000100000000000002"},
	}
	if tr.RacunPosiljaoca.ID != 1 {
		t.Errorf("expected RacunPosiljaoca.ID=1, got %d", tr.RacunPosiljaoca.ID)
	}
	if tr.RacunPrimaoca.ID != 2 {
		t.Errorf("expected RacunPrimaoca.ID=2, got %d", tr.RacunPrimaoca.ID)
	}
}

func TestTransfer_CrossCurrencyFields(t *testing.T) {
	tr := models.Transfer{
		RacunPosiljaocaID: 1,
		RacunPrimaocaID:   2,
		Iznos:             100,
		ValutaIznosa:      "EUR",
		KonvertovaniIznos: 11700,
		Kurs:              117.0,
		Status:            "uspesno",
	}
	if tr.ValutaIznosa != "EUR" {
		t.Errorf("expected ValutaIznosa=EUR, got %q", tr.ValutaIznosa)
	}
	if tr.Kurs != 117.0 {
		t.Errorf("expected Kurs=117.0, got %f", tr.Kurs)
	}
	if tr.KonvertovaniIznos != 11700 {
		t.Errorf("expected KonvertovaniIznos=11700, got %f", tr.KonvertovaniIznos)
	}
}
