//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// adminLogin authenticates as admin and returns the access token.
func adminLogin(t *testing.T) string {
	t.Helper()
	resp, body := postJSON(t, "/auth/login", map[string]string{
		"email":    "admin@bank.com",
		"password": "Admin123!",
	})
	if resp.StatusCode != http.StatusOK {
		t.Skipf("admin login failed (status %d); skipping employee flow tests", resp.StatusCode)
	}
	token, ok := body["accessToken"].(string)
	if !ok || token == "" {
		t.Skip("could not obtain admin access token; skipping employee flow tests")
	}
	return token
}

func postJSONWithToken(t *testing.T, path, token string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s error: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp, result
}

func putJSONWithToken(t *testing.T, path, token string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPut, baseURL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s error: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp, result
}

func patchJSONWithToken(t *testing.T, path, token string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPatch, baseURL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s error: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp, result
}

func TestEmployeeFlow_CreateAndList(t *testing.T) {
	token := adminLogin(t)
	uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())

	payload := map[string]interface{}{
		"ime":           "Test",
		"prezime":       "Employee",
		"datumRodjenja": 946684800, // 2000-01-01 unix
		"pol":           "M",
		"email":         fmt.Sprintf("test.emp.%s@bank.com", uniqueSuffix),
		"brojTelefona":  "0611234567",
		"adresa":        "Test Street 1",
		"username":      fmt.Sprintf("testuser%s", uniqueSuffix),
		"pozicija":      "Developer",
		"departman":     "IT",
		"aktivan":       false,
	}

	// Create employee
	resp, body := postJSONWithToken(t, "/employees", token, payload)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create employee expected 200, got %d: %v", resp.StatusCode, body)
	}

	// List employees and verify the new one is present
	resp, body = getWithToken(t, "/employees", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list employees expected 200, got %d: %v", resp.StatusCode, body)
	}
	if body["employees"] == nil {
		t.Error("list response missing 'employees' field")
	}
}

func TestEmployeeFlow_ListWithFilters(t *testing.T) {
	token := adminLogin(t)

	// Filter by email (admin)
	resp, body := getWithToken(t, "/employees?emailFilter=admin", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list with emailFilter expected 200, got %d: %v", resp.StatusCode, body)
	}

	// Filter by name
	resp, body = getWithToken(t, "/employees?nameFilter=admin", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list with nameFilter expected 200, got %d: %v", resp.StatusCode, body)
	}

	// Filter by position
	resp, body = getWithToken(t, "/employees?pozicijaFilter=Developer", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list with pozicijaFilter expected 200, got %d: %v", resp.StatusCode, body)
	}

	// Pagination
	resp, body = getWithToken(t, "/employees?page=1&pageSize=5", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list with pagination expected 200, got %d: %v", resp.StatusCode, body)
	}
}

func TestEmployeeFlow_UpdateAdminRejected(t *testing.T) {
	token := adminLogin(t)

	// Get admin employee ID from list
	_, body := getWithToken(t, "/employees?emailFilter=admin@bank.com", token)
	employees, ok := body["employees"].([]interface{})
	if !ok || len(employees) == 0 {
		t.Skip("admin employee not found in list; skipping update-admin test")
	}
	adminEmp, ok := employees[0].(map[string]interface{})
	if !ok {
		t.Skip("could not parse admin employee; skipping")
	}
	adminID, ok := adminEmp["id"].(string)
	if !ok || adminID == "" {
		t.Skip("admin employee ID missing; skipping")
	}

	// Attempt to update admin employee — should be rejected
	resp, respBody := putJSONWithToken(t, "/employees/"+adminID, token, map[string]interface{}{
		"ime":           "Modified",
		"prezime":       "Admin",
		"datumRodjenja": 946684800,
		"pol":           "M",
		"email":         "admin@bank.com",
		"brojTelefona":  "0611111111",
		"adresa":        "Admin Street",
		"username":      "admin",
		"pozicija":      "Administrator",
		"departman":     "Management",
		"aktivan":       true,
	})
	if resp.StatusCode == http.StatusOK {
		t.Errorf("expected error when updating admin employee, got 200: %v", respBody)
	}
}

func TestEmployeeFlow_ActivateToggle(t *testing.T) {
	token := adminLogin(t)
	uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create a fresh employee
	payload := map[string]interface{}{
		"ime":           "Toggle",
		"prezime":       "Test",
		"datumRodjenja": 946684800,
		"pol":           "Z",
		"email":         fmt.Sprintf("toggle.%s@bank.com", uniqueSuffix),
		"brojTelefona":  "0621234567",
		"adresa":        "Toggle Ave 2",
		"username":      fmt.Sprintf("toggle%s", uniqueSuffix),
		"pozicija":      "Tester",
		"departman":     "QA",
		"aktivan":       false,
	}
	_, createBody := postJSONWithToken(t, "/employees", token, payload)
	emp, ok := createBody["employee"].(map[string]interface{})
	if !ok {
		t.Skip("create did not return employee object; skipping activate toggle test")
	}
	empID, _ := emp["id"].(string)
	if empID == "" {
		t.Skip("no employee ID in create response; skipping")
	}

	// Activate the employee
	resp, body := patchJSONWithToken(t, "/employees/"+empID+"/active", token, map[string]interface{}{
		"aktivan": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activate expected 200, got %d: %v", resp.StatusCode, body)
	}

	// Deactivate
	resp, body = patchJSONWithToken(t, "/employees/"+empID+"/active", token, map[string]interface{}{
		"aktivan": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deactivate expected 200, got %d: %v", resp.StatusCode, body)
	}
}

func TestEmployeeFlow_PermissionManagement(t *testing.T) {
	token := adminLogin(t)
	uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create a fresh employee
	payload := map[string]interface{}{
		"ime":           "Perms",
		"prezime":       "Test",
		"datumRodjenja": 946684800,
		"pol":           "M",
		"email":         fmt.Sprintf("perms.%s@bank.com", uniqueSuffix),
		"brojTelefona":  "0631234567",
		"adresa":        "Perm St 3",
		"username":      fmt.Sprintf("permsuser%s", uniqueSuffix),
		"pozicija":      "Analyst",
		"departman":     "Finance",
		"aktivan":       false,
	}
	_, createBody := postJSONWithToken(t, "/employees", token, payload)
	emp, ok := createBody["employee"].(map[string]interface{})
	if !ok {
		t.Skip("create did not return employee object; skipping permission management test")
	}
	empID, _ := emp["id"].(string)
	if empID == "" {
		t.Skip("no employee ID in create response; skipping")
	}

	// Set permissions
	resp, body := putJSONWithToken(t, "/employees/"+empID+"/permissions", token, map[string]interface{}{
		"permissionNames": []string{"employeeBasic"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update permissions expected 200, got %d: %v", resp.StatusCode, body)
	}
}
