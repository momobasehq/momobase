package admin

import (
	"net/http"
	"strings"

	"github.com/momobasehq/momobase/internal/platform"
)

// RoleRequest creates or replaces a role's description and permissions.
type RoleRequest struct {
	// Name identifies the role. It is ignored when updating, since the path carries it.
	Name string `json:"name" example:"support"`
	// Description explains the role in an operator-facing list.
	Description string `json:"description" example:"Read transactions and reissue credentials"`
	// Permissions are the codes the role grants, from GET /api/admin/permissions.
	Permissions []string `json:"permissions" example:"transactions:read,credentials:update"`
}

// ListPermissions writes the seeded permission catalogue.
//
// @Summary List assignable permissions
// @Tags Admin - Roles
// @Produce json
// @Security BearerAuth
// @Param audience query string false "Filter by admin or app" Enums(admin, app)
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Router /api/admin/permissions [get]
func (h *Handler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	audience := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("audience")))
	permissions, err := h.authz.ListPermissions(r.Context(), audience)
	if err != nil {
		platform.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	platform.JSON(w, http.StatusOK, map[string]any{"items": permissions, "count": len(permissions)})
}

// ListRoles writes every role with the permissions it grants.
//
// @Summary List roles
// @Tags Admin - Roles
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Router /api/admin/roles [get]
func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.authz.ListRoles(r.Context())
	if err != nil {
		platform.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	platform.JSON(w, http.StatusOK, map[string]any{"items": roles, "count": len(roles)})
}

// CreateRole creates a custom role.
//
// @Summary Create a role
// @Tags Admin - Roles
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body RoleRequest true "Role definition"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Router /api/admin/roles [post]
func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	body, err := platform.DecodeJSON[RoleRequest](r)
	if err != nil {
		platform.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, http.StatusCreated, "ROLE_ERROR", func() (Response, error) {
		return h.authz.CreateRole(r.Context(), actor(r), body.Name, body.Description, body.Permissions)
	})
}

// UpdateRole replaces a custom role's description and permissions.
//
// @Summary Update a role
// @Tags Admin - Roles
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param name path string true "Role name"
// @Param request body RoleRequest true "Replacement description and permissions"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 404 {object} apidoc.ErrorResponse
// @Router /api/admin/roles/{name} [patch]
func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	body, err := platform.DecodeJSON[RoleRequest](r)
	if err != nil {
		platform.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, http.StatusOK, "ROLE_ERROR", func() (Response, error) {
		err := h.authz.UpdateRole(r.Context(), actor(r), r.PathValue("name"), body.Description, body.Permissions)
		return map[string]bool{"ok": err == nil}, err
	})
}

// DeleteRole removes a custom role that no administrator holds.
//
// @Summary Delete a role
// @Tags Admin - Roles
// @Produce json
// @Security BearerAuth
// @Param name path string true "Role name"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 404 {object} apidoc.ErrorResponse
// @Router /api/admin/roles/{name} [delete]
func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, "ROLE_ERROR", func() (Response, error) {
		err := h.authz.DeleteRole(r.Context(), actor(r), r.PathValue("name"))
		return map[string]bool{"ok": err == nil}, err
	})
}
