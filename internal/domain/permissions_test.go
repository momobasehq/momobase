package domain_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
)

// sdkPermissions is the TypeScript copy of the catalogue, relative to this package.
const sdkPermissions = "../../web/sdk/src/permissions.ts"

// codeLiteral matches one entry of the SDK's const objects, such as
// `  systemRead: "system:read",`.
var codeLiteral = regexp.MustCompile(`^\s+\w+:\s+"([^"]+)",`)

// parseSDK returns the codes the SDK declares, split by the const object holding them.
func parseSDK(t *testing.T) (admin, app []string) {
	t.Helper()
	source, err := os.ReadFile(filepath.Clean(sdkPermissions))
	if err != nil {
		t.Fatalf("read %s: %v", sdkPermissions, err)
	}
	target := &admin
	for line := range strings.Lines(string(source)) {
		switch {
		case strings.HasPrefix(line, "export const AdminPermissions"):
			target = &admin
		case strings.HasPrefix(line, "export const AppScopes"):
			target = &app
		case codeLiteral.MatchString(line):
			*target = append(*target, codeLiteral.FindStringSubmatch(line)[1])
		}
	}
	if len(admin) == 0 || len(app) == 0 {
		t.Fatalf("parsed %d admin and %d app codes from the SDK; its shape must have changed", len(admin), len(app))
	}
	return admin, app
}

// TestSDKPermissionsMatchTheCatalogue is what makes exporting the codes from the SDK
// safe rather than a liability.
//
// The SDK duplicates the catalogue so a client gets a compile error on a mistyped
// permission instead of a check that silently never matches. That duplication is only
// defensible if the two cannot drift, so this compares them both ways: a permission
// seeded but not exported leaves clients unable to gate on it, and one exported but no
// longer seeded invites a check that can never pass.
func TestSDKPermissionsMatchTheCatalogue(t *testing.T) {
	sdkAdmin, sdkApp := parseSDK(t)

	for _, audience := range []struct {
		name   string
		sdk    []string
		seeded []string
	}{
		{domain.AudienceAdmin, sdkAdmin, codesFor(domain.AudienceAdmin)},
		{domain.AudienceApp, sdkApp, codesFor(domain.AudienceApp)},
	} {
		t.Run(audience.name, func(t *testing.T) {
			for _, code := range audience.seeded {
				if !slices.Contains(audience.sdk, code) {
					t.Errorf("%s permission %q is seeded but missing from %s", audience.name, code, sdkPermissions)
				}
			}
			for _, code := range audience.sdk {
				if !slices.Contains(audience.seeded, code) {
					t.Errorf("%s exports %s permission %q, which the server does not seed", sdkPermissions, audience.name, code)
				}
			}
		})
	}
}

// codesFor returns the seeded codes for one audience.
func codesFor(audience string) []string {
	codes := make([]string, 0, len(domain.Permissions))
	for _, permission := range domain.Permissions {
		if permission.Audience == audience {
			codes = append(codes, permission.Code)
		}
	}
	return codes
}
