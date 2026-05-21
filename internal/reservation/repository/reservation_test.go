package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"parkir-pintar/services/reservation/internal/reservation/model"

	"github.com/alicebob/miniredis/v2"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testIdemKey       = "550e8400-e29b-41d4-a716-446655440000"
	testDriverID      = "660e8400-e29b-41d4-a716-446655440001"
	testSpotID        = "770e8400-e29b-41d4-a716-446655440002"
	testReservationID = "880e8400-e29b-41d4-a716-446655440003"
)

// newMocks returns a pgxmock pool, miniredis server, and the repo under test.
func newMocks(t *testing.T) (pgxmock.PgxPoolIface, *miniredis.Miniredis, *ReservationRepository) {
	t.Helper()

	db, err := pgxmock.NewPool()
	require.NoError(t, err)

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	return db, mr, &ReservationRepository{db: db, redis: rc}
}

// ── GetByIdempotencyKey ───────────────────────────────────────────────────────
// Scan order: id, spot_id, status, qr_code_url, spot_code, floor_number

func TestGetByIdempotencyKey_Found(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT r\.id`).
		WithArgs(testIdemKey).
		WillReturnRows(pgxmock.NewRows([]string{"id", "spot_id", "status", "qr_code_url", "spot_code", "floor_number"}).
			AddRow(testReservationID, testSpotID, model.ReservationStatusPendingPayment, "https://qr.example.com", "A1", 1))

	res, appErr := repo.GetByIdempotencyKey(context.Background(), testIdemKey)

	require.Nil(t, appErr)
	require.NotNil(t, res)
	assert.Equal(t, testReservationID, res.ID)
	assert.Equal(t, testSpotID, res.SpotID)
	assert.Equal(t, model.ReservationStatusPendingPayment, res.Status)
	assert.Equal(t, "https://qr.example.com", res.QRCodeURL)
	assert.Equal(t, "A1", res.SpotCode)
	assert.Equal(t, 1, res.FloorNumber)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestGetByIdempotencyKey_NotFound(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT r\.id`).
		WithArgs(testIdemKey).
		WillReturnRows(pgxmock.NewRows([]string{"id", "spot_id", "status", "qr_code_url", "spot_code", "floor_number"}))

	res, appErr := repo.GetByIdempotencyKey(context.Background(), testIdemKey)

	require.Nil(t, appErr)
	assert.Nil(t, res)
	assert.NoError(t, db.ExpectationsWereMet())
}

// ── GetByID ───────────────────────────────────────────────────────────────────
// Scan order: id, idempotency_key, driver_id, spot_id, vehicle_type,
//             assignment_mode, status, confirmed_at, expires_at, created_at,
//             spot_code, floor_number

