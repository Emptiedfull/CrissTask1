package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemRole int

const (
	Admin MemRole = iota
	TeamLead
	Member
	Adhoc
)

type ItemCat int

const (
	Misc ItemCat = iota
	Sensors
	Tools
	Electronics
	Mechanical
)

type LogAction int

const (
	checkin LogAction = iota
	checkout
)

type User struct {
	Name       string    `db:"name"`
	ID         uuid.UUID `db:"id"`
	HashedPass string    `db:"hashed_password"`
	Role       MemRole   `db:"role"`
}

type Item struct {
	ID                uuid.UUID `db:"id"`
	Name              string    `db:"name"`
	SKU               string    `db:"sku"`
	Category          string    `db:"category"`
	TotalQuantity     int       `db:"total_quantity"`
	AvailableQuantity int       `db:"available_quantity"`
}

type Transaction struct {
	ID        uuid.UUID `db:"id"`
	ItemID    uuid.UUID `db:"item_id"`
	UserID    uuid.UUID `db:"user_id"`
	Action    LogAction `db:"action"`
	Quantity  int       `db:"quantity"`
	Timestamp time.Time `db:"timestamp"`
}

func CreateUser(ctx context.Context, db *DbPool, name, password string, role MemRole) (User, error) {
	hashed, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}

	rows, err := db.Query(ctx,
		`INSERT INTO users (name, hashed_password, role) VALUES ($1, $2, $3)
		 RETURNING id, name, hashed_password, role`,
		name, hashed, role,
	)
	if err != nil {

		return User{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[User])
}

func GetUserByID(ctx context.Context, db *DbPool, id uuid.UUID) (User, error) {
	rows, err := db.Query(ctx, "SELECT id, name, hashed_password, role FROM users WHERE id = $1", id)
	if err != nil {
		return User{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[User])
}
func GetUserByName(ctx context.Context, db *DbPool, name string) (User, error) {
	rows, err := db.Query(ctx, "SELECT id, name, hashed_password, role FROM users WHERE name = $1", name)
	if err != nil {
		return User{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[User])
}

func ListUsers(ctx context.Context, db *DbPool) ([]User, error) {
	rows, err := db.Query(ctx, "SELECT id, name, hashed_password, role FROM users")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[User])
}

func UpdateRoleUser(ctx context.Context, db *DbPool, role MemRole, id uuid.UUID) (User, error) {
	rows, err := db.Query(ctx,
		"UPDATE users SET role = $1 WHERE id = $2 RETURNING id, name, hashed_password, role",
		role, id,
	)
	if err != nil {
		return User{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[User])
}

func DeleteUser(ctx context.Context, id uuid.UUID, db *DbPool) error {
	tag, err := db.Exec(ctx, "DELETE FROM users WHERE id=$1", id)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("No user found")
	}

	return nil
}

const itemCols = "id, name, sku, category, total_quantity, available_quantity"

func CreateItem(ctx context.Context, db *DbPool, name string, category string, totalQty int) (Item, error) {
	var count int
	err := db.QueryRow(ctx, "SELECT COUNT(*) FROM inventory WHERE category = $1", category).Scan(&count)
	if err != nil {
		return Item{}, err
	}
	sku := generateSKU(category, count)

	rows, err := db.Query(ctx,
		`INSERT INTO inventory (name, sku, category, total_quantity, available_quantity)
		 VALUES ($1, $2, $3, $4, $4)
		 RETURNING `+itemCols,
		name, sku, category, totalQty,
	)
	if err != nil {
		return Item{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Item])
}

func GetItemByID(ctx context.Context, db *DbPool, id uuid.UUID) (Item, error) {
	rows, err := db.Query(ctx, "SELECT "+itemCols+" FROM inventory WHERE id = $1", id)
	if err != nil {
		return Item{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Item])
}

func ListItems(ctx context.Context, db *DbPool) ([]Item, error) {
	rows, err := db.Query(ctx, "SELECT "+itemCols+" FROM inventory")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Item])
}

func DelItem(ctx context.Context, db *DbPool, id uuid.UUID) error {
	tag, err := db.Exec(ctx, "DELETE FROM inventory WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no item with given ID")
	}
	return nil
}

func CheckOut(ctx context.Context, db *DbPool, ItemID uuid.UUID, UserID uuid.UUID, qty int) (Item, error) {
	return UpdateStock(ctx, db, ItemID, UserID, qty, checkout)
}

func CheckIn(ctx context.Context, db *DbPool, ItemID uuid.UUID, UserID uuid.UUID, qty int) (Item, error) {
	return UpdateStock(ctx, db, ItemID, UserID, qty, checkin)
}

func UpdateStock(ctx context.Context, db *DbPool, itemID uuid.UUID, userID uuid.UUID, qty int, action LogAction) (Item, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return Item{}, err
	}

	defer tx.Rollback(ctx)

	var total, available int
	err = tx.QueryRow(ctx,
		"SELECT total_quantity, available_quantity FROM inventory WHERE id = $1 FOR UPDATE",
		itemID,
	).Scan(&total, &available)
	if err != nil {
		return Item{}, err
	}

	updated := available
	if action == checkin {
		updated += qty
		if updated > total {
			updated = total
		}
	} else if action == checkout {
		if available < qty {
			return Item{}, fmt.Errorf("Not enough items available, wanted: %d had: %d", qty, available)
		}

		updated -= qty
	}

	rows, err := tx.Query(ctx,
		"UPDATE inventory SET available_quantity = $1 WHERE id = $2 RETURNING "+itemCols,
		updated, itemID,
	)
	if err != nil {
		return Item{}, err
	}

	item, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Item])
	if err != nil {
		return Item{}, err
	}

	_, err = tx.Exec(ctx,
		"INSERT INTO transactions (item_id, user_id, action, quantity) VALUES ($1, $2, $3, $4)",
		itemID, userID, action, qty,
	)
	if err != nil {
		return Item{}, err
	}

	return item, tx.Commit(ctx)
}

func GetTransactionLog(ctx context.Context, db *DbPool) ([]Transaction, error) {
	rows, err := db.Query(ctx,
		"SELECT id, item_id, user_id, action, quantity, timestamp FROM transactions ORDER BY timestamp DESC",
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Transaction])
}

func GetTransactionsByItem(ctx context.Context, db *DbPool, id uuid.UUID) ([]Transaction, error) {
	rows, err := db.Query(ctx,
		"SELECT id, item_id, user_id, action, quantity, timestamp FROM transactions WHERE item_id = $1 ORDER BY timestamp DESC",
		id,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Transaction])
}

func setupSchema(ctx context.Context, db *pgxpool.Pool) error {
	cont, err := os.ReadFile("schema.sql")
	if err != nil {
		return err
	}

	_, err = db.Exec(ctx, string(cont))
	if err != nil {
		return err
	}

	return nil
}
