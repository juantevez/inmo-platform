package domain_test

import (
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func mustPrice(t *testing.T, amount float64, currency domain.Currency) domain.Price {
	t.Helper()
	p, err := domain.NewPrice(amount, currency)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}
	return p
}

func mustLocation(t *testing.T, lat, lng float64, address string) domain.Location {
	t.Helper()
	l, err := domain.NewLocation(lat, lng, address)
	if err != nil {
		t.Fatalf("NewLocation: %v", err)
	}
	return l
}

func newSaleProperty(t *testing.T) *domain.Property {
	t.Helper()
	p, err := domain.NewProperty("prop-1", "owner-1", "Depto", "desc",
		mustPrice(t, 1000, domain.USD), mustLocation(t, -34.6, -58.4, "Calle Falsa 123"),
		domain.OperationSale, domain.PetPolicyNotAllowed)
	if err != nil {
		t.Fatalf("NewProperty: %v", err)
	}
	return p
}

func newTempProperty(t *testing.T) *domain.Property {
	t.Helper()
	p, err := domain.NewProperty("prop-1", "owner-1", "Depto temp", "desc",
		mustPrice(t, 100, domain.USD), mustLocation(t, -34.6, -58.4, "Calle Falsa 123"),
		domain.OperationTemp, domain.PetPolicyAllowed)
	if err != nil {
		t.Fatalf("NewProperty: %v", err)
	}
	return p
}

func assertPreconditionFailed(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("got %v, want AppError PreconditionFailed", err)
	}
}

func assertBadRequest(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

// ─── NewProperty ────────────────────────────────────────────────────────────

func TestNewProperty_IDVacio_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewProperty("", "owner-1", "Depto", "desc", mustPrice(t, 1000, domain.USD), mustLocation(t, 0, 0, "x"), domain.OperationSale, domain.PetPolicyAllowed)
	assertBadRequest(t, err)
}

func TestNewProperty_OwnerIDVacio_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewProperty("prop-1", "", "Depto", "desc", mustPrice(t, 1000, domain.USD), mustLocation(t, 0, 0, "x"), domain.OperationSale, domain.PetPolicyAllowed)
	assertBadRequest(t, err)
}

func TestNewProperty_TituloVacio_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewProperty("prop-1", "owner-1", "", "desc", mustPrice(t, 1000, domain.USD), mustLocation(t, 0, 0, "x"), domain.OperationSale, domain.PetPolicyAllowed)
	assertBadRequest(t, err)
}

func TestNewProperty_OperationTypeInvalido_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewProperty("prop-1", "owner-1", "Depto", "desc", mustPrice(t, 1000, domain.USD), mustLocation(t, 0, 0, "x"), domain.OperationType("LEASE"), domain.PetPolicyAllowed)
	assertBadRequest(t, err)
}

func TestNewProperty_PetPolicyInvalida_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewProperty("prop-1", "owner-1", "Depto", "desc", mustPrice(t, 1000, domain.USD), mustLocation(t, 0, 0, "x"), domain.OperationSale, domain.PetPolicy("SOLO_GATOS"))
	assertBadRequest(t, err)
}

func TestNewProperty_HappyPath_NaceAvailableConTempConfigPorDefecto(t *testing.T) {
	prop := newSaleProperty(t)

	if prop.State() != domain.StateAvailable {
		t.Errorf("State: got %q, want %q", prop.State(), domain.StateAvailable)
	}
	if prop.ID() != "prop-1" || prop.OwnerID() != "owner-1" || prop.Title() != "Depto" {
		t.Errorf("got ID=%q OwnerID=%q Title=%q", prop.ID(), prop.OwnerID(), prop.Title())
	}
	tc := prop.TempConfig()
	if tc.MinNights() != 1 || tc.MaxNights() != 90 || tc.CheckInTime() != "14:00" || tc.CheckOutTime() != "10:00" {
		t.Errorf("TempConfig default: got %+v", tc)
	}
}

