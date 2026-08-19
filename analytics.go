package main

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type TopCheckedOutItem struct {
	ItemID        uuid.UUID `db:"item_id" json:"item_id"`
	Name          string    `db:"name" json:"name"`
	CheckoutCount int64     `db:"checkout_count" json:"checkout_count"`
}

type AnalRes struct {
	TopCheckedOutItem []TopCheckedOutItem `json:"top_checkout"`
	LowStock          []Item              `json:"low_stock"`
}

func HandleAnalRes(db *DbPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		topRows, err := db.Query(ctx, `
			SELECT i.id AS item_id, i.name AS name, COUNT(*) AS checkout_count
			FROM transactions t
			JOIN inventory i ON i.id = t.item_id
			WHERE t.action = $1
			GROUP BY i.id, i.name
			ORDER BY checkout_count DESC
			LIMIT 3
		`, checkout)

		if err != nil {
			writeError(w, http.StatusInternalServerError, "cant find")
			return
		}

		top, err := pgx.CollectRows(topRows, pgx.RowToStructByName[TopCheckedOutItem])
		if err != nil {
			writeError(w, http.StatusInternalServerError, "invalid db response")
			return
		}

		lowRows, err := db.Query(ctx, `
			SELECT `+itemCols+`
			FROM inventory
			WHERE available_quantity < 2
			ORDER BY available_quantity ASC
		`)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		low, err := pgx.CollectRows(lowRows, pgx.RowToStructByName[Item])
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read low stock")
			return
		}

		writeJSON(w, http.StatusOK, AnalRes{TopCheckedOutItem: top, LowStock: low})

	}
}
