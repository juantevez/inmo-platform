package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
	"time"
)

type PropertyRepository struct {
	db *sql.DB
}

func NewPropertyRepository(db *sql.DB) *PropertyRepository {
	return &PropertyRepository{db: db}
}

func (r *PropertyRepository) Save(ctx context.Context, p *domain.Property) error {
	query := `
		INSERT INTO properties (id, owner_id, title, description, price, currency, latitude, longitude, address, state, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			price = EXCLUDED.price,
			currency = EXCLUDED.currency,
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			address = EXCLUDED.address,
			state = EXCLUDED.state,
			updated_at = CURRENT_TIMESTAMP;
	`

	_, err := r.db.ExecContext(ctx, query,
		p.ID(),
		p.OwnerID(),
		p.Title(),
		p.Description(),
		p.Price().Amount(),
		string(p.Price().Currency()),
		p.Location().Latitude(),
		p.Location().Longitude(),
		p.Location().Address(),
		string(p.State()),
	)

	if err != nil {
		return apperr.NewInternal("error al guardar la propiedad en postgres", err)
	}
	return nil
}

func (r *PropertyRepository) FindByID(ctx context.Context, id string) (*domain.Property, error) {
	query := `SELECT id, owner_id, title, description, price, currency, latitude, longitude, address, state FROM properties WHERE id = $1`

	row := r.db.QueryRowContext(ctx, query, id)

	var (
		propID, ownerID, title, description, currency, state, address string
		priceAmount, lat, lng                                         float64
	)

	err := row.Scan(&propID, &ownerID, &title, &description, &priceAmount, &currency, &lat, &lng, &address, &state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Hexagonal: retornar nil si no existe, la capa de aplicacion decidira si es un 404
		}
		return nil, apperr.NewInternal("error al buscar la propiedad en postgres", err)
	}

	// Reconstrucción controlada del Agregado de Dominio protegiendo invariantes
	price, err := domain.NewPrice(priceAmount, domain.Currency(currency))
	if err != nil {
		return nil, err
	}

	location, err := domain.NewLocation(lat, lng, address)
	if err != nil {
		return nil, err
	}

	property, err := domain.NewProperty(propID, ownerID, title, description, price, location)
	if err != nil {
		return nil, err
	}

	// Forzar el estado actual recuperado de la BD ya que NewProperty por defecto nace AVAILABLE
	// Creamos un bypass controlado en base de datos o simulamos las transiciones necesarias.
	// Para hacerlo limpio en la reconstruccion desde persistencia:
	switch domain.PropertyState(state) {
	case domain.StateReserved:
		_ = property.Reserve()
	case domain.StateClosed:
		_ = property.Close()
	case domain.StateUnderRepair:
		_ = property.PutUnderRepair()
	}

	// Limpiamos los eventos del agregado porque leer de la BD no es una accion comercial que requiera re-emitir eventos
	_ = property.PullEvents()

	return property, nil
}

// SaveWithTx permite guardar el agregado y sus eventos de outbox dentro de la misma transacción de base de datos
func (r *PropertyRepository) SaveWithTx(ctx context.Context, tx *sql.Tx, p *domain.Property) error {
	query := `
		INSERT INTO properties (id, owner_id, title, description, price, currency, latitude, longitude, address, state, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title, description = EXCLUDED.description, price = EXCLUDED.price,
			currency = EXCLUDED.currency, latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude,
			address = EXCLUDED.address, state = EXCLUDED.state, updated_at = CURRENT_TIMESTAMP;
	`

	_, err := tx.ExecContext(ctx, query,
		p.ID(), p.OwnerID(), p.Title(), p.Description(), p.Price().Amount(),
		string(p.Price().Currency()), p.Location().Latitude(), p.Location().Longitude(),
		p.Location().Address(), string(p.State()),
	)
	if err != nil {
		return apperr.NewInternal("error al guardar la propiedad en la tx de postgres", err)
	}

	// Guardar los eventos de dominio en la tabla outbox dentro de la misma transacción
	for _, event := range p.PullEvents() {
		payloadBytes, err := json.Marshal(event)
		if err != nil {
			return apperr.NewInternal("error al serializar evento para outbox", err)
		}

		outboxQuery := `
			INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_name, payload, status)
			VALUES ($1, $2, $3, $4, $5, 'PENDING');
		`
		// Usamos el ID del evento o generamos uno temporal combinado
		eventID := fmt.Sprintf("evt-%s-%d", event.AggregateID(), time.Now().UnixNano())

		_, err = tx.ExecContext(ctx, outboxQuery,
			eventID,
			"Property",
			event.AggregateID(),
			event.EventName(),
			payloadBytes,
		)
		if err != nil {
			return apperr.NewInternal("error al insertar en la tabla outbox", err)
		}
	}

	return nil
}
