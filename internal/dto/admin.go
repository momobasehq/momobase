package dto

import (
	"strings"

	"github.com/momobasehq/momobase/internal/domain"
)

// The administrative request payloads.
//
// Each carries the rules that can be judged from the request alone: presence, length,
// shape, and a closed set of allowed values. What cannot be judged here stays in the
// service that can — whether a role exists, whether a scope is in the catalogue,
// whether a provider account is real, whether the caller may change this row — because
// those need the database or the caller's identity, and neither is part of the body.

// CreateAdminRequest creates an administrator.
type CreateAdminRequest struct {
	Name string `json:"name" validate:"required,max=255" example:"Operations Admin"`
	// Email is stored lowercased, so it is matched case-insensitively at sign-in.
	Email    string `json:"email" format:"email" validate:"required,email" example:"ops@example.com"`
	Password string `json:"password" format:"password" validate:"required,min=8" example:"change-me-now"`
	// Role names a seeded or operator-created role; list them with GET /api/admin/roles.
	// It is checked against the roles table, which is why an empty value is allowed
	// here and defaulted there.
	Role string `json:"role" validate:"max=64" example:"operations"`
}

// Normalize trims the payload and lowercases the values stored in a canonical form.
func (r *CreateAdminRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	r.Role = strings.ToLower(strings.TrimSpace(r.Role))
}

// ChangePasswordRequest replaces an administrator password.
type ChangePasswordRequest struct {
	Password string `json:"password" format:"password" validate:"required,min=8" example:"change-me-now"`
}

// Normalize leaves the password untouched: every character in it is significant.
func (r *ChangePasswordRequest) Normalize() {}

// ChangeAdminStatusRequest activates or deactivates an administrator.
type ChangeAdminStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=active inactive" example:"active"`
}

// Normalize trims and lowercases the status.
func (r *ChangeAdminStatusRequest) Normalize() {
	r.Status = strings.ToLower(strings.TrimSpace(r.Status))
}

// ChangeRoleRequest reassigns an administrator to a different role.
type ChangeRoleRequest struct {
	// Role names a seeded or operator-created role; list them with GET /api/admin/roles.
	Role string `json:"role" validate:"required,max=64" example:"operations"`
}

// Normalize trims and lowercases the role name.
func (r *ChangeRoleRequest) Normalize() { r.Role = strings.ToLower(strings.TrimSpace(r.Role)) }

// RoleRequest creates or replaces a role's description and permissions.
type RoleRequest struct {
	// Name identifies the role and is what an administrator is assigned.
	Name string `json:"name" validate:"max=64" example:"support"`
	// Description explains what the role is for.
	Description string `json:"description" validate:"max=255" example:"Read-only support access"`
	// Permissions are the codes the role grants; each is checked against the catalogue.
	Permissions []string `json:"permissions" example:"transactions:read"`
}

// Normalize trims the role's text fields.
func (r *RoleRequest) Normalize() {
	r.Name = strings.ToLower(strings.TrimSpace(r.Name))
	r.Description = strings.TrimSpace(r.Description)
}

// CreateAppRequest creates an application.
type CreateAppRequest struct {
	Name        string `json:"name" validate:"required,max=255" example:"Checkout"`
	Description string `json:"description" validate:"max=1000" example:"Checkout payment application"`
	// Environment is defaulted by the service when omitted.
	Environment string                 `json:"environment" enums:"sandbox,production" validate:"omitempty,oneof=sandbox production" example:"sandbox"`
	Currency    string                 `json:"currency" validate:"required,len=3" example:"UGX"`
	Charges     *domain.ChargeSchedule `json:"charges,omitempty"`
}

// Normalize trims the name and lowercases the environment.
func (r *CreateAppRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Environment = strings.ToLower(strings.TrimSpace(r.Environment))
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
}

// UpdateAppRequest updates mutable application fields. An omitted field is left as it
// is, which is why nothing here is required.
type UpdateAppRequest struct {
	Name        string                 `json:"name,omitempty" validate:"max=255" example:"Checkout"`
	Description string                 `json:"description" validate:"max=1000" example:"Checkout payment application"`
	Environment string                 `json:"environment,omitempty" enums:"sandbox,production" validate:"omitempty,oneof=sandbox production" example:"sandbox"`
	Currency    string                 `json:"currency,omitempty" validate:"omitempty,len=3" example:"UGX"`
	Charges     *domain.ChargeSchedule `json:"charges,omitempty"`
}

// Normalize trims the name and lowercases the environment.
func (r *UpdateAppRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Environment = strings.ToLower(strings.TrimSpace(r.Environment))
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
}

