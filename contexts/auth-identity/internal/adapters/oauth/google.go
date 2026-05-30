package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"inmo.platform/contexts/auth-identity/internal/ports"
)

type GoogleAdapter struct {
	clientID     string
	clientSecret string
	redirectURI  string
	httpClient   *http.Client
}

func NewGoogleAdapter(clientID, clientSecret, redirectURI string) *GoogleAdapter {
	return &GoogleAdapter{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		httpClient: &http.Client{
			Timeout: 5 * time.Second, // Timeout estricto para proteger nuestra API
		},
	}
}

func (a *GoogleAdapter) VerifyGoogleCode(ctx context.Context, code string) (*ports.SSOResult, error) {
	// 1. Preparar los parámetros para el intercambio del Authorization Code
	tokenURL := "https://oauth2.googleapis.com/token"
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", a.clientID)
	data.Set("client_secret", a.clientSecret)
	data.Set("redirect_uri", a.redirectURI)
	data.Set("grant_type", "authorization_code")

	// 2. Ejecutar la llamada POST segura contra Google
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, nil)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = data.Encode()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error de red al conectar con Google: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google rechazó el código de autorización (Status: %d)", resp.StatusCode)
	}

	// 3. Mapear la respuesta para extraer los tokens
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	// 4. Con el Access Token recuperado, llamamos al endpoint de UserInfo para traer los datos limpios
	userInfoURL := "https://www.googleapis.com/oauth2/v3/userinfo?access_token=" + tokenResp.AccessToken
	userReq, err := http.NewRequestWithContext(ctx, "GET", userInfoURL, nil)
	if err != nil {
		return nil, err
	}

	userResp, err := a.httpClient.Do(userReq)
	if err != nil {
		return nil, err
	}
	defer userResp.Body.Close()

	var profile struct {
		Sub     string `json:"sub"` // ID único e inmutable de Google
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&profile); err != nil {
		return nil, err
	}

	// 5. Normalizar la salida bajo el contrato estricto de nuestro Port
	return &ports.SSOResult{
		ProviderUserID: profile.Sub,
		Email:          profile.Email,
		Name:           profile.Name,
		AvatarURL:      profile.Picture,
	}, nil
}

func (a *GoogleAdapter) VerifyMetaToken(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
	// Cumplimos con la interfaz, pero este adaptador solo maneja Google
	return nil, fmt.Errorf("este adaptador solo procesa autenticación de Google")
}
