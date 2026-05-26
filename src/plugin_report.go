package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ReportPlugin struct{}

func (p *ReportPlugin) Execute(env *Envelop, agent interface{}) *Envelop {
	fmt.Printf("[DEBUG] ReportPlugin: intent=%s\n", env.Intent)

	if env.Intent != "report" && env.Intent != "review_trigger" && env.Intent != "webhook" {
		fmt.Println("[DEBUG] ReportPlugin: intent 不匹配，跳过")
		env.Receiver = ""
		return env
	}

	// ReviewPlugin 返回的是 JSON 字符串（数组格式）
	reviewResultStr, ok := env.Meta["review_result"].(string)
	if !ok || reviewResultStr == "" {
		fmt.Println("[DEBUG] ReportPlugin: review_result 为空")
		env.Receiver = ""
		return env
	}

	// 解析 LLM 返回的审查结果数组
	var issues []map[string]interface{}
	reviewResultStr = strings.TrimSpace(reviewResultStr)
	reviewResultStr = strings.TrimPrefix(reviewResultStr, "```json")
	reviewResultStr = strings.TrimPrefix(reviewResultStr, "```")
	reviewResultStr = strings.TrimSuffix(reviewResultStr, "```")
	reviewResultStr = strings.TrimSpace(reviewResultStr)
	if err := json.Unmarshal([]byte(reviewResultStr), &issues); err != nil {
		fmt.Printf("[DEBUG] ReportPlugin: 解析 review_result 失败: %v\n", err)
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
	if fixedCode, ok := env.Meta["fixed_code"].(string); ok && fixedCode != "" {
		md.WriteString("\n## 🔧 AI 自动修复\n\n")
		md.WriteString(fmt.Sprintf("```go\n%s\n```\n\n", fixedCode))
		md.WriteString("*🤖 由 AICP Code Review Bot 自动修复*\n")
	}

	md.WriteString(fmt.Sprintf("---\n*🤖 由 AICP Code Review Bot 自动生成 | 发现 %d 个问题*", len(issues)))

	// 存到 Meta，打印到终端
	env.Meta["review_markdown"] = md.String()
	fmt.Println("\n" + md.String() + "\n")

	env.Meta["review_result"] = nil
	env.Receiver = ""
	return env
}
