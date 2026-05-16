package model

import "time"

// VehicleType represents the type of vehicle
type VehicleType int

const (
	VehicleTypeCar        VehicleType = 1
	VehicleTypeMotorcycle VehicleType = 2
)

// AssignmentMode represents how the spot is assigned
type AssignmentMode int

const (
	AssignmentModeSystemAssigned AssignmentMode = 1
	AssignmentModeUserSelected   AssignmentMode = 2
)

// ReservationStatus represents the current state of a reservation
type ReservationStatus int

const (
	ReservationStatusPendingPayment ReservationStatus = 1
	ReservationStatusConfirmed      ReservationStatus = 2
	ReservationStatusExpired        ReservationStatus = 3
	ReservationStatusCheckedIn      ReservationStatus = 4
	ReservationStatusCompleted      ReservationStatus = 5
	ReservationStatusCancelled      ReservationStatus = 6
)

// Reservation represents a parking reservation record
type Reservation struct {
	ID             string            `db:"id"`
	IdempotencyKey string            `db:"idempotency_key"`
	DriverID       string            `db:"driver_id"`
	SpotID         string            `db:"spot_id"`
	SpotCode       string            `db:"spot_code"`    // populated via JOIN with spots
	FloorNumber    int               `db:"floor_number"` // populated via JOIN with spots
	VehicleType    VehicleType       `db:"vehicle_type"`
	AssignmentMode AssignmentMode    `db:"assignment_mode"`
	Status         ReservationStatus `db:"status"`
	ConfirmedAt    *time.Time        `db:"confirmed_at"`
	ExpiresAt      *time.Time        `db:"expires_at"`
	CreatedAt      time.Time         `db:"created_at"`
	QRCodeURL      string            `db:"qr_code_url"` // stored at creation for idempotency replay
}

// Spot represents a parking spot record
type Spot struct {
	ID          string      `db:"id"`
	FloorNumber int         `db:"floor_number"`
	SpotCode    string      `db:"spot_code"`
	VehicleType VehicleType `db:"vehicle_type"`
	Status      SpotStatus  `db:"status"`
}

// SpotStatus represents the current state of a parking spot
type SpotStatus int

const (
	SpotStatusAvailable SpotStatus = 1
	SpotStatusLocked    SpotStatus = 2
	SpotStatusOccupied  SpotStatus = 3
)

// CreateReservationRequest is the HTTP request body for creating a reservation
type CreateReservationRequest struct {
	IdempotencyKey string         `json:"idempotency_key" validate:"required,uuid"`
	DriverID       string         `json:"driver_id" validate:"required,uuid"`
	VehicleType    VehicleType    `json:"vehicle_type" validate:"required,oneof=1 2"`
	Mode           AssignmentMode `json:"mode" validate:"required,oneof=1 2"`
	SpotID         string         `json:"spot_id" validate:"omitempty,uuid"` // required when mode=USER_SELECTED
}

// CreateReservationResponse is the HTTP response for a created reservation
type CreateReservationResponse struct {
	ReservationID string            `json:"reservation_id"`
	SpotID        string            `json:"spot_id"`
	SpotCode      string            `json:"spot_code"`
	FloorNumber   int               `json:"floor_number"`
	Status        ReservationStatus `json:"status"`
	QRCodeURL     string            `json:"qr_code_url"`
}

// GetReservationResponse is the HTTP response for fetching a reservation
type GetReservationResponse struct {
	ReservationID  string            `json:"reservation_id"`
	DriverID       string            `json:"driver_id"`
	SpotID         string            `json:"spot_id"`
	SpotCode       string            `json:"spot_code"`
	FloorNumber    int               `json:"floor_number"`
	VehicleType    VehicleType       `json:"vehicle_type"`
	AssignmentMode AssignmentMode    `json:"assignment_mode"`
	Status         ReservationStatus `json:"status"`
	ConfirmedAt    *time.Time        `json:"confirmed_at,omitempty"`
	ExpiresAt      *time.Time        `json:"expires_at,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}