// ChangeAppStatusRequest changes an application's status.
type ChangeAppStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=active disabled suspended" example:"active"`
}

// Normalize trims and lowercases the status.
func (r *ChangeAppStatusRequest) Normalize() {
	r.Status = strings.ToLower(strings.TrimSpace(r.Status))
}

// CreateCredentialRequest creates an application credential.
type CreateCredentialRequest struct {
	Name string `json:"name" validate:"max=255" example:"Backend credential"`
	// Scopes is a space-separated list checked against the application permission
	// catalogue, so a typo fails here rather than at the first payment.
	Scopes    string `json:"scopes" example:"collections:create transactions:read"`
	ExpiresAt string `json:"expires_at,omitempty" format:"date-time" example:"2030-01-02T03:04:05Z"`
}

// Normalize trims the credential's fields.
func (r *CreateCredentialRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Scopes = strings.TrimSpace(r.Scopes)
	r.ExpiresAt = strings.TrimSpace(r.ExpiresAt)
}

// CreateProviderAccountRequest creates a provider account.
type CreateProviderAccountRequest struct {
	// ProviderCode names an adapter registered in the running build; list them with
	// GET /api/admin/providers/registry.
	ProviderCode string                 `json:"provider_code" validate:"required,identifier" example:"dummy"`
	Name         string                 `json:"name" validate:"required,max=255" example:"Sandbox provider"`
	Environment  string                 `json:"environment" enums:"sandbox,production" validate:"omitempty,oneof=sandbox production" example:"sandbox"`
	Country      string                 `json:"country" validate:"required,country" example:"UG"`
	Currency     string                 `json:"currency" validate:"required,len=3" example:"UGX"`
	Charges      *domain.ChargeSchedule `json:"charges,omitempty"`
	// Config is the provider's own settings and must carry a webhook_secret. It is
	// encrypted before it is stored and is never returned.
	Config map[string]any `json:"config" swaggertype:"object" validate:"required"`
}

// Normalize trims the account's text fields and lowercases the provider code.
func (r *CreateProviderAccountRequest) Normalize() {
	r.ProviderCode = strings.ToLower(strings.TrimSpace(r.ProviderCode))
	r.Name = strings.TrimSpace(r.Name)
	r.Environment = strings.ToLower(strings.TrimSpace(r.Environment))
	r.Country = strings.ToUpper(strings.TrimSpace(r.Country))
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
}

// UpdateProviderSettingsRequest atomically replaces a provider account's routing
// location and fee schedule.
type UpdateProviderSettingsRequest struct {
	Country  string                `json:"country" validate:"required,country" example:"UG"`
	Currency string                `json:"currency" validate:"required,len=3" example:"UGX"`
	Charges  domain.ChargeSchedule `json:"charges" validate:"required"`
}

// Normalize canonicalizes the settings before validation and persistence.
func (r *UpdateProviderSettingsRequest) Normalize() {
	r.Country = strings.ToUpper(strings.TrimSpace(r.Country))
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
}

// UpdateProviderConfigRequest replaces provider configuration.
type UpdateProviderConfigRequest struct {
	Config map[string]any `json:"config" swaggertype:"object" validate:"required"`
}

// Normalize leaves the configuration alone: its keys and values belong to the provider.
func (r *UpdateProviderConfigRequest) Normalize() {}

// CreateRouteRequest creates a payment route.
type CreateRouteRequest struct {
	ServiceType   string `json:"service_type" enums:"collection,disbursement" validate:"required,oneof=collection disbursement" example:"collection"`
	PaymentMethod string `json:"payment_method" validate:"required,identifier" example:"momo"`
	// ProviderAccountID must name an existing account, which the service checks.
	ProviderAccountID string `json:"provider_account_id" validate:"required" example:"pacc_123"`
	Priority          int    `json:"priority" minimum:"1" validate:"omitempty,min=1" example:"1"`
	Active            bool   `json:"active" example:"true"`
}

// Normalize trims and lowercases the values the route is matched on.
func (r *CreateRouteRequest) Normalize() {
	r.ServiceType = strings.ToLower(strings.TrimSpace(r.ServiceType))
	r.PaymentMethod = strings.ToLower(strings.TrimSpace(r.PaymentMethod))
	r.ProviderAccountID = strings.TrimSpace(r.ProviderAccountID)
}

// UpdateRouteRequest updates a payment route.
type UpdateRouteRequest struct {
	Priority int  `json:"priority" minimum:"1" validate:"min=1" example:"1"`
	Active   bool `json:"active" example:"true"`
}

// Normalize has nothing to do: both fields are already typed.
func (r *UpdateRouteRequest) Normalize() {}
