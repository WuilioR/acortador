package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var db *sql.DB

const (
// No necesitamos charset ni codeLen ahora
)

var adjectives = []string{
	"feliz", "rapido", "brillante", "genial", "valiente", "sabio", "fuerte", "amable", "calmado", "fresco",
	"rojo", "azul", "verde", "dorado", "plateado", "lindo", "epico", "super", "mega", "ultra",
}

var nouns = []string{
	"panda", "tigre", "leon", "aguila", "delfin", "perro", "gato", "lobo", "oso", "halcon",
	"sol", "luna", "estrella", "rio", "monte", "cielo", "mar", "bosque", "fuego", "rayo",
	"code", "byte", "pixel", "data", "bot", "red", "link", "web", "app", "sitio",
}

func randomSlug() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	adj := adjectives[r.Intn(len(adjectives))]
	noun := nouns[r.Intn(len(nouns))]
	num := r.Intn(1000) // 0-999
	return fmt.Sprintf("%s-%s-%d", adj, noun, num)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func initDB() {
	var err error
	connStr := getEnv("DATABASE_URL", "")
	if connStr == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error opening database connection:", err)
	}

	// Verify connection
	if err = db.Ping(); err != nil {
		log.Fatal("Error connecting to the database:", err)
	}

	// Create table if not exists (PostgreSQL syntax)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS urls (
		id SERIAL PRIMARY KEY,
		code TEXT UNIQUE NOT NULL,
		long_url TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatal("Error creating table:", err)
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

	// Generar código único (slug)
	var code string
	for {
		code = randomSlug()
		var existing string
		err := db.QueryRow("SELECT code FROM urls WHERE code = $1", code).Scan(&existing)
		if err == sql.ErrNoRows {
			break
		}
	}

	_, err := db.Exec("INSERT INTO urls (code, long_url) VALUES ($1, $2)", code, body.URL)
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
	err := db.QueryRow("SELECT long_url FROM urls WHERE code = $1", code).Scan(&longURL)
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
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
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