// ─── Reserve ────────────────────────────────────────────────────────────────

func TestReserve_DesdeAvailable_Exito(t *testing.T) {
	prop := newSaleProperty(t)

	if err := prop.Reserve(); err != nil {
		t.Fatalf("Reserve: error inesperado: %v", err)
	}
	if prop.State() != domain.StateReserved {
		t.Errorf("State: got %q, want %q", prop.State(), domain.StateReserved)
	}
	events := prop.PullEvents()
	if len(events) != 1 {
		t.Fatalf("eventos: got %d, want 1", len(events))
	}
	evt, ok := events[0].(domain.PropertyStateChanged)
	if !ok || evt.OldState != domain.StateAvailable || evt.NewState != domain.StateReserved {
		t.Errorf("evento: got %+v", events[0])
	}
}

func TestReserve_DesdeEstadosNoDisponibles_RetornaPreconditionFailed(t *testing.T) {
	setups := map[string]func(*domain.Property){
		"RESERVED":     func(p *domain.Property) { _ = p.Reserve() },
		"CLOSED":       func(p *domain.Property) { _ = p.Close() },
		"UNDER_REPAIR": func(p *domain.Property) { _ = p.PutUnderRepair() },
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			prop := newSaleProperty(t)
			setup(prop)
			prop.PullEvents() // drena eventos del setup

			err := prop.Reserve()
			assertPreconditionFailed(t, err)
			if len(prop.PullEvents()) != 0 {
				t.Error("Reserve fallido no debería registrar eventos")
			}
		})
	}
}

// ─── Close ──────────────────────────────────────────────────────────────────

func TestClose_DesdeAvailable_Exito(t *testing.T) {
	prop := newSaleProperty(t)
	if err := prop.Close(); err != nil {
		t.Fatalf("Close: error inesperado: %v", err)
	}
	if prop.State() != domain.StateClosed {
		t.Errorf("State: got %q, want %q", prop.State(), domain.StateClosed)
	}
}

func TestClose_DesdeReserved_Exito(t *testing.T) {
	prop := newSaleProperty(t)
	if err := prop.Reserve(); err != nil {
		t.Fatalf("setup Reserve: %v", err)
	}
	if err := prop.Close(); err != nil {
		t.Fatalf("Close: error inesperado: %v", err)
	}
	if prop.State() != domain.StateClosed {
		t.Errorf("State: got %q, want %q", prop.State(), domain.StateClosed)
	}
}

func TestClose_DesdeEstadosYaInactivos_RetornaPreconditionFailed(t *testing.T) {
	setups := map[string]func(*domain.Property){
		"CLOSED":       func(p *domain.Property) { _ = p.Close() },
		"UNDER_REPAIR": func(p *domain.Property) { _ = p.PutUnderRepair() },
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			prop := newSaleProperty(t)
			setup(prop)
			err := prop.Close()
			assertPreconditionFailed(t, err)
		})
	}
}

// ─── PutUnderRepair ─────────────────────────────────────────────────────────

func TestPutUnderRepair_DesdeAvailable_Exito(t *testing.T) {
	prop := newSaleProperty(t)
	if err := prop.PutUnderRepair(); err != nil {
		t.Fatalf("PutUnderRepair: error inesperado: %v", err)
	}
	if prop.State() != domain.StateUnderRepair {
		t.Errorf("State: got %q, want %q", prop.State(), domain.StateUnderRepair)
	}
	if len(prop.PullEvents()) != 1 {
		t.Error("PutUnderRepair exitoso debería registrar un evento")
	}
}

