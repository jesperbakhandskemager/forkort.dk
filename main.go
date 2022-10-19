package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"

	"github.com/BurntSushi/toml"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
)

var tmpl = template.Must(template.ParseFiles("./sites/index.html"))
var statsTmpl = template.Must(template.ParseFiles("./sites/stats.html"))
var successTmpl = template.Must(template.ParseFiles("./sites/success.html"))

type SuccessShortend struct {
	LongLink  string
	ShortLink string
	Success   bool
	Error     bool
}

// GenerateRandomString returns a securely generated random string.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomString(n int) (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret), nil
}

func HandleStats(w http.ResponseWriter, r *http.Request) {
	var TotalSaved string
	query := `SELECT * FROM TotalSaved`
	err := db.QueryRow(query).Scan(&TotalSaved)
	if err != nil {
		statsTmpl.Execute(w, TotalSaved)
		return
	}
	statsTmpl.Execute(w, TotalSaved)
}

func AboutPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./sites/about.html")
}
func UnshortenHandler(w http.ResponseWriter, r *http.Request) {
	token := mux.Vars(r)["token"]
	var unshortenedLink string
	query := `SELECT oldLink FROM redirects where newLink = BINARY ?`
	err := db.QueryRow(query, token).Scan(&unshortenedLink)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "404 not found")
		return
	}
	http.Redirect(w, r, unshortenedLink, http.StatusSeeOther)
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		tmpl.Execute(w, nil)
		return
	}

	link := r.FormValue("userLink")
	println(link)
	u, err := url.ParseRequestURI(string(link))
	if err != nil {
		tmpl.Execute(w, struct{ Error bool }{Error: true})
		return
	}

	// If no errors Occured
	s := SuccessShortend{
		LongLink:  u.String(),
		ShortLink: "",
		Success:   false,
		Error:     false,
	}

	var token string
	query := `SELECT newLink FROM redirects where oldLink = ?`
	err = db.QueryRow(query, string(link)).Scan(&token)
	if err == nil {
		s.ShortLink = "forkort.dk/" + token
		successTmpl.Execute(w, s)
		return
	}

CREATE_TOKEN:
	token, err = GenerateRandomString(4)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Internal Server Error")
		return
	}
	query = `SELECT newLink FROM redirects where newLink = BINARY ?`
	err = db.QueryRow(query, token).Scan(&token)
	if err == nil {
		goto CREATE_TOKEN
	}
	s.ShortLink = "forkort.dk/" + token
	_, err = db.Exec(`INSERT INTO redirects(oldLink, newLink) VALUES (?, ?)`, string(link), token)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		tmpl.Execute(w, struct{ Error bool }{Error: true})
		return
	}
	successTmpl.Execute(w, s)
}

func UnshortenApi(w http.ResponseWriter, r *http.Request) {
	token := mux.Vars(r)["token"]
	var unshortenedLink string
	query := `SELECT oldLink FROM redirects where newLink = BINARY ?`
	err := db.QueryRow(query, token).Scan(&unshortenedLink)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "404 not found")
		return
	}
	w.Write([]byte(unshortenedLink))
}

func ShortenApi(w http.ResponseWriter, r *http.Request) {
	link, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	u, err := url.ParseRequestURI(string(link))
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid URL")
		return
	}
	log.Println(u)
	log.Println(string(link))
	var token string
	query := `SELECT newLink FROM redirects where oldLink = ?`
	err = db.QueryRow(query, string(link)).Scan(&token)
	if err == nil {
		fmt.Fprintf(w, "forkort.dk/"+token)
		return
	}

CREATE_TOKEN:
	token, err = GenerateRandomString(4)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Internal Server Error")
	}
	query = `SELECT newLink FROM redirects where newLink = BINARY ?`
	err = db.QueryRow(query, token).Scan(&token)
	if err == nil {
		goto CREATE_TOKEN
	}
	_, err = db.Exec(`INSERT INTO redirects(oldLink, newLink) VALUES (?, ?)`, string(link), token)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Bad request")
		return
	}
	fmt.Fprintf(w, "forkort.dk/"+token)
}

var db *sql.DB

type Config struct {
	MYSQL_DB   string
	MYSQL_USER string
	MYSQL_PASS string
	MYSQL_HOST string
}

func main() {
	var cfg Config
	file, err := os.ReadFile("./config.toml")
	if err != nil {
		log.Fatal("you need a config.toml file")
	}
	err = toml.Unmarshal(file, &cfg)
	if err != nil {
		panic(err)
	}

	db, err = sql.Open("mysql", cfg.MYSQL_USER+":@("+cfg.MYSQL_HOST+")/"+cfg.MYSQL_DB+"?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	log.Println("Database connection established")

	router := mux.NewRouter()
	router.HandleFunc("/", IndexHandler)
	router.HandleFunc("/stats", HandleStats)
	router.HandleFunc("/about", AboutPage)
	router.HandleFunc("/api/shorten", ShortenApi)
	router.HandleFunc("/api/unshorten/{token}", UnshortenApi)
	router.HandleFunc(`/{token}`, UnshortenHandler)
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static", http.FileServer(http.Dir("./assets"))))

	http.ListenAndServe(":8080", router)
}
