// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/channels/utils"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
)

type keycloakOIDCProviderMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

type keycloakOIDCUserClaims struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	PreferredUsername string `json:"preferred_username"`
}

type keycloakOIDCSubject struct {
	Sub string `json:"sub"`
}

func (a *App) getKeycloakOIDCProviderMetadata(sso *model.SSOSettings) (*keycloakOIDCProviderMetadata, error) {
	discoveryEndpoint := strings.TrimSpace(model.SafeDereference(sso.DiscoveryEndpoint))
	if discoveryEndpoint == "" {
		return nil, fmt.Errorf("keycloak oidc discovery endpoint is required")
	}

	req, err := http.NewRequest(http.MethodGet, discoveryEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := a.HTTPService().MakeClient(true).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected keycloak discovery response status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var metadata keycloakOIDCProviderMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, err
	}

	switch {
	case strings.TrimSpace(metadata.Issuer) == "":
		return nil, fmt.Errorf("keycloak discovery metadata missing issuer")
	case strings.TrimSpace(metadata.AuthorizationEndpoint) == "":
		return nil, fmt.Errorf("keycloak discovery metadata missing authorization endpoint")
	case strings.TrimSpace(metadata.TokenEndpoint) == "":
		return nil, fmt.Errorf("keycloak discovery metadata missing token endpoint")
	case strings.TrimSpace(metadata.UserinfoEndpoint) == "":
		return nil, fmt.Errorf("keycloak discovery metadata missing userinfo endpoint")
	case strings.TrimSpace(metadata.JWKSURI) == "":
		return nil, fmt.Errorf("keycloak discovery metadata missing jwks uri")
	}

	return &metadata, nil
}

func (a *App) setOAuthNonceCookie(w http.ResponseWriter, r *http.Request, nonce string) {
	secure := GetProtocol(r) == "https"
	subpath, _ := utils.GetSubpathFromConfig(a.Config())
	expiresAt := time.Unix(model.GetMillis()/1000+int64(OAuthCookieMaxAgeSeconds), 0)

	http.SetCookie(w, &http.Cookie{
		Name:     CookieOAuthNonce,
		Value:    nonce,
		Path:     subpath,
		MaxAge:   OAuthCookieMaxAgeSeconds,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
	})
}

