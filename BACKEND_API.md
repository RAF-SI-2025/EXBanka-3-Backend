# EXBanka Backend API

This document describes the backend API for the EXBanka project.

Architecture consists of two main microservices:

- auth-service
- employee-service

---

# Auth Service

Handles authentication and credential management.

Base path:

/auth

---

## Create Credential (internal)

POST /auth/internal/create-credential

Used by employee-service to create login credentials.

Request:

{
  "employee_id": 1,
  "email": "user@test.com",
  "is_active": false
}

Response:

{
  "credential": {
    "id": 1,
    "employee_id": 1,
    "email": "user@test.com",
    "activation_token": "..."
  }
}

---

## Activate Account

POST /auth/activate

Activates user account and sets password.

Request:

{
  "activation_token": "...",
  "password": "Password123",
  "confirm_password": "Password123"
}

---

## Login

POST /auth/login

Request:

{
  "email": "user@test.com",
  "password": "Password123"
}

Response:

{
  "access_token": "...",
  "refresh_token": "...",
  "token_type": "Bearer"
}

---

## Refresh Token

POST /auth/refresh

Request:

{
  "refresh_token": "..."
}

Response:

{
  "access_token": "...",
  "token_type": "Bearer"
}

---

## Forgot Password

POST /auth/forgot-password

Request:

{
  "email": "user@test.com"
}

---

## Reset Password

POST /auth/reset-password

Request:

{
  "reset_token": "...",
  "password": "...",
  "confirm_password": "..."
}

---

# Employee Service

Handles employee data and permissions.

Base path:

/employees

All routes require admin authentication except health endpoint.

---

## Health Check

GET /employees/health

---

## Create Employee

POST /employees

Request:

{
  "first_name": "Petar",
  "last_name": "Petrovic",
  "email": "petar@test.com",
  "position": "Manager",
  "department": "Finance"
}

Creates employee and automatically creates credentials in auth-service.

---

## List Employees

GET /employees

Supports filters:

/employees?email=test  
/employees?first_name=petar  
/employees?last_name=petrovic  
/employees?position=manager

---

## Get Employee by ID

GET /employees/{id}

Example:

/employees/1

---

## Update Employee

PUT /employees/{id}

Updates employee information.

---

## Activate / Deactivate Employee

PATCH /employees/{id}/active

Request:

{
  "active": false
}

---

## Get Permissions

GET /employees/{id}/permissions

---

## Update Permissions

PUT /employees/{id}/permissions

Request:

{
  "permissions": ["admin", "view_employees"]
}

---

## Admin Test Route

GET /employees/admin/me

Used to verify admin authentication.

Requires header:

Authorization: Bearer <access_token>

---

# Authentication

Protected routes require JWT access token.

Header:

Authorization: Bearer <access_token>

Access token is obtained from:

POST /auth/login

---

# Next Backend Steps

Planned improvements:

- notification-service (email sending)
- activation email
- reset password email
- Swagger/OpenAPI documentation
- API gateway integration