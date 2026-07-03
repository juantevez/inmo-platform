package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"inmo.platform/contexts/finances/internal/application"
	"inmo.platform/contexts/finances/internal/domain"
)

type SettlementHandler struct {
	createUseCase *application.CreateSettlementUseCase
	addConceptUC  *application.AddConceptUseCase
	closeUseCase  *application.CloseSettlementUseCase
}

func NewSettlementHandler(
	create *application.CreateSettlementUseCase,
	addConcept *application.AddConceptUseCase,
	closeUC *application.CloseSettlementUseCase,
) *SettlementHandler {
	return &SettlementHandler{
		createUseCase: create,
		addConceptUC:  addConcept,
		closeUseCase:  closeUC,
	}
}

// HandleCreate maneja la apertura de una nueva liquidación mensual
func (h *SettlementHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.respondWithError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}

	var cmd application.CreateSettlementCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "JSON inválido en el cuerpo de la petición")
		return
	}

	err := h.createUseCase.Execute(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, application.ErrContractNotFoundOrInactive) || errors.Is(err, application.ErrSettlementAlreadyExists) {
			h.respondWithError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondWithJSON(w, http.StatusCreated, map[string]string{"status": "Liquidación creada exitosamente"})
}

// HandleAddConcept inyecta un nuevo concepto (alquiler, luz, gas) a una liquidación abierta
func (h *SettlementHandler) HandleAddConcept(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.respondWithError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}

	var cmd application.AddConceptCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	err := h.addConceptUC.Execute(r.Context(), cmd)
	if err != nil {
		// Si el dominio salta porque la liquidación no está abierta u otro error de invariante
		if errors.Is(err, domain.ErrSettlementNotOpen) || errors.Is(err, domain.ErrInvalidConceptAmount) || errors.Is(err, domain.ErrEmptyConceptDescription) {
			h.respondWithError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if errors.Is(err, application.ErrSettlementNotFound) {
			h.respondWithError(w, http.StatusNotFound, err.Error())
			return
		}
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{"status": "Concepto agregado exitosamente"})
}

// HandleClose cierra permanentemente la liquidación y dispara eventos
func (h *SettlementHandler) HandleClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.respondWithError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}

	// Esperamos un cuerpo simple {"settlement_id": "UUID"}
	var req struct {
		SettlementID string `json:"settlement_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	err := h.closeUseCase.Execute(r.Context(), req.SettlementID)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidStatusTransition) {
			h.respondWithError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if errors.Is(err, application.ErrSettlementNotFound) {
			h.respondWithError(w, http.StatusNotFound, err.Error())
			return
		}
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{"status": "Liquidación cerrada y emitida correctamente"})
}

// Helpers estándar de respuesta
func (h *SettlementHandler) respondWithError(w http.ResponseWriter, code int, message string) {
	h.respondWithJSON(w, code, map[string]string{"error": message})
}

func (h *SettlementHandler) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
