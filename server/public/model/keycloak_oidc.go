// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import "strings"

const (
	ServiceKeycloakOIDC         = "keycloak_oidc"
	UserAuthServiceKeycloakOIDC = ServiceKeycloakOIDC
)

func AuthServiceDisplayName(service string) string {
	if service == ServiceKeycloakOIDC {
		return "Fratar OIDC"
	}

	return strings.Title(service)
}

func ShouldAliasOpenIdToKeycloakOIDC(config *Config) bool {
	if config == nil {
		return false
	}

	return SafeDereference(config.KeycloakOIDCSettings.Enable) && !SafeDereference(config.OpenIdSettings.Enable)
}
