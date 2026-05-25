package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	bus := NewBus()
	engine := NewEngine(bus)

	webhookCh := bus.Subscribe("github-webhook")
	codeReviewCh := bus.Subscribe("grp.code-review")
	reportCh := bus.Subscribe("grp.report")

	engine.RegisterRoute("github-webhook", []Plugin{
		&GitHubWebhookPlugin{},
	})
	engine.RegisterRoute("grp.code-review", []Plugin{
		&ReviewPlugin{},
	})
	engine.RegisterRoute("grp.report", []Plugin{
		&ReportPlugin{},
	})

	go func() {
		for env := range webhookCh {
			engine.Route(env)
		}
	}()
	go func() {
		for env := range codeReviewCh {
			engine.Route(env)
		}
	}()
	go func() {
		for env := range reportCh {
			engine.Route(env)
		}
	}()

	http.HandleFunc("/webhook/github", handleGitHubWebhook(bus))
	http.HandleFunc("/pr/review", handleReviewRequest(bus))

	fmt.Println("Server started on :8080")
	http.ListenAndServe(":8080", nil)
}

func handleGitHubWebhook(bus *Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body error", 400)
			return
		}
		defer r.Body.Close()

		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid json", 400)
			return
		}

		env := &Envelop{
			Sender:   "http",
			Receiver: "github-webhook",
			Intent:   "webhook",
			Payload:  map[string]interface{}{"payload": payload},
			Meta: map[string]interface{}{
				"api_key": os.Getenv("API_KEY"),
				"api_url": os.Getenv("API_URL"),
				"model":   os.Getenv("MODEL"),
			},
			TTL: 10,
		}
		bus.Publish("github-webhook", env)

		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{"status": "received"})
	}
}

func handleReviewRequest(bus *Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body error", 400)
			return
		}
		defer r.Body.Close()

		var req struct {
			Repo   string   `json:"repo"`
			PRNum  int      `json:"pr_number"`
			Author string   `json:"author"`
			Diff   string   `json:"diff_content"`
			Strict bool     `json:"strictness"`
			Ignore []string `json:"ignore_patterns"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid json", 400)
			return
		}

		prInfo := map[string]interface{}{
			"repo":      req.Repo,
			"pr_number": req.PRNum,
			"author":    req.Author,
		}

		env := &Envelop{
			Sender:   "api",
			Receiver: "github-webhook",
			Intent:   "review_trigger",
			Payload:  map[string]interface{}{},
			Meta: map[string]interface{}{
				"pr_info":         prInfo,
				"diff_content":    req.Diff,
				"ignore_patterns": req.Ignore,
				"strictness":      req.Strict,
				"api_key":         os.Getenv("API_KEY"),
				"api_url":         os.Getenv("API_URL"),
				"model":           os.Getenv("MODEL"),
			},
			TTL: 10,
		}
		bus.Publish("github-webhook", env)

		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{"status": "review started"})
	}
}