func TestPutUnderRepair_Idempotente_NoRegistraEvento(t *testing.T) {
	prop := newSaleProperty(t)
	if err := prop.PutUnderRepair(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	prop.PullEvents() // drena el evento de la primera transición

	if err := prop.PutUnderRepair(); err != nil {
		t.Fatalf("PutUnderRepair (repetido): got %v, want nil (idempotente)", err)
	}
	if len(prop.PullEvents()) != 0 {
		t.Error("una segunda llamada idempotente no debería registrar un nuevo evento")
	}
}

// ─── ReleaseRepair ──────────────────────────────────────────────────────────

func TestReleaseRepair_DesdeUnderRepair_Exito(t *testing.T) {
	prop := newSaleProperty(t)
	if err := prop.PutUnderRepair(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := prop.ReleaseRepair(); err != nil {
		t.Fatalf("ReleaseRepair: error inesperado: %v", err)
	}
	if prop.State() != domain.StateAvailable {
		t.Errorf("State: got %q, want %q", prop.State(), domain.StateAvailable)
	}
}

func TestReleaseRepair_DesdeOtrosEstados_RetornaPreconditionFailed(t *testing.T) {
	setups := map[string]func(*domain.Property){
		"AVAILABLE": func(p *domain.Property) {},
		"RESERVED":  func(p *domain.Property) { _ = p.Reserve() },
		"CLOSED":    func(p *domain.Property) { _ = p.Close() },
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			prop := newSaleProperty(t)
			setup(prop)
			err := prop.ReleaseRepair()
			assertPreconditionFailed(t, err)
		})
	}
}

// ─── UpdateDetails ──────────────────────────────────────────────────────────

func TestUpdateDetails_DesdeAvailable_Exito(t *testing.T) {
	prop := newSaleProperty(t)
	newPrice := mustPrice(t, 2000, domain.USD)

	err := prop.UpdateDetails("Nuevo título", "Nueva desc", newPrice)

	if err != nil {
		t.Fatalf("UpdateDetails: error inesperado: %v", err)
	}
	if prop.Title() != "Nuevo título" || prop.Description() != "Nueva desc" || prop.Price().Amount() != 2000 {
		t.Errorf("got Title=%q Description=%q Price=%v", prop.Title(), prop.Description(), prop.Price())
	}
	events := prop.PullEvents()
	if len(events) != 1 {
		t.Fatalf("eventos: got %d, want 1", len(events))
	}
	evt, ok := events[0].(domain.PropertyDetailsUpdated)
	if !ok || evt.OldTitle != "Depto" || evt.NewTitle != "Nuevo título" || evt.OldPrice != 1000 || evt.NewPrice != 2000 {
		t.Errorf("evento: got %+v", events[0])
	}
}

func TestUpdateDetails_DesdeUnderRepair_Exito(t *testing.T) {
	prop := newSaleProperty(t)
	if err := prop.PutUnderRepair(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := prop.UpdateDetails("Nuevo título", "desc", mustPrice(t, 1000, domain.USD)); err != nil {
		t.Fatalf("UpdateDetails: error inesperado (UNDER_REPAIR debería permitir edición): %v", err)
	}
}

func TestUpdateDetails_DesdeReservedOClosed_RetornaPreconditionFailed(t *testing.T) {
	setups := map[string]func(*domain.Property){
		"RESERVED": func(p *domain.Property) { _ = p.Reserve() },
		"CLOSED":   func(p *domain.Property) { _ = p.Close() },
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			prop := newSaleProperty(t)
			setup(prop)
			err := prop.UpdateDetails("Nuevo título", "desc", mustPrice(t, 1000, domain.USD))
			assertPreconditionFailed(t, err)
		})
	}
}

func TestUpdateDetails_TituloVacio_RetornaBadRequest(t *testing.T) {
	prop := newSaleProperty(t)
	err := prop.UpdateDetails("", "desc", mustPrice(t, 1000, domain.USD))
	assertBadRequest(t, err)
}

