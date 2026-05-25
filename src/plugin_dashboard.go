package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"
)

type DashboardPlugin struct {
	history  []ReviewRecord
	mu       sync.RWMutex
	server   *http.Server
}

type ReviewRecord struct {
	Timestamp string `json:"timestamp"`
	PR        string `json:"pr"`
	Status    string `json:"status"`
	Score     int    `json:"score"`
	Comments  int    `json:"comments"`
}

type DashboardStats struct {
	TotalReviews   int            `json:"total_reviews"`
	ApprovedCount  int            `json:"approved_count"`
	RejectedCount  int            `json:"rejected_count"`
	PendingCount   int            `json:"pending_count"`
	AverageScore   float64        `json:"average_score"`
	ReviewsByDay   map[string]int `json:"reviews_by_day"`
}

func (p *DashboardPlugin) Execute(env *Envelop, agent interface{}) *Envelop {
	if env.ChannelID != "dashboard" {
		env.Receiver = ""
		return env
	}

	if p.server == nil {
		go p.startServer()
	}

	p.loadHistory()

	env.Receiver = ""
	return env
}

func (p *DashboardPlugin) startServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/dashboard", p.handleDashboard)
	mux.HandleFunc("/api/history", p.handleHistory)
	mux.HandleFunc("/api/stats", p.handleStats)

	p.server = &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}

	p.server.ListenAndServe()
}

func (p *DashboardPlugin) loadHistory() {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := readPersistentFile("review_history.json")
	if err != nil || len(data) == 0 {
		p.history = []ReviewRecord{}
		return
	}

	var records []ReviewRecord
	if err := json.Unmarshal(data, &records); err != nil {
		p.history = []ReviewRecord{}
		return
	}

	if records == nil {
		p.history = []ReviewRecord{}
	} else {
		p.history = records
	}
}

func (p *DashboardPlugin) handleDashboard(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	stats := p.calculateStats()
	p.mu.RUnlock()

	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Code Review Dashboard</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; }
        .stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 20px; margin: 20px 0; }
        .stat-card { background: #f8f9fa; padding: 20px; border-radius: 4px; text-align: center; }
        .stat-value { font-size: 32px; font-weight: bold; color: #007bff; }
        .stat-label { color: #666; margin-top: 5px; }
        .chart { margin-top: 20px; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #f8f9fa; }
        .status-approved { color: #28a745; }
        .status-rejected { color: #dc3545; }
        .status-pending { color: #ffc107; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Code Review Dashboard</h1>
        <div class="stats">
            <div class="stat-card">
                <div class="stat-value">{{.TotalReviews}}</div>
                <div class="stat-label">Total Reviews</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{.ApprovedCount}}</div>
                <div class="stat-label">Approved</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{.RejectedCount}}</div>
                <div class="stat-label">Rejected</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{.AverageScore}}</div>
                <div class="stat-label">Avg Score</div>
            </div>
        </div>
    </div>
</body>
</html>`

	t, _ := template.New("dashboard").Parse(tmpl)
	t.Execute(w, stats)
}

func (p *DashboardPlugin) handleHistory(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p.history)
}

func (p *DashboardPlugin) handleStats(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := p.calculateStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (p *DashboardPlugin) calculateStats() DashboardStats {
	stats := DashboardStats{
		ReviewsByDay: make(map[string]int),
	}

	if p.history == nil || len(p.history) == 0 {
		return stats
	}

	var totalScore int
	for _, record := range p.history {
		stats.TotalReviews++
		totalScore += record.Score

		day := record.Timestamp[:10]
		stats.ReviewsByDay[day]++

		switch record.Status {
		case "approved":
			stats.ApprovedCount++
		case "rejected":
			stats.RejectedCount++
		case "pending":
			stats.PendingCount++
		}
	}

	if stats.TotalReviews > 0 {
		stats.AverageScore = float64(totalScore) / float64(stats.TotalReviews)
		stats.AverageScore = float64(int(stats.AverageScore*100)) / 100
	}

	return stats
}

func readPersistentFile(filename string) ([]byte, error) {
	content, err := readFile(filename)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (p *DashboardPlugin) saveHistory() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	data, err := json.MarshalIndent(p.history, "", "  ")
	if err != nil {
		return err
	}

	return writeFile("review_history.json", data)
}

func (p *DashboardPlugin) AddRecord(pr string, status string, score int, comments int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	record := ReviewRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		PR:        pr,
		Status:    status,
		Score:     score,
		Comments:  comments,
	}

	p.history = append(p.history, record)
	p.saveHistoryAsync()
}

func (p *DashboardPlugin) saveHistoryAsync() {
	go func() {
		data, err := json.MarshalIndent(p.history, "", "  ")
		if err != nil {
			return
		}
		writeFile("review_history.json", data)
	}()
}

var readFile func(string) ([]byte, error)
var writeFile func(string, []byte) error

func init() {
	readFile = defaultReadFile
	writeFile = defaultWriteFile
}

func defaultReadFile(filename string) ([]byte, error) {
	return nil, fmt.Errorf("read not implemented")
}

func defaultWriteFile(filename string, data []byte) error {
	return fmt.Errorf("write not implemented")
}