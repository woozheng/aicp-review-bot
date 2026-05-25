package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type ReportPlugin struct{}

func (p *ReportPlugin) Execute(env *Envelop, agent interface{}) *Envelop {
	fmt.Printf("[DEBUG] ReportPlugin: intent=%s\n", env.Intent)

	if env.Intent != "report" && env.Intent != "review_trigger" && env.Intent != "webhook" {
		env.Receiver = ""
		return env
	}

	reviewResultStr, ok := env.Meta["review_result"].(string)
	if !ok || reviewResultStr == "" || strings.Contains(reviewResultStr, `"error"`) {
		fmt.Println("[DEBUG] ReportPlugin: review_result 为空或错误")
		env.Receiver = ""
		return env
	}

	reviewResultStr = cleanJSONResponse(reviewResultStr)

	var issues []map[string]interface{}
	if err := json.Unmarshal([]byte(reviewResultStr), &issues); err != nil {
		fmt.Printf("[DEBUG] ReportPlugin: 解析失败: %v\n", err)
		env.Receiver = ""
		return env
	}

	if len(issues) == 0 {
		fmt.Println("[DEBUG] ReportPlugin: 没有发现问题")
		env.Receiver = ""
		return env
	}

	// 按严重程度分类
	var highIssues, mediumIssues, lowIssues []string
	for _, issue := range issues {
		file, _ := issue["file"].(string)
		lineNum := ""
		if l, ok := issue["line"].(float64); ok {
			lineNum = fmt.Sprintf("%d", int(l))
		}
		severity, _ := issue["severity"].(string)
		dimension, _ := issue["dimension"].(string)
		desc, _ := issue["issue"].(string)
		suggestion, _ := issue["suggestion"].(string)

		entry := fmt.Sprintf("| `%s:%s` | **%s** | %s | %s |",
			file, lineNum, dimension, desc, suggestion)

		switch severity {
		case "high":
			highIssues = append(highIssues, entry)
		case "medium":
			mediumIssues = append(mediumIssues, entry)
		default:
			lowIssues = append(lowIssues, entry)
		}
	}

	// 生成 Markdown
	var md strings.Builder
	md.WriteString("## 🤖 AI Code Review\n\n")

	if len(highIssues) > 0 {
		md.WriteString("### 🔴 严重问题\n")
		md.WriteString("| 位置 | 维度 | 问题 | 建议 |\n|---|---|---|---|\n")
		for _, l := range highIssues {
			md.WriteString(l + "\n")
		}
		md.WriteString("\n")
	}

	if len(mediumIssues) > 0 {
		md.WriteString("### 🟡 警告\n")
		md.WriteString("| 位置 | 维度 | 问题 | 建议 |\n|---|---|---|---|\n")
		for _, l := range mediumIssues {
			md.WriteString(l + "\n")
		}
		md.WriteString("\n")
	}

	if len(lowIssues) > 0 {
		md.WriteString("### 🟢 建议\n")
		md.WriteString("| 位置 | 维度 | 问题 | 建议 |\n|---|---|---|---|\n")
		for _, l := range lowIssues {
			md.WriteString(l + "\n")
		}
		md.WriteString("\n")
	}

	md.WriteString(fmt.Sprintf("---\n*🤖 由 AICP Code Review Bot 自动生成 | 发现 %d 个问题*", len(issues)))

	// 打印到终端
	fmt.Println("\n" + md.String() + "\n")

	// 发 GitHub 评论
	token := os.Getenv("GITHUB_TOKEN")
	repoFull, _ := env.Meta["repo"].(string)
	prNum := fmt.Sprintf("%v", env.Meta["pr_number"])

	if token != "" && repoFull != "" && prNum != "" && prNum != "0" && prNum != "<nil>" {
		parts := strings.Split(repoFull, "/")
		if len(parts) == 2 {
			go postGitHubComment(token, parts[0], parts[1], prNum, md.String())
			fmt.Printf("[DEBUG] ReportPlugin: 正在发布评论到 %s/issues/%s/comments\n", repoFull, prNum)
		}
	} else {
		fmt.Printf("[DEBUG] ReportPlugin: 跳过评论发布 token=%t repo=%s pr=%s\n", token != "", repoFull, prNum)
	}

	env.Meta["review_result"] = nil
	env.Receiver = ""
	return env
}

func postGitHubComment(token, owner, repo, prNum, body string) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments", owner, repo, prNum)
	payload, _ := json.Marshal(map[string]string{"body": body})

	req, err := http.NewRequest("POST", url, strings.NewReader(string(payload)))
	if err != nil {
		fmt.Printf("[DEBUG] postGitHubComment: 请求失败 %v\n", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[DEBUG] postGitHubComment: 请求失败 %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 201 || resp.StatusCode == 200 {
		fmt.Printf("[DEBUG] postGitHubComment: 评论发布成功！%s/issues/%s\n", owner+"/"+repo, prNum)
	} else {
		fmt.Printf("[DEBUG] postGitHubComment: 发布失败，状态码 %d\n", resp.StatusCode)
	}
}
func cleanJSONResponse(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	return raw
}