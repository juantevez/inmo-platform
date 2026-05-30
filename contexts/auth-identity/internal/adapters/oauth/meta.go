package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"inmo.platform/contexts/auth-identity/internal/ports"
)

type MetaAdapter struct {
	httpClient *http.Client
}

func NewMetaAdapter() *MetaAdapter {
	return &MetaAdapter{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (a *MetaAdapter) VerifyMetaToken(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
	// 1. Petición directa a la Graph API solicitando explícitamente los campos que necesitamos
	// Nota: Pasamos el Token directo en la URL de forma segura bajo HTTPS
	graphURL := fmt.Sprintf("https://graph.facebook.com/me?fields=id,name,email,picture&access_token=%s", accessToken)

	req, err := http.NewRequestWithContext(ctx, "GET", graphURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error de red al conectar con Meta Graph API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Meta rechazó el token de acceso proporcionado (Status: %d)", resp.StatusCode)
	}

	// 2. Mapear la estructura nativa de Facebook
	var metaProfile struct {
		ID      string `json:"id"` // ID único de la App de Meta
		Name    string `json:"name"`
		Email   string `json:"email"` // 🚀 OJO: Puede venir ausente/vacío si se registraron con celular
		Picture struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		} `json:"picture"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&metaProfile); err != nil {
		return nil, err
	}

	// 3. Devolver los datos estandarizados al puerto
	return &ports.SSOResult{
		ProviderUserID: metaProfile.ID,
		Email:          metaProfile.Email, // Puede ir vacío "" disparando el flujo alternativo en el UC-05
		Name:           metaProfile.Name,
		AvatarURL:      metaProfile.Picture.Data.URL,
	}, nil
}

func (a *MetaAdapter) VerifyGoogleCode(ctx context.Context, code string) (*ports.SSOResult, error) {
	// Cumplimos con la interfaz, pero este adaptador solo maneja Meta
	return nil, fmt.Errorf("este adaptador solo procesa autenticación de Meta")
}
