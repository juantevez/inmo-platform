package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// WorkerStatus rastrea si un worker de background sigue corriendo.
// Debe crearse antes de lanzar la goroutine y pasarse al Checker.
type WorkerStatus struct {
	Name    string
	running atomic.Bool
}

func NewWorkerStatus(name string) *WorkerStatus {
	w := &WorkerStatus{Name: name}
	w.running.Store(true)
	return w
}

// MarkStopped marca al worker como detenido. Llamar con defer dentro de la goroutine.
func (w *WorkerStatus) MarkStopped() {
	w.running.Store(false)
}

func (w *WorkerStatus) IsRunning() bool {
	return w.running.Load()
}

// Checker implementa los handlers de liveness y readiness.
type Checker struct {
	db       *sql.DB
	natsConn *nats.Conn
	workers  []*WorkerStatus
}

func NewChecker(db *sql.DB, natsConn *nats.Conn, workers ...*WorkerStatus) *Checker {
	return &Checker{db: db, natsConn: natsConn, workers: workers}
}

// LiveHandler responde 200 siempre. Indica que el proceso está vivo.
func (c *Checker) LiveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type componentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type readyResponse struct {
	Status     string                     `json:"status"`
	Components map[string]componentStatus `json:"components"`
}

// ReadyHandler verifica Postgres, NATS y el estado de cada worker.
// Responde 503 si algún componente falla, 200 si todo está operativo.
func (c *Checker) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	components := make(map[string]componentStatus)
	healthy := true

	if err := c.db.PingContext(ctx); err != nil {
		components["postgres"] = componentStatus{Status: "error", Error: err.Error()}
		healthy = false
	} else {
		components["postgres"] = componentStatus{Status: "ok"}
	}

	if c.natsConn.Status() != nats.CONNECTED {
		components["nats"] = componentStatus{Status: "error", Error: "not connected"}
		healthy = false
	} else {
		components["nats"] = componentStatus{Status: "ok"}
	}

	for _, ws := range c.workers {
		if ws.IsRunning() {
			components[ws.Name] = componentStatus{Status: "ok"}
		} else {
			components[ws.Name] = componentStatus{Status: "stopped"}
			healthy = false
		}
	}

	status := "ok"
	code := http.StatusOK
	if !healthy {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(readyResponse{Status: status, Components: components})
}
