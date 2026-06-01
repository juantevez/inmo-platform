package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
	"strings"
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
		INSERT INTO properties (id, owner_id, title, description, price, currency, latitude, longitude, address, state, operation_type, pet_policy, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			price = EXCLUDED.price,
			currency = EXCLUDED.currency,
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			address = EXCLUDED.address,
			state = EXCLUDED.state,
			operation_type = EXCLUDED.operation_type,
			pet_policy = EXCLUDED.pet_policy,
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
		string(p.OperationType()),
		string(p.PetPolicy()),
	)

	if err != nil {
		return apperr.NewInternal("error al guardar la propiedad en postgres", err)
	}
	return nil
}

func (r *PropertyRepository) FindByID(ctx context.Context, id string) (*domain.Property, error) {
	query := `SELECT id, owner_id, title, description, price, currency, latitude, longitude, address, state, operation_type, pet_policy FROM properties WHERE id = $1`

	row := r.db.QueryRowContext(ctx, query, id)

	var (
		propID, ownerID, title, description, currency, state, address, opType, petPolicy string
		priceAmount, lat, lng                                                             float64
	)

	err := row.Scan(&propID, &ownerID, &title, &description, &priceAmount, &currency, &lat, &lng, &address, &state, &opType, &petPolicy)
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

	property, err := domain.NewProperty(propID, ownerID, title, description, price, location, domain.OperationType(opType), domain.PetPolicy(petPolicy))
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

func (r *PropertyRepository) FindAll(ctx context.Context, f ports.ListFilters) ([]*domain.Property, int, error) {
	where, args := buildListWhere(f)

	// Total antes de paginar
	var total int
	countQuery := "SELECT COUNT(*) FROM properties" + where
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, apperr.NewInternal("error al contar propiedades en postgres", err)
	}

	// Página de resultados
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	n := len(args) + 1
	dataQuery := fmt.Sprintf(
		"SELECT id, owner_id, title, description, price, currency, latitude, longitude, address, state, operation_type, pet_policy FROM properties%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, n, n+1,
	)
	args = append(args, limit, f.Offset)

	rows, err := r.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, apperr.NewInternal("error al listar propiedades en postgres", err)
	}
	defer rows.Close()

	var properties []*domain.Property
	for rows.Next() {
		var (
			propID, ownerID, title, description, currency, state, address, opType, petPolicy string
			priceAmount, lat, lng                                                             float64
		)
		if err := rows.Scan(&propID, &ownerID, &title, &description, &priceAmount, &currency, &lat, &lng, &address, &state, &opType, &petPolicy); err != nil {
			return nil, 0, apperr.NewInternal("error al escanear propiedad en postgres", err)
		}

		price, err := domain.NewPrice(priceAmount, domain.Currency(currency))
		if err != nil {
			return nil, 0, err
		}
		location, err := domain.NewLocation(lat, lng, address)
		if err != nil {
			return nil, 0, err
		}
		property, err := domain.NewProperty(propID, ownerID, title, description, price, location, domain.OperationType(opType), domain.PetPolicy(petPolicy))
		if err != nil {
			return nil, 0, err
		}

		switch domain.PropertyState(state) {
		case domain.StateReserved:
			_ = property.Reserve()
		case domain.StateClosed:
			_ = property.Close()
		case domain.StateUnderRepair:
			_ = property.PutUnderRepair()
		}
		_ = property.PullEvents()

		properties = append(properties, property)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, apperr.NewInternal("error al iterar propiedades en postgres", err)
	}
	return properties, total, nil
}

func buildListWhere(f ports.ListFilters) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	n := 1

	if f.State != "" {
		clauses = append(clauses, fmt.Sprintf("state = $%d", n))
		args = append(args, f.State)
		n++
	}
	if f.OperationType != "" {
		clauses = append(clauses, fmt.Sprintf("operation_type = $%d", n))
		args = append(args, f.OperationType)
		n++
	}
	if f.PetPolicy != "" {
		// ALLOWED como filtro de búsqueda significa "acepta mascotas en cualquier modalidad"
		clauses = append(clauses, fmt.Sprintf("pet_policy IN ($%d, $%d)", n, n+1))
		args = append(args, string(domain.PetPolicyAllowed), string(domain.PetPolicySmallOnly))
		n += 2
	}
	if f.MinPrice > 0 {
		clauses = append(clauses, fmt.Sprintf("price >= $%d", n))
		args = append(args, f.MinPrice)
		n++
	}
	if f.MaxPrice > 0 {
		clauses = append(clauses, fmt.Sprintf("price <= $%d", n))
		args = append(args, f.MaxPrice)
		n++
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// SaveWithTx permite guardar el agregado y sus eventos de outbox dentro de la misma transacción de base de datos
func (r *PropertyRepository) SaveWithTx(ctx context.Context, tx *sql.Tx, p *domain.Property) error {
	query := `
		INSERT INTO properties (id, owner_id, title, description, price, currency, latitude, longitude, address, state, operation_type, pet_policy, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title, description = EXCLUDED.description, price = EXCLUDED.price,
			currency = EXCLUDED.currency, latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude,
			address = EXCLUDED.address, state = EXCLUDED.state, operation_type = EXCLUDED.operation_type,
			pet_policy = EXCLUDED.pet_policy, updated_at = CURRENT_TIMESTAMP;
	`

	_, err := tx.ExecContext(ctx, query,
		p.ID(), p.OwnerID(), p.Title(), p.Description(), p.Price().Amount(),
		string(p.Price().Currency()), p.Location().Latitude(), p.Location().Longitude(),
		p.Location().Address(), string(p.State()), string(p.OperationType()), string(p.PetPolicy()),
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
