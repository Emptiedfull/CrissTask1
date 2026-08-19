package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DbPool = pgxpool.Pool

const swaggerUIPage = `<!DOCTYPE html>
<html>
<head>
  <title>Inventory API Docs</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({ url: "/openapi.yaml", dom_id: "#swagger-ui" });
  </script>
</body>
</html>`

func main() {
	fmt.Println("starting up")
	ctx := context.Background()
	db, err := pgxpool.New(ctx, getEnv("DATABASE_URL", "postgres://..."))

	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("could not reach database: %v", err)
	}

	err = setupSchema(ctx, db)
	if err != nil {
		log.Fatal("unablke to srtup schema", err)
	}
	log.Println("db connected")

	controller := Controller{
		conns: make(map[*websocket.Conn]bool),
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /ws", handleWS(&controller))

	mux.HandleFunc("GET /inventory/analytics", AuthMiddleware(HandleAnalRes(db)))

	mux.HandleFunc("POST /auth/register", HandleRegister(db))
	mux.HandleFunc("POST /auth/login", HandleLogin(db))

	mux.HandleFunc("GET /inventory", AuthMiddleware(ListInventoryHandler(db)))
	mux.HandleFunc("GET /inventory/{id}", AuthMiddleware(GetInventoryItemHandler(db)))
	mux.HandleFunc("POST /inventory", RequireRole(Admin, CreateInventoryItemHandler(db)))

	mux.HandleFunc("DELETE /inventory/{id}", RequireRole(Admin, DeleteInventoryItemHandler(db)))
	mux.HandleFunc("POST /inventory/{id}/checkout", AuthMiddleware(CheckoutHandler(db, &controller)))
	mux.HandleFunc("POST /inventory/{id}/checkin", AuthMiddleware(CheckinHandler(db, &controller)))

	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "openapi.yaml")
	})

	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(swaggerUIPage))
	})

	fmt.Println("serving ")
	addr := ":" + getEnv("PORT", "8080")
	log.Fatal(http.ListenAndServe(addr, mux))
	fmt.Println("server up")

}
