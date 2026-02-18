package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const codeLen = 6

func randomCode() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, codeLen)
	for i := range b {
		b[i] = charset[r.Intn(len(charset))]
	}
	return string(b)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func initDB() {
	var err error
	dbPath := getEnv("DB_PATH", "./urls.db")
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal("Error abriendo la base de datos:", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS urls (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		code      TEXT UNIQUE NOT NULL,
		long_url  TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatal("Error creando la tabla:", err)
	}
}

// POST /shorten  body: {"url": "https://..."}
func shortenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		http.Error(w, "URL inválida", http.StatusBadRequest)
		return
	}

	// Agregar esquema si falta
	if !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
		body.URL = "https://" + body.URL
	}

	// Generar código único
	var code string
	for {
		code = randomCode()
		var existing string
		err := db.QueryRow("SELECT code FROM urls WHERE code = ?", code).Scan(&existing)
		if err == sql.ErrNoRows {
			break
		}
	}

	_, err := db.Exec("INSERT INTO urls (code, long_url) VALUES (?, ?)", code, body.URL)
	if err != nil {
		http.Error(w, "Error guardando la URL", http.StatusInternalServerError)
		return
	}

	var shortURL string
	baseURL := os.Getenv("BASE_URL")
	if baseURL != "" {
		// Asegurarse de que no termine con slash
		baseURL = strings.TrimSuffix(baseURL, "/")
		shortURL = baseURL + "/" + code
	} else {
		// Fallback dinámico
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		shortURL = scheme + "://" + r.Host + "/" + code
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"short_url": shortURL})
}

// GET /{code}
func redirectHandler(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/")
	if code == "" {
		http.ServeFile(w, r, "./static/index.html")
		return
	}

	var longURL string
	err := db.QueryRow("SELECT long_url FROM urls WHERE code = ?", code).Scan(&longURL)
	if err == sql.ErrNoRows {
		http.Error(w, "URL no encontrada", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Error interno", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, longURL, http.StatusFound)
}

func main() {
	initDB()
	defer db.Close()

	port := getEnv("PORT", "8080")

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/shorten", shortenHandler)
	http.HandleFunc("/", redirectHandler)

	log.Printf("Servidor corriendo en el puerto %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
