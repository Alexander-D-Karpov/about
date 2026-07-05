package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const redirectURI = "http://127.0.0.1:8888/callback"

func openBrowser(u string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, u)
	_ = exec.Command(cmd, args...).Start()
}

func main() {
	_ = godotenv.Load()

	id := os.Getenv("SPOTIFY_CLIENT_ID")
	secret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if id == "" || secret == "" {
		log.Fatal("set SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET (in .env or env)")
	}

	scopes := "user-library-read user-read-currently-playing"
	authURL := "https://accounts.spotify.com/authorize?" + url.Values{
		"client_id":     {id},
		"response_type": {"code"},
		"redirect_uri":  {redirectURI},
		"scope":         {scopes},
	}.Encode()

	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: "127.0.0.1:8888"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "no code: "+r.URL.Query().Get("error"), http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, "Got it. You can close this tab.")
		codeCh <- code
	})
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	fmt.Println("Opening browser. If it doesn't open, visit:\n" + authURL)
	openBrowser(authURL)

	code := <-codeCh
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	req, _ := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(id+":"+secret)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		log.Fatalf("token exchange failed %d: %s", resp.StatusCode, body)
	}

	var out struct {
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nSPOTIFY_REFRESH_TOKEN=%s\n", out.RefreshToken)
	fmt.Printf("# granted scopes: %s\n", out.Scope)
}
