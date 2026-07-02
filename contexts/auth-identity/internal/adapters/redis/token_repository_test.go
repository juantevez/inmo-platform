package redis_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	redisadapter "inmo.platform/contexts/auth-identity/internal/adapters/redis"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// Usamos miniredis (un servidor Redis in-memory) en vez de mockear el cliente
// call-por-call: RedisTokenRepository se apoya en semántica real de Redis
// (TTLs, SCAN con cursores, pipelines atómicos) que sería frágil y poco fiel
// de simular con expectativas manuales — miniredis ejecuta esos comandos de
// verdad, así que el test cubre el comportamiento real del adapter.

func newTestRepo(t *testing.T) (*redisadapter.RedisTokenRepository, *miniredis.Miniredis, *goredis.Client) {
	t.Helper()
	mr := miniredis.RunT(t) // RunT registra el Close() automático al terminar el test
	// MaxRetries: -1 desactiva los reintentos automáticos del cliente — los tests
	// que fuerzan errores de conexión (mr.Close()) no necesitan esperar los backoffs.
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { client.Close() })

	return redisadapter.NewRedisTokenRepository(client), mr, client
}

// ─── SetRefreshToken ────────────────────────────────────────────────────────

func TestSetRefreshToken_HappyPath_GuardaConTTL(t *testing.T) {
	repo, mr, _ := newTestRepo(t)

	err := repo.SetRefreshToken(context.Background(), "tok-1", "user-1", 7*24*time.Hour)

	if err != nil {
		t.Fatalf("SetRefreshToken: error inesperado: %v", err)
	}
	got, err := mr.Get("refresh_token:tok-1")
	if err != nil {
		t.Fatalf("mr.Get: %v", err)
	}
	if got != "user-1" {
		t.Errorf("valor guardado: got %q, want %q", got, "user-1")
	}
	ttl := mr.TTL("refresh_token:tok-1")
	if ttl <= 0 {
		t.Errorf("TTL: got %v, want > 0", ttl)
	}
}

func TestSetRefreshToken_ErrorDeConexion_RetornaErrorEnvuelto(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	mr.Close() // fuerza que cualquier comando falle a nivel de red

	err := repo.SetRefreshToken(context.Background(), "tok-1", "user-1", time.Hour)

	if err == nil || !strings.Contains(err.Error(), "error al guardar refresh token en Redis") {
		t.Fatalf("SetRefreshToken: got %v, want error de Redis envuelto", err)
	}
}

// ─── GetRefreshToken ────────────────────────────────────────────────────────

func TestGetRefreshToken_HappyPath(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	if err := mr.Set("refresh_token:tok-1", "user-42"); err != nil {
		t.Fatalf("mr.Set: %v", err)
	}

	userID, err := repo.GetRefreshToken(context.Background(), "tok-1")

	if err != nil {
		t.Fatalf("GetRefreshToken: error inesperado: %v", err)
	}
	if userID != "user-42" {
		t.Errorf("GetRefreshToken: got %q, want %q", userID, "user-42")
	}
}

func TestGetRefreshToken_NoExiste_RetornaErrorEnvolviendoRedisNil(t *testing.T) {
	repo, _, _ := newTestRepo(t)

	_, err := repo.GetRefreshToken(context.Background(), "no-existe")

	if err == nil || !errors.Is(err, goredis.Nil) {
		t.Fatalf("GetRefreshToken: got %v, want error que envuelva redis.Nil", err)
	}
	if !strings.Contains(err.Error(), "token inexistente o expirado") {
		t.Errorf("GetRefreshToken: mensaje got %q, want que contenga %q", err.Error(), "token inexistente o expirado")
	}
}

func TestGetRefreshToken_ErrorDeConexion_SePropagaSinEnvolver(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	mr.Close()

	_, err := repo.GetRefreshToken(context.Background(), "tok-1")

	// A diferencia del caso redis.Nil, un error de red genérico NO lleva el mensaje custom.
	if err == nil || strings.Contains(err.Error(), "token inexistente o expirado") {
		t.Fatalf("GetRefreshToken: got %v, want un error de red crudo sin el mensaje custom", err)
	}
}