func TestUpdateDetails_EstadoInvalidoTienePrioridadSobreTituloVacio(t *testing.T) {
	// A diferencia de UpdatePetPolicy, acá el chequeo de estado va PRIMERO en el código —
	// una propiedad reservada con título vacío falla por estado, no por el título.
	prop := newSaleProperty(t)
	if err := prop.Reserve(); err != nil {
		t.Fatalf("setup Reserve: %v", err)
	}

	err := prop.UpdateDetails("", "desc", mustPrice(t, 1000, domain.USD))
	assertPreconditionFailed(t, err)
}

// ─── UpdateLocation ─────────────────────────────────────────────────────────

func TestUpdateLocation_DesdeAvailable_Exito(t *testing.T) {
	prop := newSaleProperty(t)

	err := prop.UpdateLocation(-31.4, -64.2, "Nueva Dirección 456")

	if err != nil {
		t.Fatalf("UpdateLocation: error inesperado: %v", err)
	}
	if prop.Location().Address() != "Nueva Dirección 456" {
		t.Errorf("Address: got %q", prop.Location().Address())
	}
	events := prop.PullEvents()
	if len(events) != 1 {
		t.Fatalf("eventos: got %d, want 1", len(events))
	}
	if _, ok := events[0].(domain.PropertyLocationUpdated); !ok {
		t.Errorf("evento: got %T, want PropertyLocationUpdated", events[0])
	}
}

