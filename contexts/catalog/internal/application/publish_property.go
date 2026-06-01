package application

import (
	"context"
	"database/sql"
	"fmt"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
)

// PublishPropertyDTO define los datos de entrada requeridos para publicar una propiedad.
type PublishPropertyDTO struct {
	ID            string
	OwnerID       string
	Title         string
	Description   string
	Price         float64
	Currency      string
	Latitude      float64
	Longitude     float64
	Address       string
	OperationType string
	PetPolicy     string
	// Campos de alquiler temporario (opcionales, solo aplican si OperationType == "TEMP")
	Amenities       []domain.Amenity
	CheckInTime     string
	CheckOutTime    string
	MinNights       int
	MaxNights       int
	NightPrice      float64
	CleaningFee     float64
	SecurityDeposit float64
	PricingRules    []domain.PricingRule
}

type PublishPropertyUseCase struct {
	dbPool       *sql.DB
	propertyRepo ports.PropertyRepository
}

func NewPublishPropertyUseCase(dbPool *sql.DB, repo ports.PropertyRepository) *PublishPropertyUseCase {
	return &PublishPropertyUseCase{
		dbPool:       dbPool,
		propertyRepo: repo,
	}
}

func (uc *PublishPropertyUseCase) Execute(ctx context.Context, dto PublishPropertyDTO) error {
	price, err := domain.NewPrice(dto.Price, domain.Currency(dto.Currency))
	if err != nil {
		return err
	}

	location, err := domain.NewLocation(dto.Latitude, dto.Longitude, dto.Address)
	if err != nil {
		return err
	}

	opType := domain.OperationType(dto.OperationType)
	if opType == "" {
		opType = domain.OperationSale
	}

	petPolicy := domain.PetPolicy(dto.PetPolicy)
	if petPolicy == "" {
		petPolicy = domain.PetPolicyNotAllowed
	}

	property, err := domain.NewProperty(dto.ID, dto.OwnerID, dto.Title, dto.Description, price, location, opType, petPolicy)
	if err != nil {
		return err
	}

	if opType == domain.OperationTemp {
		tc, err := domain.NewTempConfig(
			dto.Amenities, dto.CheckInTime, dto.CheckOutTime,
			dto.MinNights, dto.MaxNights,
			dto.NightPrice, dto.CleaningFee, dto.SecurityDeposit,
			dto.PricingRules,
		)
		if err != nil {
			return err
		}
		property.SetTempConfig(tc)
	}

	// El evento se registra acá, después de SetTempConfig, para que el snapshot
	// incluya night_price, pricing_rules, etc. con sus valores reales.
	property.RecordEvent(domain.NewPropertyPublished(property))

	// ─── TRANSACCIÓN ATÓMICA ───
	tx, err := uc.dbPool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Type Assertion para usar las capacidades de la base de datos real sin romper el desacoplamiento
	type TxRepository interface {
		SaveWithTx(ctx context.Context, tx *sql.Tx, p *domain.Property) error
	}

	txRepo, ok := uc.propertyRepo.(TxRepository)
	if !ok {
		return fmt.Errorf("el repositorio configurado no soporta transacciones outbox")
	}

	// Guardamos la propiedad y los eventos en el Outbox usando la Tx de Postgres
	if err := txRepo.SaveWithTx(ctx, tx, property); err != nil {
		return err
	}

	// Confirmamos de forma atómica en Postgres
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}
