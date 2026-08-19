package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))
	t.Cleanup(pool.Close)

	require.NoError(t, setupSchema(ctx, pool))

	_, err = pool.Exec(ctx, "TRUNCATE transactions, inventory, users CASCADE")
	require.NoError(t, err)

	return pool
}

func TestLogin(t *testing.T) {
	db := setupTestDB(t)

	registerBody, _ := json.Marshal(Req_Register{Name: "gurt", Password: "secret123", Role: Admin})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(registerBody))
	rec := httptest.NewRecorder()
	HandleRegister(db)(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	loginBody, _ := json.Marshal(Req_Login{Name: "gurt", Password: "secret123"})
	req = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	rec = httptest.NewRecorder()
	HandleLogin(db)(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["token"])

	badBody, _ := json.Marshal(Req_Login{Name: "alice", Password: "wrong"})
	req = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(badBody))
	rec = httptest.NewRecorder()
	HandleLogin(db)(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCheckoutInsufficientStock(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	user, err := CreateUser(ctx, db, "bob", "password123", Member)
	require.NoError(t, err)

	item, err := CreateItem(ctx, db, "Drill", "Tools", 5)
	require.NoError(t, err)

	_, err = db.Exec(ctx, "UPDATE inventory SET available_quantity = 0 WHERE id = $1", item.ID)
	require.NoError(t, err)

	token, err := GenJWT(user.ID, user.Role)
	require.NoError(t, err)

	body, _ := json.Marshal(Req_StockChange{Quantity: 1})
	req := httptest.NewRequest(http.MethodPost, "/inventory/"+item.ID.String()+"/checkout", bytes.NewReader(body))
	req.SetPathValue("id", item.ID.String())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	AuthMiddleware(CheckoutHandler(db, &Controller{
		conns: make(map[*websocket.Conn]bool),
	}))(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestCheckoutSuccess(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	user, err := CreateUser(ctx, db, "carol", "password123", Member)
	require.NoError(t, err)

	item, err := CreateItem(ctx, db, "Hammer", "Tools", 5)
	require.NoError(t, err)

	token, err := GenJWT(user.ID, user.Role)
	require.NoError(t, err)

	body, _ := json.Marshal(Req_StockChange{Quantity: 2})
	req := httptest.NewRequest(http.MethodPost, "/inventory/"+item.ID.String()+"/checkout", bytes.NewReader(body))
	req.SetPathValue("id", item.ID.String())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	AuthMiddleware(CheckoutHandler(db, &Controller{
		conns: make(map[*websocket.Conn]bool),
	}))(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var updated Item
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, 3, updated.AvailableQuantity) // 5 - 2

	var txCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM transactions WHERE item_id = $1", item.ID).Scan(&txCount)
	require.NoError(t, err)
	assert.Equal(t, 1, txCount)
}
