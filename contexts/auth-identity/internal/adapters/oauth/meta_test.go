package oauth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// Reutiliza roundTripFunc y jsonResponse definidos en google_test.go (mismo
// paquete oauth) — la técnica es la misma: interceptar a nivel de Transport
// porque la URL de Meta está hardcodeada y httpClient no es inyectable desde
// afuera del paquete.

func newMetaAdapterWithTransport(rt roundTripFunc) *MetaAdapter {
	return &MetaAdapter{
		httpClient: &http.Client{Transport: rt},
	}
}

// ─── NewMetaAdapter ─────────────────────────────────────────────────────────

func TestNewMetaAdapter_ConfiguraHTTPClientConTimeout(t *testing.T) {
	a := NewMetaAdapter()

	if a.httpClient == nil {
		t.Fatal("NewMetaAdapter: httpClient no debería ser nil")
	}
	if a.httpClient.Timeout <= 0 {
		t.Error("NewMetaAdapter: httpClient debería tener un timeout configurado")
	}
}

// ─── VerifyMetaToken ────────────────────────────────────────────────────────

func TestVerifyMetaToken_HappyPath_RetornaSSOResultNormalizado(t *testing.T) {
	var capturedReq *http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return jsonResponse(http.StatusOK, `{
			"id": "meta-uid-1",
			"name": "Juan Perez",
			"email": "user@fb.com",
			"picture": {"data": {"url": "https://pic.test/juan.jpg"}}
		}`), nil
	})
	a := newMetaAdapterWithTransport(rt)

	result, err := a.VerifyMetaToken(context.Background(), "access-token-1")

	if err != nil {
		t.Fatalf("VerifyMetaToken: error inesperado: %v", err)
	}
	if result.ProviderUserID != "meta-uid-1" || result.Email != "user@fb.com" ||
		result.Name != "Juan Perez" || result.AvatarURL != "https://pic.test/juan.jpg" {
		t.Errorf("VerifyMetaToken: got %+v, want el perfil normalizado de Meta", result)
	}

	if capturedReq == nil {
		t.Fatal("no se llamó a la Graph API")
	}
	if capturedReq.Method != http.MethodGet {
		t.Errorf("request method: got %q, want %q", capturedReq.Method, http.MethodGet)
	}
	if capturedReq.URL.Hostname() != "graph.facebook.com" {
		t.Errorf("request host: got %q, want %q", capturedReq.URL.Hostname(), "graph.facebook.com")
	}
	q := capturedReq.URL.Query()
	if q.Get("access_token") != "access-token-1" {
		t.Errorf("access_token: got %q, want %q", q.Get("access_token"), "access-token-1")
	}
	if q.Get("fields") != "id,name,email,picture" {
		t.Errorf("fields: got %q, want %q", q.Get("fields"), "id,name,email,picture")
	}
}

func TestVerifyMetaToken_EmailAusente_RetornaSSOResultConEmailVacio(t *testing.T) {
	// Documentado explícitamente en el código: Meta puede omitir el email si el usuario
	// se registró solo con celular. El adapter debe dejarlo vacío, no fallar.
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"id":"meta-uid-1","name":"Juan Perez","picture":{"data":{"url":"https://pic.test/juan.jpg"}}}`), nil
	})
	a := newMetaAdapterWithTransport(rt)

	result, err := a.VerifyMetaToken(context.Background(), "access-token-1")

	if err != nil {
		t.Fatalf("VerifyMetaToken: error inesperado: %v", err)
	}
	if result.Email != "" {
		t.Errorf("VerifyMetaToken Email: got %q, want vacío", result.Email)
	}
	if result.ProviderUserID != "meta-uid-1" {
		t.Errorf("VerifyMetaToken ProviderUserID: got %q, want %q", result.ProviderUserID, "meta-uid-1")
	}
}

func TestVerifyMetaToken_ErrorDeRed_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("connection refused")
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, boom })
	a := newMetaAdapterWithTransport(rt)

	_, err := a.VerifyMetaToken(context.Background(), "access-token-1")

	if err == nil || !strings.Contains(err.Error(), "error de red al conectar con Meta Graph API") {
		t.Fatalf("VerifyMetaToken: got %v, want error de red envuelto", err)
	}
}

func TestVerifyMetaToken_TokenRechazado_RetornaError(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, `{"error":{"message":"Invalid OAuth access token"}}`), nil
	})
	a := newMetaAdapterWithTransport(rt)

	_, err := a.VerifyMetaToken(context.Background(), "token-vencido")

	if err == nil || !strings.Contains(err.Error(), "rechazó el token de acceso") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("VerifyMetaToken: got %v, want error de token rechazado con el status incluido", err)
	}
}

func TestVerifyMetaToken_JSONMalformado_RetornaError(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `esto no es json`), nil
	})
	a := newMetaAdapterWithTransport(rt)

	_, err := a.VerifyMetaToken(context.Background(), "access-token-1")

	if err == nil {
		t.Fatal("VerifyMetaToken: esperaba error por JSON malformado en la respuesta de la Graph API")
	}
}

// ─── VerifyGoogleCode ───────────────────────────────────────────────────────

func TestMetaAdapter_VerifyGoogleCode_SiempreRetornaError_SinLlamarALaRed(t *testing.T) {
	called := false
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	a := newMetaAdapterWithTransport(rt)

	_, err := a.VerifyGoogleCode(context.Background(), "cualquier-code")

	if err == nil {
		t.Fatal("VerifyGoogleCode: este adapter es solo de Meta, esperaba error")
	}
	if called {
		t.Error("VerifyGoogleCode: no debería haber disparado ninguna llamada HTTP")
	}
}
