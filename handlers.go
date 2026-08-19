package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

type Req_Register struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Role     MemRole
}

type Req_Login struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type Req_CreateItem struct {
	Name          string `json:"name"`
	Category      string `json:"category"`
	TotalQuantity int    `json:"total_quantity"`
}

type Req_UpdateItem struct {
	Name          *string `json:"name"`
	TotalQuantity *int    `json:"total_quantity"`
}

type Req_StockChange struct {
	Quantity int `json:"quantity"`
}

func HandleRegister(db *DbPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req_Register
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" || req.Password == "" {
			writeError(w, http.StatusBadRequest, "name and password are required")
			return
		}

		user, err := CreateUser(context.Background(), db, req.Name, req.Password, req.Role)
		if err != nil {
			fmt.Println(err)
			writeError(w, http.StatusBadRequest, "could not create user")
			return
		}
		writeJSON(w, http.StatusCreated, user)
	}
}

func HandleLogin(db *DbPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req_Login
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		user, err := GetUserByName(context.Background(), db, req.Name)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid name or password")
			return
		}
		if !checkPassword(req.Password, user.HashedPass) {
			writeError(w, http.StatusUnauthorized, "invalid name or password")
			return
		}

		token, err := GenJWT(user.ID, user.Role)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user})
	}
}

func ListInventoryHandler(db *DbPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := ListItems(context.Background(), db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database go bad bad")
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func GetInventoryItemHandler(db *DbPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		item, err := GetItemByID(context.Background(), db, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func CreateInventoryItemHandler(db *DbPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req_CreateItem
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		item, err := CreateItem(context.Background(), db, req.Name, req.Category, req.TotalQuantity)
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not create item")
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}
func DeleteInventoryItemHandler(db *DbPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		if err := DelItem(context.Background(), db, id); err != nil {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func CheckoutHandler(db *DbPool, hub *Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		itemID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		var req Req_StockChange
		if err := decodeJSON(r, &req); err != nil || req.Quantity <= 0 {
			req.Quantity = 1
		}

		claims := getClaims(r)
		item, err := CheckOut(context.Background(), db, itemID, claims.UserID, req.Quantity)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		hub.Broadcast("stock_updated", item)

		writeJSON(w, http.StatusOK, item)
	}
}

func CheckinHandler(db *DbPool, hub *Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		itemID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		var req Req_StockChange
		if err := decodeJSON(r, &req); err != nil || req.Quantity <= 0 {
			req.Quantity = 1
		}

		claims := getClaims(r)
		item, err := CheckIn(context.Background(), db, itemID, claims.UserID, req.Quantity)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		hub.Broadcast("stock_updated", item)

		writeJSON(w, http.StatusOK, item)
	}
}
