package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type PropertyRepository struct {
	db *sql.DB
}

func NewPropertyRepository(db *sql.DB) *PropertyRepository {
	return &PropertyRepository{db: db}
}

func (r *PropertyRepository) Save(ctx context.Context, p *domain.Property) error {
	amenitiesJSON, pricingRulesJSON, err := marshalTempFields(p)
	if err != nil {
		return err
	}
	tc := p.TempConfig()
	query := `
		INSERT INTO properties (
			id, owner_id, title, description, price, currency, latitude, longitude, address,
			state, operation_type, pet_policy,
			amenities, check_in_time, check_out_time, min_nights, max_nights,
			night_price, cleaning_fee, security_deposit, pricing_rules,
			updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			title=EXCLUDED.title, description=EXCLUDED.description,
			price=EXCLUDED.price, currency=EXCLUDED.currency,
			latitude=EXCLUDED.latitude, longitude=EXCLUDED.longitude, address=EXCLUDED.address,
			state=EXCLUDED.state, operation_type=EXCLUDED.operation_type, pet_policy=EXCLUDED.pet_policy,
			amenities=EXCLUDED.amenities, check_in_time=EXCLUDED.check_in_time,
			check_out_time=EXCLUDED.check_out_time, min_nights=EXCLUDED.min_nights,
			max_nights=EXCLUDED.max_nights, night_price=EXCLUDED.night_price,
			cleaning_fee=EXCLUDED.cleaning_fee, security_deposit=EXCLUDED.security_deposit,
			pricing_rules=EXCLUDED.pricing_rules, updated_at=CURRENT_TIMESTAMP;
	`
	_, err = r.db.ExecContext(ctx, query,
		p.ID(), p.OwnerID(), p.Title(), p.Description(),
		p.Price().Amount(), string(p.Price().Currency()),
		p.Location().Latitude(), p.Location().Longitude(), p.Location().Address(),
		string(p.State()), string(p.OperationType()), string(p.PetPolicy()),
		amenitiesJSON, tc.CheckInTime(), tc.CheckOutTime(), tc.MinNights(), tc.MaxNights(),
		nullFloat(tc.NightPrice()), tc.CleaningFee(), tc.SecurityDeposit(), pricingRulesJSON,
	)
	if err != nil {
		return apperr.NewInternal("error al guardar la propiedad en postgres", err)
	}
	return nil
}

func (r *PropertyRepository) FindByID(ctx context.Context, id string) (*domain.Property, error) {
	query := `
		SELECT id, owner_id, title, description, price, currency, latitude, longitude, address,
		       state, operation_type, pet_policy,
		       amenities, check_in_time, check_out_time, min_nights, max_nights,
		       night_price, cleaning_fee, security_deposit, pricing_rules
		FROM properties WHERE id = $1`

	row := r.db.QueryRowContext(ctx, query, id)

	var (
		propID, ownerID, title, description, currency, state, address, opType, petPolicy string
		priceAmount, lat, lng                                                            float64
		amenitiesRaw, pricingRulesRaw                                                    []byte
		checkInTime, checkOutTime                                                        string
		minNights, maxNights                                                             int
		nightPrice, cleaningFee, securityDeposit                                         sql.NullFloat64
	)

	err := row.Scan(
		&propID, &ownerID, &title, &description, &priceAmount, &currency, &lat, &lng, &address,
		&state, &opType, &petPolicy,
		&amenitiesRaw, &checkInTime, &checkOutTime, &minNights, &maxNights,
		&nightPrice, &cleaningFee, &securityDeposit, &pricingRulesRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperr.NewInternal("error al buscar la propiedad en postgres", err)
	}

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

	switch domain.PropertyState(state) {
	case domain.StateReserved:
		_ = property.Reserve()
	case domain.StateClosed:
		_ = property.Close()
	case domain.StateUnderRepair:
		_ = property.PutUnderRepair()
	}

	if tc, err := unmarshalTempConfig(amenitiesRaw, pricingRulesRaw, checkInTime, checkOutTime, minNights, maxNights, nightPrice.Float64, cleaningFee.Float64, securityDeposit.Float64); err == nil {
		property.SetTempConfig(tc)
	}

	_ = property.PullEvents()
	return property, nil
}

