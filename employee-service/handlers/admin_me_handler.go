package handlers

import (
	"encoding/json"
	"net/http"

	"employee-service/middleware"
)
// AdminMeHandler godoc
// @Summary Admin test route
// @Description Verifies admin-only access
// @Tags employees
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Router /employees/admin/me [get]
func AdminMeHandler(w http.ResponseWriter, r *http.Request) {
	employeeID, _ := r.Context().Value(middleware.ContextEmployeeIDKey).(int64)
	email, _ := r.Context().Value(middleware.ContextEmailKey).(string)

	response := map[string]any{
		"message":     "admin access granted",
		"employee_id": employeeID,
		"email":       email,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}