func (a *App) clearOAuthCookie(w http.ResponseWriter, name string) {
	subpath, _ := utils.GetSubpathFromConfig(a.Config())

	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     subpath,
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func (a *App) authorizeKeycloakOIDCUser(rctx request.CTX, w http.ResponseWriter, r *http.Request, provider einterfaces.OAuthProvider, sso *model.SSOSettings, code, redirectURI string) (io.ReadCloser, *model.User, *model.AppError) {
	metadata, err := a.getKeycloakOIDCProviderMetadata(sso)
	if err != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.service.app_error",
			map[string]any{"Service": model.AuthServiceDisplayName(model.ServiceKeycloakOIDC)}, "", http.StatusInternalServerError).Wrap(err)
	}

	p := url.Values{}
	p.Set("client_id", *sso.Id)
	p.Set("client_secret", *sso.Secret)
	p.Set("code", code)
	p.Set("grant_type", model.AccessTokenGrantType)
	p.Set("redirect_uri", redirectURI)

	req, requestErr := http.NewRequest(http.MethodPost, metadata.TokenEndpoint, strings.NewReader(p.Encode()))
	if requestErr != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.token_failed.app_error", nil, "", http.StatusInternalServerError).Wrap(requestErr)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := a.HTTPService().MakeClient(true).Do(req)
	if err != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.token_failed.app_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, &buf)
	var accessResponse *model.AccessResponse
	if err := json.NewDecoder(tee).Decode(&accessResponse); err != nil || resp.StatusCode != http.StatusOK {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.bad_response.app_error", nil,
			fmt.Sprintf("response_body=%s, status_code=%d, error=%v", buf.String(), resp.StatusCode, err), http.StatusInternalServerError).Wrap(err)
	}

	if strings.ToLower(accessResponse.TokenType) != model.AccessTokenType {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.bad_token.app_error", nil,
			"token_type="+accessResponse.TokenType+", response_body="+buf.String(), http.StatusInternalServerError)
	}

	if accessResponse.AccessToken == "" || accessResponse.IdToken == "" {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.missing.app_error", nil,
			"response_body="+buf.String(), http.StatusInternalServerError)
	}

	nonceCookie, cookieErr := r.Cookie(CookieOAuthNonce)
	if cookieErr != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.invalid_state.app_error", nil, "", http.StatusBadRequest).Wrap(cookieErr)
	}
	a.clearOAuthCookie(w, CookieOAuthNonce)

	oidcContext := oidc.ClientContext(context.Background(), a.HTTPService().MakeClient(true))
	keySet := oidc.NewRemoteKeySet(oidcContext, metadata.JWKSURI)
	verifier := oidc.NewVerifier(metadata.Issuer, keySet, &oidc.Config{ClientID: *sso.Id})

	idToken, err := verifier.Verify(oidcContext, accessResponse.IdToken)
	if err != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.bad_token.app_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	if idToken.Nonce != nonceCookie.Value {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.invalid_state.app_error", nil, "invalid oidc nonce", http.StatusBadRequest)
	}

	if idToken.AccessTokenHash != "" {
		if err := idToken.VerifyAccessToken(accessResponse.AccessToken); err != nil {
			return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.bad_token.app_error", nil, "", http.StatusInternalServerError).Wrap(err)
		}
	}

	var claims keycloakOIDCUserClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.bad_token.app_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	claims.Sub = strings.TrimSpace(claims.Sub)
	claims.Email = strings.ToLower(strings.TrimSpace(claims.Email))
	if claims.Sub == "" {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.bad_token.app_error", nil, "missing sub claim", http.StatusInternalServerError)
	}

	tokenPayload, err := json.Marshal(claims)
	if err != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.bad_token.app_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	tokenUser, err := provider.GetUserFromJSON(rctx, bytes.NewReader(tokenPayload), nil, sso)
	if err != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.token_failed.app_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	req, requestErr = http.NewRequest(http.MethodGet, metadata.UserinfoEndpoint, nil)
	if requestErr != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.service.app_error",
			map[string]any{"Service": model.AuthServiceDisplayName(model.ServiceKeycloakOIDC)}, "", http.StatusInternalServerError).Wrap(requestErr)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessResponse.AccessToken)

	resp, err = a.HTTPService().MakeClient(true).Do(req)
	if err != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.service.app_error",
			map[string]any{"Service": model.AuthServiceDisplayName(model.ServiceKeycloakOIDC)}, "", http.StatusInternalServerError).Wrap(err)
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.response.app_error", nil, "", http.StatusInternalServerError).Wrap(readErr)
	}

	if resp.StatusCode != http.StatusOK {
		bodyString := string(bodyBytes)
		rctx.Logger().Error("Error getting Keycloak OIDC user", mlog.Int("response", resp.StatusCode), mlog.String("body_string", bodyString))
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.response.app_error", nil, "response_body="+bodyString, http.StatusInternalServerError)
	}

	var subject keycloakOIDCSubject
	if err := json.Unmarshal(bodyBytes, &subject); err != nil {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.response.app_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	if strings.TrimSpace(subject.Sub) == "" || subject.Sub != idToken.Subject {
		return nil, nil, model.NewAppError("authorizeKeycloakOIDCUser", "api.user.authorize_oauth_user.bad_token.app_error", nil,
			fmt.Sprintf("userinfo_sub=%s id_token_sub=%s", subject.Sub, idToken.Subject), http.StatusInternalServerError)
	}

	return io.NopCloser(bytes.NewReader(bodyBytes)), tokenUser, nil
}
