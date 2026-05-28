package eventbus

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JetStreamConnection encapsula el cliente nativo de NATS y el motor JetStream v2.
type JetStreamConnection struct {
	NC nats.Conn
	JS jetstream.JetStream
}

func NewJetStreamConnection(url string) (*JetStreamConnection, error) {
	// Conexión básica a NATS con reintentos automáticos si se cae el contenedor
	nc, err := nats.Connect(url,
		nats.MaxReconnects(5),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Printf("[NATS WARN] Desconectado del broker: %v. Reintentando...", err)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("error al conectar a NATS Core: %w", err)
	}

	// Inicializar la interfaz moderna de JetStream (v2)
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("error al inicializar JetStream: %w", err)
	}

	return &JetStreamConnection{NC: *nc, JS: js}, nil
}

// EnsureStream garantiza que exista el contenedor persistente de mensajes para un contexto dado.
func (j *JetStreamConnection) EnsureStream(ctx context.Context, streamName string, subjects []string) error {
	cfg := jetstream.StreamConfig{
		Name:      streamName,
		Subjects:  subjects,
		Retention: jetstream.LimitsPolicy, // Guarda los mensajes según límites de tamaño/tiempo
		MaxAge:    24 * 7 * time.Hour,     // Persistir por 1 semana
		Storage:   jetstream.FileStorage,  // Guardar en volumen de disco
	}

	_, err := j.JS.CreateOrUpdateStream(ctx, cfg)
	if err != nil {
		return fmt.Errorf("no se pudo crear/actualizar el stream %s: %w", streamName, err)
	}

	log.Printf("[NATS INFO] Stream '%s' validado y mapeado a los subjects: %v\n", streamName, subjects)
	return nil
}

func (j *JetStreamConnection) Close() {
	j.NC.Close()
}