func (r *PropertyRepository) FindAll(ctx context.Context, f ports.ListFilters) ([]ports.PropertyResult, int, error) {
	where, whereArgs := buildListWhere(f)

	// Total antes de paginar
	var total int
	countQuery := "SELECT COUNT(*) FROM properties" + where
	if err := r.db.QueryRowContext(ctx, countQuery, whereArgs...).Scan(&total); err != nil {
		return nil, 0, apperr.NewInternal("error al contar propiedades en postgres", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	// 🗺️ Construimos queryArgs en orden estricto: whereArgs → distCol → orderBy → limit/offset
	queryArgs := make([]interface{}, len(whereArgs))
	copy(queryArgs, whereArgs)
	n := len(queryArgs) + 1

	var distCol, orderBy string
	if f.RadiusKm > 0 {
		distCol = fmt.Sprintf(
			", ROUND(ST_Distance(location, ST_MakePoint($%d, $%d)::geography)::numeric, 0) AS dist_m",
			n, n+1,
		)
		queryArgs = append(queryArgs, f.Longitude, f.Latitude)
		n += 2

		orderBy = fmt.Sprintf(
			" ORDER BY location <-> ST_MakePoint($%d, $%d)::geography",
			n, n+1,
		)
		queryArgs = append(queryArgs, f.Longitude, f.Latitude)
		n += 2
	} else {
		orderBy = " ORDER BY created_at DESC"
	}

	dataQuery := fmt.Sprintf(
		`SELECT id, owner_id, title, description, price, currency, latitude, longitude, address,
		        state, operation_type, pet_policy,
		        amenities, check_in_time, check_out_time, min_nights, max_nights,
		        night_price, cleaning_fee, security_deposit, pricing_rules%s
		 FROM properties%s%s LIMIT $%d OFFSET $%d`,
		distCol, where, orderBy, n, n+1,
	)
	queryArgs = append(queryArgs, limit, f.Offset)

	rows, err := r.db.QueryContext(ctx, dataQuery, queryArgs...)
	if err != nil {
		return nil, 0, apperr.NewInternal("error al listar propiedades en postgres", err)
	}
	defer rows.Close()

	var results []ports.PropertyResult
	for rows.Next() {
		var (
			propID, ownerID, title, description, currency, state, address, opType, petPolicy string
			priceAmount, lat, lng                                                            float64
			amenitiesRaw, pricingRulesRaw                                                    []byte
			checkInTime, checkOutTime                                                        string
			minNights, maxNights                                                             int
			nightPrice, cleaningFee, securityDeposit                                         sql.NullFloat64
			distM                                                                            sql.NullFloat64
		)

		scanArgs := []any{
			&propID, &ownerID, &title, &description, &priceAmount, &currency, &lat, &lng, &address,
			&state, &opType, &petPolicy,
			&amenitiesRaw, &checkInTime, &checkOutTime, &minNights, &maxNights,
			&nightPrice, &cleaningFee, &securityDeposit, &pricingRulesRaw,
		}
		if f.RadiusKm > 0 {
			scanArgs = append(scanArgs, &distM)
		}

		if err := rows.Scan(scanArgs...); err != nil {
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
		if tc, err := unmarshalTempConfig(amenitiesRaw, pricingRulesRaw, checkInTime, checkOutTime, minNights, maxNights, nightPrice.Float64, cleaningFee.Float64, securityDeposit.Float64); err == nil {
			property.SetTempConfig(tc)
		}
		_ = property.PullEvents()

		result := ports.PropertyResult{Property: property}
		if f.RadiusKm > 0 && distM.Valid {
			d := distM.Float64
			result.DistanceM = &d
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, apperr.NewInternal("error al iterar propiedades en postgres", err)
	}
	return results, total, nil
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

	if f.OwnerID != "" {
		clauses = append(clauses, fmt.Sprintf("owner_id = $%d", n))
		args = append(args, f.OwnerID)
		n++
	}

	if f.RadiusKm > 0 {
		clauses = append(clauses, fmt.Sprintf(
			"ST_DWithin(location, ST_MakePoint($%d, $%d)::geography, $%d)",
			n, n+1, n+2,
		))
		args = append(args, f.Longitude, f.Latitude, f.RadiusKm*1000) // metros
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// SaveWithTx permite guardar el agregado y sus eventos de outbox dentro de la misma transacción de base de datos
func (r *PropertyRepository) SaveWithTx(ctx context.Context, tx *sql.Tx, p *domain.Property) error {
	amenitiesJSON, pricingRulesJSON, err := marshalTempFields(p)
	if err != nil {
		return err
	}
	tc := p.TempConfig()
	query := `
		INSERT INTO properties (
			id, owner_id, title, description, price, currency, latitude, longitude, address,
			state, operation_type, pet_policy,
			amenities, check_in_time, check_out_time, min_nights, max_nights,
			night_price, cleaning_fee, security_deposit, pricing_rules,
			updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			title=EXCLUDED.title, description=EXCLUDED.description,
			price=EXCLUDED.price, currency=EXCLUDED.currency,
			latitude=EXCLUDED.latitude, longitude=EXCLUDED.longitude, address=EXCLUDED.address,
			state=EXCLUDED.state, operation_type=EXCLUDED.operation_type, pet_policy=EXCLUDED.pet_policy,
			amenities=EXCLUDED.amenities, check_in_time=EXCLUDED.check_in_time,
			check_out_time=EXCLUDED.check_out_time, min_nights=EXCLUDED.min_nights,
			max_nights=EXCLUDED.max_nights, night_price=EXCLUDED.night_price,
			cleaning_fee=EXCLUDED.cleaning_fee, security_deposit=EXCLUDED.security_deposit,
			pricing_rules=EXCLUDED.pricing_rules, updated_at=CURRENT_TIMESTAMP;
	`
	_, err = tx.ExecContext(ctx, query,
		p.ID(), p.OwnerID(), p.Title(), p.Description(),
		p.Price().Amount(), string(p.Price().Currency()),
		p.Location().Latitude(), p.Location().Longitude(), p.Location().Address(),
		string(p.State()), string(p.OperationType()), string(p.PetPolicy()),
		amenitiesJSON, tc.CheckInTime(), tc.CheckOutTime(), tc.MinNights(), tc.MaxNights(),
		nullFloat(tc.NightPrice()), tc.CleaningFee(), tc.SecurityDeposit(), pricingRulesJSON,
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

// ── Helpers de serialización TEMP ──────────────────────────────────────────

func marshalTempFields(p *domain.Property) (amenitiesJSON, pricingRulesJSON []byte, err error) {
	tc := p.TempConfig()
	if len(tc.Amenities()) > 0 {
		amenitiesJSON, err = json.Marshal(tc.Amenities())
		if err != nil {
			return nil, nil, apperr.NewInternal("error al serializar amenities", err)
		}
	}
	if len(tc.PricingRules()) > 0 {
		pricingRulesJSON, err = json.Marshal(tc.PricingRules())
		if err != nil {
			return nil, nil, apperr.NewInternal("error al serializar pricing_rules", err)
		}
	}
	return amenitiesJSON, pricingRulesJSON, nil
}

func unmarshalTempConfig(amenitiesRaw, pricingRulesRaw []byte, checkIn, checkOut string, minN, maxN int, nightPrice, cleaningFee, securityDeposit float64) (domain.TempConfig, error) {
	var amenities []domain.Amenity
	if len(amenitiesRaw) > 0 {
		if err := json.Unmarshal(amenitiesRaw, &amenities); err != nil {
			return domain.TempConfig{}, apperr.NewInternal("error al deserializar amenities", err)
		}
	}
	var rules []domain.PricingRule
	if len(pricingRulesRaw) > 0 {
		if err := json.Unmarshal(pricingRulesRaw, &rules); err != nil {
			return domain.TempConfig{}, apperr.NewInternal("error al deserializar pricing_rules", err)
		}
	}
	if minN == 0 {
		minN = 1
	}
	if maxN == 0 {
		maxN = 90
	}
	return domain.NewTempConfig(amenities, checkIn, checkOut, minN, maxN, nightPrice, cleaningFee, securityDeposit, rules)
}

func nullFloat(f float64) interface{} {
	if f == 0 {
		return nil
	}
	return f
}
