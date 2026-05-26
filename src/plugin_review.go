package main

import (
	"fmt"
	"strings"
)

type ReviewPlugin struct{}

func (p *ReviewPlugin) Execute(env *Envelop, agent interface{}) *Envelop {
	fmt.Printf("[DEBUG] ReviewPlugin: intent=%s\n", env.Intent)

	if env.Intent != "fix" && env.Intent != "webhook" && env.Intent != "review_trigger" {
		fmt.Println("[DEBUG] ReviewPlugin: intent 不匹配，跳过")
		env.Receiver = "grp.report"
		return env
	}

	prInfo, ok := env.Meta["pr_info"].(map[string]interface{})
	if !ok || prInfo == nil {
		fmt.Println("[DEBUG] ReviewPlugin: pr_info 为空")
		env.Receiver = "grp.report"
		return env
	}
	fmt.Println("[DEBUG] ReviewPlugin: pr_info 有值")

	diffContent, ok := env.Meta["diff_content"].(string)
	if !ok || diffContent == "" {
		fmt.Printf("[DEBUG] ReviewPlugin: diff_content 为空, ok=%v\n", ok)
		env.Receiver = "grp.report"
		return env
	}
	fmt.Printf("[DEBUG] ReviewPlugin: diff_content 长度=%d\n", len(diffContent))

	config := ReviewConfig{}
	if cfg, ok := env.Meta["review_config"].(map[string]interface{}); ok {
		if patterns, ok := cfg["ignore_patterns"].([]interface{}); ok {
			for _, pat := range patterns {
				if s, ok := pat.(string); ok {
					config.IgnorePatterns = append(config.IgnorePatterns, s)
				}
			}
		}
		if strictness, ok := cfg["strictness"].(string); ok {
			config.Strictness = strictness
		}
	}
	fmt.Printf("[DEBUG] ReviewPlugin: ignore_patterns=%v, strictness=%s\n", config.IgnorePatterns, config.Strictness)

	filteredFiles := []string{}
	lines := strings.Split(diffContent, "\n")
	var currentFile string
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ b/") || strings.HasPrefix(line, "diff --git") {
			parts := strings.Split(line, " ")
			if len(parts) >= 2 {
				currentFile = strings.TrimPrefix(parts[len(parts)-1], "b/")
			}
		}
		skip := false
		for _, pattern := range config.IgnorePatterns {
			if pattern != "" && strings.Contains(currentFile, pattern) {
				skip = true
				break
			}
		}
		if !skip && currentFile != "" {
			if !contains(filteredFiles, currentFile) {
				filteredFiles = append(filteredFiles, currentFile)
			}
		}
	}
	// 没有文件头时用默认文件名
	if len(filteredFiles) == 0 && diffContent != "" {
		filteredFiles = append(filteredFiles, "changed_file")
	}
	fmt.Printf("[DEBUG] ReviewPlugin: filteredFiles=%v\n", filteredFiles)

	strictLevel := "medium"
	if config.Strictness != "" {
		strictLevel = config.Strictness
	}

	prTitle, _ := prInfo["title"].(string)
	prDesc, _ := prInfo["description"].(string)

	fmt.Println("[DEBUG] ReviewPlugin: 开始调用 LLM...")
	prompt := buildReviewPrompt(prTitle, prDesc, diffContent, strictLevel)

	reviewResult := p.callOpenAI(env, prompt)
	fmt.Printf("[DEBUG] ReviewPlugin: LLM 返回长度=%d\n", len(reviewResult))
	env.Meta["review_result"] = reviewResult
	env.Meta["reviewed_files"] = filteredFiles

	env.Receiver = "grp.fix"
	return env
}

type ReviewConfig struct {
	IgnorePatterns []string
	Strictness     string
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func buildReviewPrompt(title, description, diff, strictness string) string {
	return `You are an expert code reviewer. Analyze the following Pull Request:

Title: ` + title + `
Description: ` + description + `

Code Changes:
` + diff + `

Provide a structured code review focusing on these 4 dimensions:
1. Code Quality: Readability, maintainability, naming conventions
2. Security: Potential vulnerabilities, injection risks, authentication issues
3. Performance: Inefficient algorithms, unnecessary loops, memory issues
4. Maintainability: Technical debt, testability, coupling

Return your review as a JSON array with this exact structure:
[
  {
    "file": "filename",
    "line": line_number_or_null,
    "severity": "high" or "medium" or "low",
    "dimension": "quality" or "security" or "performance" or "maintainability",
    "issue": "description of the issue",
    "suggestion": "how to fix it"
  }
]

Strictness level: ` + strictness + `
If no issues found, return an empty array [].`
}

func (p *ReviewPlugin) callOpenAI(env *Envelop, prompt string) string {
	apiKey := getString(env.Meta, "api_key")
	apiURL := getString(env.Meta, "api_url")
	model := getString(env.Meta, "model")
	if apiURL == "" {
		apiURL = "https://api.openai.com"
	}
	if model == "" {
		model = "gpt-4"
	}
	fmt.Printf("[DEBUG] callOpenAI: apiURL=%s, model=%s, key长度=%d\n", apiURL, model, len(apiKey))

	result, err := CallAI(apiURL, apiKey, model, "", prompt)
	if err != nil {
		fmt.Printf("[DEBUG] callOpenAI: 调用失败: %v\n", err)
		return `{"error": "` + err.Error() + `"}`
	}
	return result
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
