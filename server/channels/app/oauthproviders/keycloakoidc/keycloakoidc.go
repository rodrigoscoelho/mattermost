// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package oauthkeycloakoidc

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
)

type KeycloakOIDCProvider struct{}

type KeycloakOIDCUserInfo struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	PreferredUsername string `json:"preferred_username"`
}

func init() {
	einterfaces.RegisterOAuthProvider(model.ServiceKeycloakOIDC, &KeycloakOIDCProvider{})
}

func keycloakOIDCUserInfoFromJSON(data io.Reader) (*KeycloakOIDCUserInfo, error) {
	decoder := json.NewDecoder(data)

	var claims KeycloakOIDCUserInfo
	if err := decoder.Decode(&claims); err != nil {
		return nil, err
	}

	return &claims, nil
}

func (u *KeycloakOIDCUserInfo) IsValid(tokenUser *model.User) error {
	if strings.TrimSpace(u.Sub) == "" && (tokenUser == nil || tokenUser.AuthData == nil || strings.TrimSpace(*tokenUser.AuthData) == "") {
		return errors.New("user subject should not be empty")
	}

	return nil
}

func userFromKeycloakOIDCUser(logger mlog.LoggerIFace, claims *KeycloakOIDCUserInfo, tokenUser *model.User, settings *model.SSOSettings) *model.User {
	user := &model.User{AuthService: model.ServiceKeycloakOIDC}

	authData := strings.TrimSpace(claims.Sub)
	if tokenUser != nil && tokenUser.AuthData != nil && strings.TrimSpace(*tokenUser.AuthData) != "" {
		authData = strings.TrimSpace(*tokenUser.AuthData)
	}
	user.AuthData = &authData

	user.Email = strings.ToLower(strings.TrimSpace(claims.Email))
	if user.Email == "" && tokenUser != nil {
		user.Email = strings.ToLower(strings.TrimSpace(tokenUser.Email))
	}

	var username string
	if settings != nil && model.SafeDereference(settings.UsePreferredUsername) && claims.PreferredUsername != "" {
		username = strings.Split(claims.PreferredUsername, "@")[0]
	} else if claims.PreferredUsername != "" {
		username = strings.Split(claims.PreferredUsername, "@")[0]
	} else if user.Email != "" {
		username = strings.Split(user.Email, "@")[0]
	} else {
		username = authData
	}
	user.Username = model.CleanUsername(logger, username)

	user.FirstName = strings.TrimSpace(claims.GivenName)
	user.LastName = strings.TrimSpace(claims.FamilyName)
	if user.FirstName == "" && user.LastName == "" && strings.TrimSpace(claims.Name) != "" {
		splitName := strings.Fields(strings.TrimSpace(claims.Name))
		if len(splitName) == 1 {
			user.FirstName = splitName[0]
		} else if len(splitName) >= 2 {
			user.FirstName = splitName[0]
			user.LastName = strings.Join(splitName[1:], " ")
		}
	}

	if tokenUser != nil {
		if user.FirstName == "" {
			user.FirstName = tokenUser.FirstName
		}
		if user.LastName == "" {
			user.LastName = tokenUser.LastName
		}
	}

	return user
}

func (kp *KeycloakOIDCProvider) GetUserFromJSON(rctx request.CTX, data io.Reader, tokenUser *model.User, settings *model.SSOSettings) (*model.User, error) {
	claims, err := keycloakOIDCUserInfoFromJSON(data)
	if err != nil {
		return nil, err
	}

	if err = claims.IsValid(tokenUser); err != nil {
		return nil, err
	}

	return userFromKeycloakOIDCUser(rctx.Logger(), claims, tokenUser, settings), nil
}

func (kp *KeycloakOIDCProvider) GetSSOSettings(_ request.CTX, config *model.Config, _ string) (*model.SSOSettings, error) {
	return &config.KeycloakOIDCSettings, nil
}

func (kp *KeycloakOIDCProvider) GetUserFromIdToken(_ request.CTX, _ string) (*model.User, error) {
	return nil, nil
}

func (kp *KeycloakOIDCProvider) IsSameUser(_ request.CTX, dbUser, oauthUser *model.User) bool {
	return dbUser.AuthData != nil &&
		oauthUser.AuthData != nil &&
		*dbUser.AuthData == *oauthUser.AuthData
}
