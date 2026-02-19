package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Estructura para la tabla de Supabase
type URLMapping struct {
	Code    string `json:"code"`
	LongURL string `json:"long_url"`
}

var (
	supabaseURL string
	supabaseKey string
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const codeLen = 6

func randomSlug() string {
	b := make([]byte, codeLen)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fallback extremadamente improbable, pero por seguridad idiomática
			b[i] = charset[0]
			continue
		}
		b[i] = charset[num.Int64()]
	}
	return string(b)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// POST /shorten
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

	if !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
		body.URL = "https://" + body.URL
	}

	// Validación estricta de URL
	u, err := url.ParseRequestURI(body.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		http.Error(w, "URL con formato o protocolo no permitido", http.StatusBadRequest)
		return
	}

	if u.Host == "" || !strings.Contains(u.Host, ".") {
		http.Error(w, "Host de URL inválido", http.StatusBadRequest)
		return
	}

	// Generar código único verificando en Supabase
	var code string
	for {
		code = randomSlug()
		// Verificar si existe usando REST (SELECT)
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/rest/v1/urls?code=eq.%s&select=code", supabaseURL, code), nil)
		req.Header.Set("apikey", supabaseKey)
		req.Header.Set("Authorization", "Bearer "+supabaseKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("Error verificando código: %v", err)
			continue
		}

		var results []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&results)
		resp.Body.Close()

		if len(results) == 0 {
			break
		}
	}

	// Insertar en Supabase via REST
	mapping := URLMapping{Code: code, LongURL: body.URL}
	jsonData, _ := json.Marshal(mapping)

	req, _ := http.NewRequest("POST", supabaseURL+"/rest/v1/urls", bytes.NewBuffer(jsonData))
	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || (resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK) {
		http.Error(w, "Error guardando en Supabase", http.StatusInternalServerError)
		return
	}
	resp.Body.Close()

	var shortURL string
	baseURL := os.Getenv("BASE_URL")
	if baseURL != "" {
		baseURL = strings.TrimSuffix(baseURL, "/")
		shortURL = baseURL + "/" + code
	} else {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		shortURL = scheme + "://" + r.Host + "/" + code
	}

	w.Header().Set("Content-Type", "application/json")
	// Quitar el protocolo para fines estéticos si el usuario lo prefiere
	displayURL := shortURL
	displayURL = strings.TrimPrefix(displayURL, "https://")
	displayURL = strings.TrimPrefix(displayURL, "http://")

	json.NewEncoder(w).Encode(map[string]string{
		"short_url":   shortURL,
		"display_url": displayURL,
	})
}

// GET /{code}
func redirectHandler(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/")
	if code == "" {
		http.ServeFile(w, r, "./static/index.html")
		return
	}

	// Buscar en Supabase via REST (SELECT)
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/rest/v1/urls?code=eq.%s&select=long_url", supabaseURL, code), nil)
	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "Error de conexión con Supabase", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var results []URLMapping
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil || len(results) == 0 {
		http.Error(w, "URL no encontrada", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, results[0].LongURL, http.StatusFound)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; connect-src 'self' https://djhxciacebtabtlwlgwx.supabase.co")
		next.ServeHTTP(w, r)
	})
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	supabaseURL = os.Getenv("SUPABASE_URL")
	supabaseKey = os.Getenv("SUPABASE_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		log.Fatal("SUPABASE_URL and SUPABASE_KEY are required")
	}

	port := getEnv("PORT", "8080")

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
	mux.HandleFunc("/shorten", shortenHandler)
	mux.HandleFunc("/", redirectHandler)

	// Aplicar middleware de cabeceras de seguridad
	handler := securityHeaders(mux)

	log.Printf("Servidor REST corriendo en el puerto %s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