// ─── DeleteRefreshToken ─────────────────────────────────────────────────────

func TestDeleteRefreshToken_HappyPath_BorraLaKey(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	if err := mr.Set("refresh_token:tok-1", "user-1"); err != nil {
		t.Fatalf("mr.Set: %v", err)
	}

	if err := repo.DeleteRefreshToken(context.Background(), "tok-1"); err != nil {
		t.Fatalf("DeleteRefreshToken: error inesperado: %v", err)
	}
	if mr.Exists("refresh_token:tok-1") {
		t.Error("DeleteRefreshToken: la key debería haber sido eliminada")
	}
}

func TestDeleteRefreshToken_KeyInexistente_NoRetornaError(t *testing.T) {
	repo, _, _ := newTestRepo(t)

	if err := repo.DeleteRefreshToken(context.Background(), "no-existe"); err != nil {
		t.Fatalf("DeleteRefreshToken: DEL sobre key inexistente no debería fallar: %v", err)
	}
}

// ─── DeleteAllRefreshTokens ─────────────────────────────────────────────────

func TestDeleteAllRefreshTokens_BorraSoloLasSesionesDelUsuarioIndicado(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	mustSet := func(key, val string) {
		t.Helper()
		if err := mr.Set(key, val); err != nil {
			t.Fatalf("mr.Set(%q): %v", key, err)
		}
	}
	mustSet("refresh_token:tok-a1", "user-1")
	mustSet("refresh_token:tok-a2", "user-1")
	mustSet("refresh_token:tok-b1", "user-2")
	mustSet("otra_clave:no_tocar", "user-1") // no matchea el prefijo refresh_token:*

	if err := repo.DeleteAllRefreshTokens(context.Background(), "user-1"); err != nil {
		t.Fatalf("DeleteAllRefreshTokens: error inesperado: %v", err)
	}

	if mr.Exists("refresh_token:tok-a1") || mr.Exists("refresh_token:tok-a2") {
		t.Error("DeleteAllRefreshTokens: las sesiones de user-1 deberían haberse borrado")
	}
	if !mr.Exists("refresh_token:tok-b1") {
		t.Error("DeleteAllRefreshTokens: la sesión de user-2 NO debería tocarse")
	}
	if !mr.Exists("otra_clave:no_tocar") {
		t.Error("DeleteAllRefreshTokens: claves fuera del prefijo refresh_token:* no deberían tocarse")
	}
}

func TestDeleteAllRefreshTokens_RecorreMultiplesBatchesDeSCAN(t *testing.T) {
	// El batch de SCAN es de 100 — sembramos más para forzar que el cursor haga
	// más de una vuelta y confirmar que el loop no corta antes de tiempo.
	repo, mr, _ := newTestRepo(t)
	const totalKeys = 250
	for i := 0; i < totalKeys; i++ {
		key := fmt.Sprintf("refresh_token:tok-%d", i)
		if err := mr.Set(key, "user-target"); err != nil {
			t.Fatalf("mr.Set(%q): %v", key, err)
		}
	}

	if err := repo.DeleteAllRefreshTokens(context.Background(), "user-target"); err != nil {
		t.Fatalf("DeleteAllRefreshTokens: error inesperado: %v", err)
	}

	for i := 0; i < totalKeys; i++ {
		key := "refresh_token:tok-" + strconv.Itoa(i)
		if mr.Exists(key) {
			t.Fatalf("DeleteAllRefreshTokens: %q debería haberse borrado (loop de SCAN incompleto)", key)
		}
	}
}

func TestDeleteAllRefreshTokens_ErrorDeConexion_RetornaErrorEnvuelto(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	mr.Close()

	err := repo.DeleteAllRefreshTokens(context.Background(), "user-1")

	if err == nil || !strings.Contains(err.Error(), "error al escanear sesiones en Redis") {
		t.Fatalf("DeleteAllRefreshTokens: got %v, want error de scan envuelto", err)
	}
}

// ─── AddToBlocklist / IsInBlocklist ─────────────────────────────────────────

