package oauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// Este test vive en el paquete oauth (no oauth_test) para poder inyectar un
// *http.Client con un Transport falso directamente en el campo no exportado
// httpClient — el adapter no expone forma de configurarlo desde afuera, y las
// URLs de Google están hardcodeadas en el código, así que la única forma de
// aislarlo de la red real es interceptar a nivel de RoundTripper.

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func newAdapterWithTransport(rt roundTripFunc) *GoogleAdapter {
	return &GoogleAdapter{
		clientID:     "client-id",
		clientSecret: "client-secret",
		redirectURI:  "https://app.test/callback",
		httpClient:   &http.Client{Transport: rt},
	}
}

// ─── NewGoogleAdapter ───────────────────────────────────────────────────────

func TestNewGoogleAdapter_GuardaConfiguracion(t *testing.T) {
	a := NewGoogleAdapter("cid", "csecret", "https://app.test/callback")

	if a.clientID != "cid" || a.clientSecret != "csecret" || a.redirectURI != "https://app.test/callback" {
		t.Errorf("NewGoogleAdapter: got clientID=%q clientSecret=%q redirectURI=%q", a.clientID, a.clientSecret, a.redirectURI)
	}
	if a.httpClient == nil {
		t.Fatal("NewGoogleAdapter: httpClient no debería ser nil")
	}
	if a.httpClient.Timeout <= 0 {
		t.Error("NewGoogleAdapter: httpClient debería tener un timeout configurado")
	}
}

// ─── VerifyGoogleCode ───────────────────────────────────────────────────────

func TestVerifyGoogleCode_HappyPath_RetornaSSOResultNormalizado(t *testing.T) {
	var tokenReq, userInfoReq *http.Request

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Hostname() {
		case "oauth2.googleapis.com":
			tokenReq = req
			return jsonResponse(http.StatusOK, `{"access_token":"tok-123"}`), nil
		case "www.googleapis.com":
			userInfoReq = req
			return jsonResponse(http.StatusOK, `{"sub":"google-uid-1","email":"user@gmail.com","name":"Juan Perez","picture":"https://pic.test/juan.jpg"}`), nil
		default:
			t.Fatalf("host inesperado: %s", req.URL.Host)
			return nil, nil
		}
	})
	a := newAdapterWithTransport(rt)

	result, err := a.VerifyGoogleCode(context.Background(), "auth-code-1")

	if err != nil {
		t.Fatalf("VerifyGoogleCode: error inesperado: %v", err)
	}
	if result.ProviderUserID != "google-uid-1" || result.Email != "user@gmail.com" ||
		result.Name != "Juan Perez" || result.AvatarURL != "https://pic.test/juan.jpg" {
		t.Errorf("VerifyGoogleCode: got %+v, want el perfil normalizado de Google", result)
	}

	// El intercambio de code debe viajar con los parámetros correctos y grant_type=authorization_code
	if tokenReq == nil {
		t.Fatal("no se llamó al endpoint de token")
	}
	if tokenReq.Method != http.MethodPost {
		t.Errorf("token request method: got %q, want %q", tokenReq.Method, http.MethodPost)
	}
	q := tokenReq.URL.Query()
	if q.Get("code") != "auth-code-1" {
		t.Errorf("token request code: got %q, want %q", q.Get("code"), "auth-code-1")
	}
	if q.Get("client_id") != "client-id" || q.Get("client_secret") != "client-secret" {
		t.Errorf("token request credentials: got client_id=%q client_secret=%q", q.Get("client_id"), q.Get("client_secret"))
	}
	if q.Get("redirect_uri") != "https://app.test/callback" {
		t.Errorf("token request redirect_uri: got %q, want %q", q.Get("redirect_uri"), "https://app.test/callback")
	}
	if q.Get("grant_type") != "authorization_code" {
		t.Errorf("token request grant_type: got %q, want %q", q.Get("grant_type"), "authorization_code")
	}

	// El userinfo debe pedirse con el access_token recién obtenido
	if userInfoReq == nil {
		t.Fatal("no se llamó al endpoint de userinfo")
	}
	if userInfoReq.Method != http.MethodGet {
		t.Errorf("userinfo request method: got %q, want %q", userInfoReq.Method, http.MethodGet)
	}
	if got := userInfoReq.URL.Query().Get("access_token"); got != "tok-123" {
		t.Errorf("userinfo request access_token: got %q, want %q", got, "tok-123")
	}
}

