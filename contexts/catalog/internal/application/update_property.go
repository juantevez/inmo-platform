package application

import (
	"context"
	"database/sql"
	"fmt"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

// UpdatePropertyDTO define los datos de entrada para actualizar una propiedad.
type UpdatePropertyDTO struct {
	Title       *string  `json:"title,omitempty"`
	Description *string  `json:"description,omitempty"`
	Price       *float64 `json:"price,omitempty"`
	Currency    *string  `json:"currency,omitempty"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	Address     *string  `json:"address,omitempty"`
	PetPolicy   *string  `json:"pet_policy,omitempty"`
	// Campos de alquiler temporario (opcionales, solo aplican si OperationType == "TEMP")
	CheckInTime     *string                `json:"check_in_time,omitempty"`
	CheckOutTime    *string                `json:"check_out_time,omitempty"`
	MinNights       *int                   `json:"min_nights,omitempty"`
	MaxNights       *int                   `json:"max_nights,omitempty"`
	NightPrice      *float64               `json:"night_price,omitempty"`
	CleaningFee     *float64               `json:"cleaning_fee,omitempty"`
	SecurityDeposit *float64               `json:"security_deposit,omitempty"`
	PricingRules    []domain.PricingRule   `json:"pricing_rules,omitempty"`
}

type UpdatePropertyUseCase struct {
	dbPool       *sql.DB
	propertyRepo ports.PropertyRepository
}

func NewUpdatePropertyUseCase(dbPool *sql.DB, repo ports.PropertyRepository) *UpdatePropertyUseCase {
	return &UpdatePropertyUseCase{
		dbPool:       dbPool,
		propertyRepo: repo,
	}
}

func (uc *UpdatePropertyUseCase) Execute(ctx context.Context, propertyID string, dto UpdatePropertyDTO) error {
	// Recuperar la propiedad actual
	property, err := uc.propertyRepo.FindByID(ctx, propertyID)
	if err != nil {
		return err
	}
	if property == nil {
		return apperr.NewNotFound("propiedad no encontrada", nil)
	}

	// Actualizar título, descripción y precio si se proporcionan
	if dto.Title != nil || dto.Description != nil || dto.Price != nil {
		newTitle := property.Title()
		newDescription := property.Description()
		newPrice := property.Price()

		if dto.Title != nil {
			newTitle = *dto.Title
		}
		if dto.Description != nil {
			newDescription = *dto.Description
		}
		if dto.Price != nil {
			currency := property.Price().Currency()
			if dto.Currency != nil {
				currency = domain.Currency(*dto.Currency)
			}
			newPrice, err = domain.NewPrice(*dto.Price, currency)
			if err != nil {
				return err
			}
		}

		if err := property.UpdateDetails(newTitle, newDescription, newPrice); err != nil {
			return err
		}
	}

	// Actualizar ubicación si se proporciona
	if dto.Latitude != nil || dto.Longitude != nil || dto.Address != nil {
		newLat := property.Location().Latitude()
		newLng := property.Location().Longitude()
		newAddr := property.Location().Address()

		if dto.Latitude != nil {
			newLat = *dto.Latitude
		}
		if dto.Longitude != nil {
			newLng = *dto.Longitude
		}
		if dto.Address != nil {
			newAddr = *dto.Address
		}

		if err := property.UpdateLocation(newLat, newLng, newAddr); err != nil {
			return err
		}
	}

	// Actualizar política de mascotas si se proporciona
	if dto.PetPolicy != nil {
		if err := property.UpdatePetPolicy(domain.PetPolicy(*dto.PetPolicy)); err != nil {
			return err
		}
	}

	// Actualizar configuración temporal si se proporciona algún campo
	if property.OperationType() == domain.OperationTemp && (dto.CheckInTime != nil || dto.CheckOutTime != nil ||
		dto.MinNights != nil || dto.MaxNights != nil || dto.NightPrice != nil ||
		dto.CleaningFee != nil || dto.SecurityDeposit != nil || len(dto.PricingRules) > 0) {

		currentConfig := property.TempConfig()

		newCheckIn := currentConfig.CheckInTime()
		newCheckOut := currentConfig.CheckOutTime()
		newMinNights := currentConfig.MinNights()
		newMaxNights := currentConfig.MaxNights()
		newNightPrice := currentConfig.NightPrice()
		newCleaningFee := currentConfig.CleaningFee()
		newSecurityDeposit := currentConfig.SecurityDeposit()
		newPricingRules := currentConfig.PricingRules()

		if dto.CheckInTime != nil {
			newCheckIn = *dto.CheckInTime
		}
		if dto.CheckOutTime != nil {
			newCheckOut = *dto.CheckOutTime
		}
		if dto.MinNights != nil {
			newMinNights = *dto.MinNights
		}
		if dto.MaxNights != nil {
			newMaxNights = *dto.MaxNights
		}
		if dto.NightPrice != nil {
			newNightPrice = *dto.NightPrice
		}
		if dto.CleaningFee != nil {
			newCleaningFee = *dto.CleaningFee
		}
		if dto.SecurityDeposit != nil {
			newSecurityDeposit = *dto.SecurityDeposit
		}
		if len(dto.PricingRules) > 0 {
			newPricingRules = dto.PricingRules
		}

		newConfig, err := domain.NewTempConfig(
			currentConfig.Amenities(),
			newCheckIn, newCheckOut,
			newMinNights, newMaxNights,
			newNightPrice, newCleaningFee, newSecurityDeposit,
			newPricingRules,
		)
		if err != nil {
			return err
		}

		if err := property.UpdateTempConfig(newConfig); err != nil {
			return err
		}
	}

	// ─── TRANSACCIÓN ATÓMICA ───
	tx, err := uc.dbPool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Type Assertion para usar las capacidades de la base de datos real
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
