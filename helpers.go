package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

func generateSKU(cat string, count int) string {
	var pre string

	switch cat {
	case "Tools":
		pre = "TOL"
	case "Misc":
		pre = "MSC"
	case "Sensors":
		pre = "SNR"
	case "Electronics":
		pre = "ELE"
	case "Mechanical":
		pre = "MCH"
	}

	return fmt.Sprintf("%s-%04d", pre, count+1)

}

func writeJSON(w http.ResponseWriter, status int, x interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(x)
}

func decodeJSON(r *http.Request, x interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(x)
}

func checkPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