func TestUpdateLocation_DesdeReservedOClosed_RetornaPreconditionFailed(t *testing.T) {
	prop := newSaleProperty(t)
	if err := prop.Reserve(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := prop.UpdateLocation(-31.4, -64.2, "Nueva Dirección")
	assertPreconditionFailed(t, err)
}

func TestUpdateLocation_CoordenadasInvalidas_RetornaErrorDeDominio(t *testing.T) {
	prop := newSaleProperty(t)
	err := prop.UpdateLocation(999, 0, "Nueva Dirección")
	assertBadRequest(t, err)
}

// ─── UpdatePetPolicy ────────────────────────────────────────────────────────

func TestUpdatePetPolicy_DesdeAvailable_Exito(t *testing.T) {
	prop := newSaleProperty(t) // NOT_ALLOWED
	err := prop.UpdatePetPolicy(domain.PetPolicyAllowed)

	if err != nil {
		t.Fatalf("UpdatePetPolicy: error inesperado: %v", err)
	}
	if prop.PetPolicy() != domain.PetPolicyAllowed {
		t.Errorf("PetPolicy: got %q", prop.PetPolicy())
	}
	events := prop.PullEvents()
	if len(events) != 1 {
		t.Fatalf("eventos: got %d, want 1", len(events))
	}
	evt, ok := events[0].(domain.PropertyPetPolicyUpdated)
	if !ok || evt.OldPolicy != domain.PetPolicyNotAllowed || evt.NewPolicy != domain.PetPolicyAllowed {
		t.Errorf("evento: got %+v", events[0])
	}
}

func TestUpdatePetPolicy_PoliticaInvalida_RetornaBadRequest(t *testing.T) {
	prop := newSaleProperty(t)
	err := prop.UpdatePetPolicy(domain.PetPolicy("SOLO_GATOS"))
	assertBadRequest(t, err)
}

func TestUpdatePetPolicy_DesdeReservedOClosed_RetornaPreconditionFailed(t *testing.T) {
	prop := newSaleProperty(t)
	if err := prop.Close(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := prop.UpdatePetPolicy(domain.PetPolicyAllowed)
	assertPreconditionFailed(t, err)
}

func TestUpdatePetPolicy_ValidacionDePoliticaTienePrioridadSobreEstado(t *testing.T) {
	// A diferencia de UpdateDetails, acá la validación del VALOR va primero en el código:
	// una propiedad cerrada con una política inválida falla por BadRequest, no por PreconditionFailed.
	prop := newSaleProperty(t)
	if err := prop.Close(); err != nil {
		t.Fatalf("setup Close: %v", err)
	}

	err := prop.UpdatePetPolicy(domain.PetPolicy("SOLO_GATOS"))
	assertBadRequest(t, err)
}

// ─── UpdateTempConfig ───────────────────────────────────────────────────────

func TestUpdateTempConfig_PropiedadNoTemp_RetornaPreconditionFailed(t *testing.T) {
	prop := newSaleProperty(t) // SALE, no TEMP
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	execErr := prop.UpdateTempConfig(tc)
	assertPreconditionFailed(t, execErr)
}

func TestUpdateTempConfig_NoTemp_ChequeoDeOperationTypeTienePrioridadSobreEstado(t *testing.T) {
	// Aunque la propiedad SALE esté disponible (no reservada/cerrada), el chequeo
	// de "no es TEMP" corta primero — el mensaje debe reflejar eso, no el de estado.
	prop := newSaleProperty(t)
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	execErr := prop.UpdateTempConfig(tc)
	var appErr *apperr.AppError
	if !errors.As(execErr, &appErr) {
		t.Fatalf("got %v, want *apperr.AppError", execErr)
	}
	if appErr.Message != "solo se puede actualizar la configuración temporal para propiedades de tipo TEMP" {
		t.Errorf("Message: got %q", appErr.Message)
	}
}

func TestUpdateTempConfig_DesdeReservedOClosed_RetornaPreconditionFailed(t *testing.T) {
	prop := newTempProperty(t)
	if err := prop.Reserve(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	execErr := prop.UpdateTempConfig(tc)
	assertPreconditionFailed(t, execErr)
}

func TestUpdateTempConfig_HappyPath(t *testing.T) {
	prop := newTempProperty(t)
	tc, err := domain.NewTempConfig(nil, "16:00", "12:00", 3, 60, 80, 25, 200, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	execErr := prop.UpdateTempConfig(tc)

	if execErr != nil {
		t.Fatalf("UpdateTempConfig: error inesperado: %v", execErr)
	}
	if prop.TempConfig().NightPrice() != 80 {
		t.Errorf("NightPrice: got %v, want 80", prop.TempConfig().NightPrice())
	}
	events := prop.PullEvents()
	if len(events) != 1 {
		t.Fatalf("eventos: got %d, want 1", len(events))
	}
	if _, ok := events[0].(domain.PropertyTempConfigUpdated); !ok {
		t.Errorf("evento: got %T, want PropertyTempConfigUpdated", events[0])
	}
}

// ─── SetTempConfig ──────────────────────────────────────────────────────────

func TestSetTempConfig_AsignaSinValidarNiRegistrarEvento(t *testing.T) {
	prop := newSaleProperty(t)
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 999, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	prop.SetTempConfig(tc)

	if prop.TempConfig().NightPrice() != 999 {
		t.Errorf("TempConfig: got %+v, want reflejar el config seteado", prop.TempConfig())
	}
	if len(prop.PullEvents()) != 0 {
		t.Error("SetTempConfig no debería registrar eventos (es un setter de infraestructura, no una transición de negocio)")
	}
}

// ─── Múltiples mutaciones acumulan eventos hasta el próximo PullEvents ─────

func TestProperty_AcumulaEventosDeMultiplesMutacionesHastaElProximoPull(t *testing.T) {
	prop := newSaleProperty(t)

	if err := prop.UpdatePetPolicy(domain.PetPolicyAllowed); err != nil {
		t.Fatalf("UpdatePetPolicy: %v", err)
	}
	if err := prop.Reserve(); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	events := prop.PullEvents()
	if len(events) != 2 {
		t.Fatalf("eventos acumulados: got %d, want 2", len(events))
	}
	// PullEvents debe drenar — una segunda llamada sin nuevas mutaciones no repite nada.
	if len(prop.PullEvents()) != 0 {
		t.Error("PullEvents debería vaciar la cola de eventos tras la primera lectura")
	}
}
