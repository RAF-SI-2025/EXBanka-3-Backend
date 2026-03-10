package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"employee-service/models"
)

type AuthClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewAuthClient(baseURL string) *AuthClient {
	return &AuthClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *AuthClient) CreateCredential(employeeID int64, email string, isActive bool) (*models.CreateCredentialResponse, error) {
	reqBody := models.CreateCredentialRequest{
		EmployeeID: employeeID,
		Email:      email,
		IsActive:   isActive,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/auth/internal/create-credential", c.BaseURL)

	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("auth-service returned status %d", resp.StatusCode)
	}

	var result models.CreateCredentialResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}