func TestGetByID_Found(t *testing.T) {
	db, _, repo := newMocks(t)

	now := time.Now()
	db.ExpectQuery(`SELECT r\.id`).
		WithArgs(testReservationID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "idempotency_key", "driver_id", "spot_id", "vehicle_type",
			"assignment_mode", "status", "confirmed_at", "expires_at", "created_at",
			"spot_code", "floor_number",
		}).AddRow(
			testReservationID, testIdemKey, testDriverID, testSpotID,
			model.VehicleTypeCar, model.AssignmentModeSystemAssigned,
			model.ReservationStatusConfirmed, nil, nil, now,
			"B3", 2,
		))

	res, appErr := repo.GetByID(context.Background(), testReservationID)

	require.Nil(t, appErr)
	assert.Equal(t, testReservationID, res.ID)
	assert.Equal(t, testDriverID, res.DriverID)
	assert.Equal(t, "B3", res.SpotCode)
	assert.Equal(t, 2, res.FloorNumber)
	assert.Equal(t, model.ReservationStatusConfirmed, res.Status)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestGetByID_NotFound(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT r\.id`).
		WithArgs(testReservationID).
		WillReturnRows(pgxmock.NewRows([]string{"id"}))

	_, appErr := repo.GetByID(context.Background(), testReservationID)

	require.NotNil(t, appErr)
	assert.Equal(t, "not_found", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

// ── HasActiveReservation ──────────────────────────────────────────────────────
// Scan order: count

func TestHasActiveReservation_True(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT COUNT`).
		WithArgs(testDriverID, pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))

	has, appErr := repo.HasActiveReservation(context.Background(), testDriverID)

	require.Nil(t, appErr)
	assert.True(t, has)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestHasActiveReservation_False(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT COUNT`).
		WithArgs(testDriverID, pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

	has, appErr := repo.HasActiveReservation(context.Background(), testDriverID)

	require.Nil(t, appErr)
	assert.False(t, has)
	assert.NoError(t, db.ExpectationsWereMet())
}

// ── GetAvailableSpot ──────────────────────────────────────────────────────────
// Scan order: id, floor_number, spot_code, vehicle_type, status

func TestGetAvailableSpot_Found(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT id`).
		WithArgs(model.SpotStatusAvailable, model.VehicleTypeCar).
		WillReturnRows(pgxmock.NewRows([]string{"id", "floor_number", "spot_code", "vehicle_type", "status"}).
			AddRow(testSpotID, 1, "A1", model.VehicleTypeCar, model.SpotStatusAvailable))

	spot, appErr := repo.GetAvailableSpot(context.Background(), model.VehicleTypeCar)

	require.Nil(t, appErr)
	assert.Equal(t, testSpotID, spot.ID)
	assert.Equal(t, "A1", spot.SpotCode)
	assert.Equal(t, 1, spot.FloorNumber)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestGetAvailableSpot_NotFound(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT id`).
		WithArgs(model.SpotStatusAvailable, model.VehicleTypeCar).
		WillReturnRows(pgxmock.NewRows([]string{"id", "floor_number", "spot_code", "vehicle_type", "status"}))

	_, appErr := repo.GetAvailableSpot(context.Background(), model.VehicleTypeCar)

	require.NotNil(t, appErr)
	assert.Equal(t, "no_spots_available", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

// ── GetSpotByID ───────────────────────────────────────────────────────────────
// Scan order: id, floor_number, spot_code, vehicle_type, status

func TestGetSpotByID_Found(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT id`).
		WithArgs(testSpotID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "floor_number", "spot_code", "vehicle_type", "status"}).
			AddRow(testSpotID, 3, "C5", model.VehicleTypeCar, model.SpotStatusAvailable))

	spot, appErr := repo.GetSpotByID(context.Background(), testSpotID)

	require.Nil(t, appErr)
	assert.Equal(t, testSpotID, spot.ID)
	assert.Equal(t, "C5", spot.SpotCode)
	assert.Equal(t, 3, spot.FloorNumber)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestGetSpotByID_NotFound(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT id`).
		WithArgs(testSpotID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "floor_number", "spot_code", "vehicle_type", "status"}))

	_, appErr := repo.GetSpotByID(context.Background(), testSpotID)

	require.NotNil(t, appErr)
	assert.Equal(t, "not_found", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

// ── CreateReservationAndLockSpot ──────────────────────────────────────────────

func TestCreateReservationAndLockSpot_Success(t *testing.T) {
	db, _, repo := newMocks(t)

	now := time.Now()
	db.ExpectBegin()
	db.ExpectQuery(`INSERT INTO reservations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"created_at"}).AddRow(now))
	db.ExpectExec(`UPDATE spots`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	created, appErr := repo.CreateReservationAndLockSpot(context.Background(), &model.Reservation{
		ID:             testReservationID,
		IdempotencyKey: testIdemKey,
		DriverID:       testDriverID,
		SpotID:         testSpotID,
		VehicleType:    model.VehicleTypeCar,
		AssignmentMode: model.AssignmentModeSystemAssigned,
		QRCodeURL:      "https://qr.example.com",
	})

	require.Nil(t, appErr)
	assert.Equal(t, testReservationID, created.ID)
	assert.Equal(t, model.ReservationStatusPendingPayment, created.Status)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestCreateReservationAndLockSpot_InsertFails(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectBegin()
	db.ExpectQuery(`INSERT INTO reservations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(fmt.Errorf("duplicate key"))
	db.ExpectRollback()

	_, appErr := repo.CreateReservationAndLockSpot(context.Background(), &model.Reservation{
		ID:             testReservationID,
		IdempotencyKey: testIdemKey,
		DriverID:       testDriverID,
		SpotID:         testSpotID,
		VehicleType:    model.VehicleTypeCar,
		AssignmentMode: model.AssignmentModeSystemAssigned,
	})

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestCreateReservationAndLockSpot_UpdateFails(t *testing.T) {
	db, _, repo := newMocks(t)

	now := time.Now()
	db.ExpectBegin()
	db.ExpectQuery(`INSERT INTO reservations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"created_at"}).AddRow(now))
	db.ExpectExec(`UPDATE spots`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(fmt.Errorf("update failed"))
	db.ExpectRollback()

	_, appErr := repo.CreateReservationAndLockSpot(context.Background(), &model.Reservation{
		ID:             testReservationID,
		IdempotencyKey: testIdemKey,
		DriverID:       testDriverID,
		SpotID:         testSpotID,
		VehicleType:    model.VehicleTypeCar,
		AssignmentMode: model.AssignmentModeSystemAssigned,
	})

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

// ── AcquireSpotLock / ReleaseSpotLock ─────────────────────────────────────────

func TestAcquireSpotLock_Success(t *testing.T) {
	_, mr, repo := newMocks(t)

	ok, appErr := repo.AcquireSpotLock(context.Background(), testSpotID, testDriverID)

	require.Nil(t, appErr)
	assert.True(t, ok)
	val, err := mr.Get(fmt.Sprintf("%s%s", spotLockKeyPrefix, testSpotID))
	require.NoError(t, err)
	assert.Equal(t, testDriverID, val)
}

func TestAcquireSpotLock_AlreadyLocked(t *testing.T) {
	_, _, repo := newMocks(t)

	ok, _ := repo.AcquireSpotLock(context.Background(), testSpotID, testDriverID)
	require.True(t, ok)

	ok, appErr := repo.AcquireSpotLock(context.Background(), testSpotID, "other-driver")
	require.Nil(t, appErr)
	assert.False(t, ok)
}

func TestReleaseSpotLock_Success(t *testing.T) {
	_, mr, repo := newMocks(t)

	_, _ = repo.AcquireSpotLock(context.Background(), testSpotID, testDriverID)

	appErr := repo.ReleaseSpotLock(context.Background(), testSpotID)
	require.Nil(t, appErr)

	val, _ := mr.Get(fmt.Sprintf("%s%s", spotLockKeyPrefix, testSpotID))
	assert.Equal(t, "", val)
}

func TestReleaseSpotLock_KeyNotExist(t *testing.T) {
	_, _, repo := newMocks(t)

	appErr := repo.ReleaseSpotLock(context.Background(), testSpotID)
	assert.Nil(t, appErr)
}

func TestAcquireRelease_LockCycle(t *testing.T) {
	_, _, repo := newMocks(t)

	ok, _ := repo.AcquireSpotLock(context.Background(), testSpotID, testDriverID)
	require.True(t, ok)

	_ = repo.ReleaseSpotLock(context.Background(), testSpotID)

	ok, appErr := repo.AcquireSpotLock(context.Background(), testSpotID, "driver-2")
	require.Nil(t, appErr)
	assert.True(t, ok)
}

// ── DB error paths ────────────────────────────────────────────────────────────

func TestGetByIdempotencyKey_DBError(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT r\.id`).
		WithArgs(testIdemKey).
		WillReturnError(fmt.Errorf("connection refused"))

	_, appErr := repo.GetByIdempotencyKey(context.Background(), testIdemKey)

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestGetByID_DBError(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT r\.id`).
		WithArgs(testReservationID).
		WillReturnError(fmt.Errorf("connection refused"))

	_, appErr := repo.GetByID(context.Background(), testReservationID)

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestHasActiveReservation_DBError(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT COUNT`).
		WithArgs(testDriverID, pgxmock.AnyArg()).
		WillReturnError(fmt.Errorf("connection refused"))

	_, appErr := repo.HasActiveReservation(context.Background(), testDriverID)

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestGetAvailableSpot_DBError(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT id`).
		WithArgs(model.SpotStatusAvailable, model.VehicleTypeCar).
		WillReturnError(fmt.Errorf("connection refused"))

	_, appErr := repo.GetAvailableSpot(context.Background(), model.VehicleTypeCar)

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestGetSpotByID_DBError(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectQuery(`SELECT id`).
		WithArgs(testSpotID).
		WillReturnError(fmt.Errorf("connection refused"))

	_, appErr := repo.GetSpotByID(context.Background(), testSpotID)

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestCreateReservationAndLockSpot_BeginFails(t *testing.T) {
	db, _, repo := newMocks(t)

	db.ExpectBegin().WillReturnError(fmt.Errorf("begin failed"))

	_, appErr := repo.CreateReservationAndLockSpot(context.Background(), &model.Reservation{
		IdempotencyKey: testIdemKey,
		DriverID:       testDriverID,
		SpotID:         testSpotID,
		VehicleType:    model.VehicleTypeCar,
		AssignmentMode: model.AssignmentModeSystemAssigned,
	})

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestCreateReservationAndLockSpot_CommitFails(t *testing.T) {
	db, _, repo := newMocks(t)

	now := time.Now()
	db.ExpectBegin()
	db.ExpectQuery(`INSERT INTO reservations`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"created_at"}).AddRow(now))
	db.ExpectExec(`UPDATE spots`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit().WillReturnError(fmt.Errorf("commit failed"))

	_, appErr := repo.CreateReservationAndLockSpot(context.Background(), &model.Reservation{
		ID:             testReservationID,
		IdempotencyKey: testIdemKey,
		DriverID:       testDriverID,
		SpotID:         testSpotID,
		VehicleType:    model.VehicleTypeCar,
		AssignmentMode: model.AssignmentModeSystemAssigned,
	})

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestAcquireSpotLock_RedisError(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)

	rc := redis.NewClient(&redis.Options{Addr: "localhost:1", DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = rc.Close() })

	repo := &ReservationRepository{db: db, redis: rc}

	_, appErr := repo.AcquireSpotLock(context.Background(), testSpotID, testDriverID)

	require.NotNil(t, appErr)
	assert.Equal(t, "redis_error", appErr.ErrorCode)
}

func TestReleaseSpotLock_RedisError(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)

	rc := redis.NewClient(&redis.Options{Addr: "localhost:1", DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = rc.Close() })

	repo := &ReservationRepository{db: db, redis: rc}

	appErr := repo.ReleaseSpotLock(context.Background(), testSpotID)

	require.NotNil(t, appErr)
	assert.Equal(t, "redis_error", appErr.ErrorCode)
}
