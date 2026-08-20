package admin

import (
	"github.com/gofiber/fiber/v3"

	"strings"

	"github.com/momobasehq/momobase/internal/dto"
	"github.com/momobasehq/momobase/internal/platform"
)

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
func (h *Handler) ListPermissions(c fiber.Ctx) error {
	audience := strings.ToLower(strings.TrimSpace(c.Query("audience")))
	permissions, err := h.authz.ListPermissions(c.Context(), audience)
	if err != nil {
		return platform.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	return platform.JSON(c, fiber.StatusOK, map[string]any{"items": permissions, "count": len(permissions)})
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
func (h *Handler) ListRoles(c fiber.Ctx) error {
	roles, err := h.authz.ListRoles(c.Context())
	if err != nil {
		return platform.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	return platform.JSON(c, fiber.StatusOK, map[string]any{"items": roles, "count": len(roles)})
}

// CreateRole creates a custom role.
//
// @Summary Create a role
// @Tags Admin - Roles
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.RoleRequest true "Role definition"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Router /api/admin/roles [post]
func (h *Handler) CreateRole(c fiber.Ctx) error {
	body, err := bind[dto.RoleRequest](c)
	if err != nil {
		return platform.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, fiber.StatusCreated, "ROLE_ERROR", func() (Response, error) {
		return h.authz.CreateRole(c.Context(), actor(c), body.Name, body.Description, body.Permissions)
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
// @Param request body dto.RoleRequest true "Replacement description and permissions"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 404 {object} apidoc.ErrorResponse
// @Router /api/admin/roles/{name} [patch]
func (h *Handler) UpdateRole(c fiber.Ctx) error {
	body, err := bind[dto.RoleRequest](c)
	if err != nil {
		return platform.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, fiber.StatusOK, "ROLE_ERROR", func() (Response, error) {
		err := h.authz.UpdateRole(c.Context(), actor(c), c.Params("name"), body.Description, body.Permissions)
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
func (h *Handler) DeleteRole(c fiber.Ctx) error {
	return reply(c, fiber.StatusOK, "ROLE_ERROR", func() (Response, error) {
		err := h.authz.DeleteRole(c.Context(), actor(c), c.Params("name"))
		return map[string]bool{"ok": err == nil}, err
	})
}
