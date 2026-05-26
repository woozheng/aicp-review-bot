package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type FixPlugin struct{}

func (p *FixPlugin) Execute(env *Envelop, agent interface{}) *Envelop {
	fmt.Printf("[DEBUG] FixPlugin: intent=%s\n", env.Intent)

	// 拿到审查结果
	reviewResultStr, ok := env.Meta["review_result"].(string)
	fmt.Printf("[DEBUG] FixPlugin: review_result ok=%v, len=%d\n", ok, len(reviewResultStr))
	if !ok || reviewResultStr == "" || strings.Contains(reviewResultStr, `"error"`) {
		env.Receiver = "grp.report"
		return env
	}

	// 清洗 LLM 返回的 markdown 包裹
	reviewResultStr = strings.TrimSpace(reviewResultStr)
	reviewResultStr = strings.TrimPrefix(reviewResultStr, "```json")
	reviewResultStr = strings.TrimPrefix(reviewResultStr, "```")
	reviewResultStr = strings.TrimSuffix(reviewResultStr, "```")
	reviewResultStr = strings.TrimSpace(reviewResultStr)

	var issues []map[string]interface{}
	if err := json.Unmarshal([]byte(reviewResultStr), &issues); err != nil {
		fmt.Printf("[DEBUG] FixPlugin: 解析失败: %v\n", err)
		env.Receiver = "grp.report"
		return env
	}
	fmt.Printf("[DEBUG] FixPlugin: 解析到 %d 个问题\n", len(issues))

	// 只修复严重问题
	highIssues := []map[string]interface{}{}
	for _, issue := range issues {
		severity, _ := issue["severity"].(string)
		if severity == "high" {
			highIssues = append(highIssues, issue)
		}
	}
	fmt.Printf("[DEBUG] FixPlugin: highIssues=%d\n", len(highIssues))

	if len(highIssues) == 0 {
		fmt.Println("[DEBUG] FixPlugin: 没有需要修复的严重问题")
		env.Receiver = "grp.report"
		return env
	}

	// 获取 diff 和仓库信息
	diffContent, _ := env.Meta["diff_content"].(string)
	repoFull, _ := env.Meta["repo"].(string)
	// repo 可能在 pr_info 里
	if repoFull == "" {
		if prInfo, ok := env.Meta["pr_info"].(map[string]interface{}); ok {
			repoFull, _ = prInfo["repo"].(string)
		}
	}

	prNum := fmt.Sprintf("%v", env.Meta["pr_number"])
	if prNum == "" || prNum == "<nil>" || prNum == "0" {
		if prInfo, ok := env.Meta["pr_info"].(map[string]interface{}); ok {
			prNum = fmt.Sprintf("%v", prInfo["pr_number"])
		}
	}

	fmt.Printf("[DEBUG] FixPlugin: diffContent len=%d, repoFull=%s, prNum=%s\n", len(diffContent), repoFull, prNum)

	if diffContent == "" || repoFull == "" || prNum == "" || prNum == "<nil>" || prNum == "0" {
		fmt.Println("[DEBUG] FixPlugin: 缺少必要信息，跳过修复")
		env.Receiver = "grp.report"
		return env
	}

	// 调 LLM 修复
	fixedCode := p.fixWithLLM(env, diffContent, highIssues)
	fixedCode = strings.TrimPrefix(fixedCode, "```javascript")
	fixedCode = strings.TrimPrefix(fixedCode, "```go")
	fixedCode = strings.TrimPrefix(fixedCode, "```")
	fixedCode = strings.TrimSuffix(fixedCode, "```")
	fixedCode = strings.TrimSpace(fixedCode)
	fmt.Printf("[DEBUG] FixPlugin: fixedCode len=%d\n", len(fixedCode))
	if fixedCode == "" {
		env.Receiver = "grp.report"
		return env
	}

	// 发评论到原 PR
	token := os.Getenv("GITHUB_TOKEN")
	parts := strings.Split(repoFull, "/")
	if token != "" && len(parts) == 2 {
		msg := fmt.Sprintf("## 🔧 AI 自动修复\n\n发现 %d 个严重问题，已自动修复：\n\n```go\n%s\n```\n\n*🤖 由 AICP Code Review Bot 自动修复*", len(highIssues), fixedCode)
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments", parts[0], parts[1], prNum)
		payload, _ := json.Marshal(map[string]string{"body": msg})
		req, _ := http.NewRequest("POST", url, strings.NewReader(string(payload)))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		go http.DefaultClient.Do(req)
		fmt.Printf("[DEBUG] FixPlugin: 正在发布修复评论到 %s/issues/%s/comments\n", repoFull, prNum)
	}

	env.Meta["fixed_code"] = fixedCode
	env.Receiver = "grp.report"
	return env
}

func (p *FixPlugin) fixWithLLM(env *Envelop, diff string, issues []map[string]interface{}) string {
	apiKey := getString(env.Meta, "api_key")
	apiURL := getString(env.Meta, "api_url")
	model := getString(env.Meta, "model")
	if apiURL == "" {
		apiURL = os.Getenv("API_URL")
	}
	if model == "" {
		model = os.Getenv("MODEL")
	}

	issuesJSON, _ := json.Marshal(issues)

	prompt := fmt.Sprintf(`You are an expert code fixer. Fix the following issues in this diff:

Code Diff:
%s

Issues Found:
%s

Return ONLY the fixed code. No explanations. No markdown. Just the corrected code.`, diff, string(issuesJSON))

	result, err := CallAI(apiURL, apiKey, model, "", prompt)
	if err != nil {
		fmt.Printf("[DEBUG] FixPlugin: LLM 调用失败: %v\n", err)
		return ""
	}
	return strings.TrimSpace(result)
}