func TestVerifyGoogleCode_ErrorDeRedEnTokenEndpoint_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("connection refused")
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, boom })
	a := newAdapterWithTransport(rt)

	_, err := a.VerifyGoogleCode(context.Background(), "auth-code-1")

	if err == nil || !strings.Contains(err.Error(), "error de red al conectar con Google") {
		t.Fatalf("VerifyGoogleCode: got %v, want error de red envuelto", err)
	}
}

func TestVerifyGoogleCode_TokenEndpointRechazaCodigo_RetornaError(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, `{"error":"invalid_grant"}`), nil
	})
	a := newAdapterWithTransport(rt)

	_, err := a.VerifyGoogleCode(context.Background(), "code-expirado-o-reusado")

	if err == nil || !strings.Contains(err.Error(), "rechazó el código de autorización") || !strings.Contains(err.Error(), "400") {
		t.Fatalf("VerifyGoogleCode: got %v, want error de código rechazado con el status incluido", err)
	}
}

func TestVerifyGoogleCode_TokenEndpointJSONMalformado_RetornaError(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `esto no es json`), nil
	})
	a := newAdapterWithTransport(rt)

	_, err := a.VerifyGoogleCode(context.Background(), "auth-code-1")

	if err == nil {
		t.Fatal("VerifyGoogleCode: esperaba error por JSON malformado en la respuesta del token endpoint")
	}
}

func TestVerifyGoogleCode_ErrorDeRedEnUserInfoEndpoint_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de red")
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "oauth2.googleapis.com" {
			return jsonResponse(http.StatusOK, `{"access_token":"tok-123"}`), nil
		}
		return nil, boom
	})
	a := newAdapterWithTransport(rt)

	_, err := a.VerifyGoogleCode(context.Background(), "auth-code-1")

	// A diferencia del error de red del token endpoint, este NO se envuelve con fmt.Errorf.
	if !errors.Is(err, boom) {
		t.Fatalf("VerifyGoogleCode: got %v, want %v", err, boom)
	}
}

func TestVerifyGoogleCode_UserInfoJSONMalformado_RetornaError(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "oauth2.googleapis.com" {
			return jsonResponse(http.StatusOK, `{"access_token":"tok-123"}`), nil
		}
		return jsonResponse(http.StatusOK, `esto no es json`), nil
	})
	a := newAdapterWithTransport(rt)

	_, err := a.VerifyGoogleCode(context.Background(), "auth-code-1")

	if err == nil {
		t.Fatal("VerifyGoogleCode: esperaba error por JSON malformado en la respuesta de userinfo")
	}
}

func TestVerifyGoogleCode_UserInfoStatusDeErrorNoValidado_NoRetornaError(t *testing.T) {
	// Pin de comportamiento actual: el adapter NO valida el status code de la respuesta
	// de userinfo (a diferencia del token endpoint, que sí lo hace). Si Google devuelve
	// un 401 con un cuerpo JSON válido pero sin los campos esperados, el decode no falla
	// y el adapter devuelve un SSOResult vacío en vez de un error. Esto documenta el gap;
	// si algún día se agrega la validación de status, este test debería actualizarse.
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "oauth2.googleapis.com" {
			return jsonResponse(http.StatusOK, `{"access_token":"tok-expirado"}`), nil
		}
		return jsonResponse(http.StatusUnauthorized, `{"error":"invalid_token"}`), nil
	})
	a := newAdapterWithTransport(rt)

	result, err := a.VerifyGoogleCode(context.Background(), "auth-code-1")

	if err != nil {
		t.Fatalf("VerifyGoogleCode: got error %v, want nil (comportamiento actual no valida el status de userinfo)", err)
	}
	if result.ProviderUserID != "" || result.Email != "" {
		t.Errorf("VerifyGoogleCode: got %+v, want SSOResult vacío ante un userinfo 401 no validado", result)
	}
}

// ─── VerifyMetaToken ────────────────────────────────────────────────────────

func TestVerifyMetaToken_SiempreRetornaError_SinLlamarALaRed(t *testing.T) {
	called := false
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	a := newAdapterWithTransport(rt)

	_, err := a.VerifyMetaToken(context.Background(), "cualquier-token")

	if err == nil {
		t.Fatal("VerifyMetaToken: este adapter es solo de Google, esperaba error")
	}
	if called {
		t.Error("VerifyMetaToken: no debería haber disparado ninguna llamada HTTP")
	}
}
