package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

type GitHubWebhookPlugin struct{}

func (p *GitHubWebhookPlugin) Execute(env *Envelop, agent interface{}) *Envelop {
	fmt.Printf("[DEBUG] GitHubWebhook: intent=%s\n", env.Intent)

	// 从 Meta 直接读（handleReviewRequest 已经把数据放 Meta 了）
	diffContent, _ := env.Meta["diff_content"].(string)

	// 如果 Meta 没有，尝试从 Payload 的 payload 字段读（GitHub webhook）
	if diffContent == "" {
		payloadData, _ := env.Payload["payload"].(map[string]interface{})
		if payloadData != nil {
			// 这是 GitHub webhook 请求，提取 PR 信息
			pr, _ := payloadData["pull_request"].(map[string]interface{})
			repoData, _ := payloadData["repository"].(map[string]interface{})
			action, _ := payloadData["action"].(string)
fmt.Printf("[DEBUG] GitHubWebhook: action=%s\n", action)
			if action != "opened" && action != "synchronize" {
				env.Receiver = ""
				return env
			}

			repo := ""
			if repoData != nil {
				repo, _ = repoData["full_name"].(string)
			}
			prNumber := 0
			if pr != nil {
				if n, ok := pr["number"].(float64); ok {
					prNumber = int(n)
				}
			}
			author := ""
			if pr != nil {
				if user, ok := pr["user"].(map[string]interface{}); ok {
					author, _ = user["login"].(string)
				}
			}
			diffURL := ""
			if pr != nil {
				diffURL, _ = pr["diff_url"].(string)
			}

			diffContent = p.fetchDiff(diffURL)
			env.Meta["repo"] = repo
			env.Meta["pr_number"] = prNumber
			env.Meta["author"] = author
			env.Meta["pr_info"] = map[string]interface{}{
				"repo": repo, "pr_number": prNumber, "author": author,
			}
		}
	}

	if diffContent == "" {
		fmt.Println("[DEBUG] GitHubWebhook: diff_content 为空")
		env.Receiver = ""
		return env
	}

	env.Meta["diff_content"] = diffContent
	env.Meta["api_key"] = os.Getenv("API_KEY")
	env.Meta["api_url"] = os.Getenv("API_URL")
	env.Meta["model"] = os.Getenv("MODEL")

	env.Receiver = "grp.code-review"
	return env
}
func (p *GitHubWebhookPlugin) fetchDiff(diffURL string) string {
	if diffURL == "" {
		return ""
	}

	req, err := http.NewRequest("GET", diffURL, nil)
	if err != nil {
		fmt.Printf("[DEBUG] GitHubWebhook: fetchDiff 请求失败: %v\n", err)
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[DEBUG] GitHubWebhook: fetchDiff 请求失败: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	return string(body)
}