func TestAddToBlocklist_HappyPath(t *testing.T) {
	repo, mr, _ := newTestRepo(t)

	err := repo.AddToBlocklist(context.Background(), "jwt-abc", time.Hour)

	if err != nil {
		t.Fatalf("AddToBlocklist: error inesperado: %v", err)
	}
	if !mr.Exists("blocklist:jwt-abc") {
		t.Error("AddToBlocklist: la key debería existir en Redis")
	}
}

func TestIsInBlocklist_TokenBlacklisteado_RetornaTrue(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	if err := mr.Set("blocklist:jwt-abc", "revoked"); err != nil {
		t.Fatalf("mr.Set: %v", err)
	}

	got, err := repo.IsInBlocklist(context.Background(), "jwt-abc")

	if err != nil {
		t.Fatalf("IsInBlocklist: error inesperado: %v", err)
	}
	if !got {
		t.Error("IsInBlocklist: got false, want true")
	}
}

func TestIsInBlocklist_TokenNoBlacklisteado_RetornaFalse(t *testing.T) {
	repo, _, _ := newTestRepo(t)

	got, err := repo.IsInBlocklist(context.Background(), "jwt-limpio")

	if err != nil {
		t.Fatalf("IsInBlocklist: error inesperado: %v", err)
	}
	if got {
		t.Error("IsInBlocklist: got true, want false")
	}
}

func TestIsInBlocklist_ErrorDeConexion_RetornaError(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	mr.Close()

	_, err := repo.IsInBlocklist(context.Background(), "jwt-abc")

	if err == nil {
		t.Fatal("IsInBlocklist: esperaba un error de conexión")
	}
}

// ─── IncrementLoginAttempts ─────────────────────────────────────────────────

func TestIncrementLoginAttempts_HappyPath_CuentaYAplicaTTL(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	key := "login_limit:1.2.3.4:user@test.com"

	first, err := repo.IncrementLoginAttempts(context.Background(), key, 15*time.Minute)
	if err != nil {
		t.Fatalf("IncrementLoginAttempts (1ra vez): error inesperado: %v", err)
	}
	if first != 1 {
		t.Errorf("primer intento: got %d, want 1", first)
	}

	second, err := repo.IncrementLoginAttempts(context.Background(), key, 15*time.Minute)
	if err != nil {
		t.Fatalf("IncrementLoginAttempts (2da vez): error inesperado: %v", err)
	}
	if second != 2 {
		t.Errorf("segundo intento: got %d, want 2", second)
	}

	ttl := mr.TTL(key)
	if ttl <= 0 {
		t.Errorf("TTL tras incrementar: got %v, want > 0", ttl)
	}
}

func TestIncrementLoginAttempts_ErrorDeConexion_RetornaErrorEnvuelto(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	mr.Close()

	_, err := repo.IncrementLoginAttempts(context.Background(), "login_limit:x:y", time.Minute)

	if err == nil || !strings.Contains(err.Error(), "falló la ejecución atómica del rate limit") {
		t.Fatalf("IncrementLoginAttempts: got %v, want error de pipeline envuelto", err)
	}
}

// ─── ClearLoginAttempts ─────────────────────────────────────────────────────

func TestClearLoginAttempts_HappyPath_BorraElContador(t *testing.T) {
	repo, mr, _ := newTestRepo(t)
	key := "login_limit:1.2.3.4:user@test.com"
	if err := mr.Set(key, "3"); err != nil {
		t.Fatalf("mr.Set: %v", err)
	}

	if err := repo.ClearLoginAttempts(context.Background(), key); err != nil {
		t.Fatalf("ClearLoginAttempts: error inesperado: %v", err)
	}
	if mr.Exists(key) {
		t.Error("ClearLoginAttempts: la key debería haberse eliminado")
	}
}

func TestClearLoginAttempts_KeyInexistente_NoRetornaError(t *testing.T) {
	repo, _, _ := newTestRepo(t)

	if err := repo.ClearLoginAttempts(context.Background(), "no-existe"); err != nil {
		t.Fatalf("ClearLoginAttempts: DEL sobre key inexistente no debería fallar: %v", err)
	}
}
