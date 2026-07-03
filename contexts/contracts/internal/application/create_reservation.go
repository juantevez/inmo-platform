package application

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/contexts/contracts/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type CreateReservationUseCase struct {
	db       *sql.DB
	resRepo  ports.ReservationRepository
	snapRepo ports.PropertySnapshotRepository
}

func NewCreateReservationUseCase(db *sql.DB, resRepo ports.ReservationRepository, snapRepo ports.PropertySnapshotRepository) *CreateReservationUseCase {
	return &CreateReservationUseCase{db: db, resRepo: resRepo, snapRepo: snapRepo}
}

type CreateReservationCommand struct {
	PropertyID   string
	TenantID     string
	CheckInDate  time.Time
	CheckOutDate time.Time
	GuestMessage string
}

type ReservationDTO struct {
	ID              string  `json:"id"`
	PropertyID      string  `json:"property_id"`
	TenantID        string  `json:"tenant_id"`
	OwnerID         string  `json:"owner_id"`
	CheckInDate     string  `json:"check_in_date"`
	CheckOutDate    string  `json:"check_out_date"`
	CheckInTime     string  `json:"check_in_time"`
	CheckOutTime    string  `json:"check_out_time"`
	Nights          int     `json:"nights"`
	NightPrice      float64 `json:"night_price"`
	DiscountPct     float64 `json:"discount_pct"`
	CleaningFee     float64 `json:"cleaning_fee"`
	SecurityDeposit float64 `json:"security_deposit"`
	TotalAmount     float64 `json:"total_amount"`
	Status          string  `json:"status"`
	GuestMessage    string  `json:"guest_message,omitempty"`
}

func (uc *CreateReservationUseCase) Execute(ctx context.Context, cmd CreateReservationCommand) (*ReservationDTO, error) {
	snap, err := uc.snapRepo.FindByID(ctx, cmd.PropertyID)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, apperr.NewNotFound("propiedad no encontrada o no es de tipo temporario", nil)
	}

	nights := int(cmd.CheckOutDate.Sub(cmd.CheckInDate).Hours() / 24)
	if nights < snap.MinNights {
		return nil, apperr.NewBadRequest(fmt.Sprintf("estadía mínima es %d noches", snap.MinNights), nil)
	}
	if snap.MaxNights > 0 && nights > snap.MaxNights {
		return nil, apperr.NewBadRequest(fmt.Sprintf("estadía máxima es %d noches", snap.MaxNights), nil)
	}

	hasOverlap, err := uc.resRepo.HasOverlap(ctx, cmd.PropertyID, cmd.CheckInDate, cmd.CheckOutDate)
	if err != nil {
		return nil, err
	}
	if hasOverlap {
		return nil, apperr.NewPreconditionFailed("las fechas seleccionadas ya están reservadas", nil)
	}

	discountPct := snap.ApplyDiscount(nights)
	subtotal := snap.NightPrice * float64(nights)
	discount := subtotal * discountPct / 100
	total := subtotal - discount + snap.CleaningFee

	resID := fmt.Sprintf("res-%d", time.Now().UnixNano())
	reservation, err := domain.NewReservation(
		resID, cmd.PropertyID, cmd.TenantID, snap.OwnerID,
		cmd.CheckInDate, cmd.CheckOutDate, nights,
		snap.NightPrice, discountPct, snap.CleaningFee, snap.SecurityDeposit, total,
		cmd.GuestMessage,
	)
	if err != nil {
		return nil, err
	}

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperr.NewInternal("error al iniciar transacción", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := uc.resRepo.SaveWithTx(ctx, tx, reservation); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, apperr.NewInternal("error al confirmar transacción", err)
	}

	return toReservationDTO(reservation, snap), nil
}

func toReservationDTO(r *domain.Reservation, snap *domain.PropertySnapshot) *ReservationDTO {
	dto := &ReservationDTO{
		ID:              r.ID(),
		PropertyID:      r.PropertyID(),
		TenantID:        r.TenantID(),
		OwnerID:         r.OwnerID(),
		CheckInDate:     r.CheckInDate().Format("2006-01-02"),
		CheckOutDate:    r.CheckOutDate().Format("2006-01-02"),
		Nights:          r.Nights(),
		NightPrice:      r.NightPriceSnapshot(),
		DiscountPct:     r.DiscountPct(),
		CleaningFee:     r.CleaningFee(),
		SecurityDeposit: r.SecurityDeposit(),
		TotalAmount:     r.TotalAmount(),
		Status:          string(r.Status()),
		GuestMessage:    r.GuestMessage(),
	}
	if snap != nil {
		dto.CheckInTime = snap.CheckInTime
		dto.CheckOutTime = snap.CheckOutTime
	}
	return dto
}
