package management

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor"
)

// Generic helpers for list[string]
func (h *Handler) putStringList(c *gin.Context, set func([]string), after func()) {
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var arr []string
	if err = json.Unmarshal(data, &arr); err != nil {
		var obj struct {
			Items []string `json:"items"`
		}
		if err2 := json.Unmarshal(data, &obj); err2 != nil || len(obj.Items) == 0 {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		arr = obj.Items
	}
	set(arr)
	if after != nil {
		after()
	}
	h.persist(c)
}

func (h *Handler) patchStringList(c *gin.Context, target *[]string, after func()) {
	var body struct {
		Old   *string `json:"old"`
		New   *string `json:"new"`
		Index *int    `json:"index"`
		Value *string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	if body.Index != nil && body.Value != nil && *body.Index >= 0 && *body.Index < len(*target) {
		(*target)[*body.Index] = *body.Value
		if after != nil {
			after()
		}
		h.persist(c)
		return
	}
	if body.Old != nil && body.New != nil {
		for i := range *target {
			if (*target)[i] == *body.Old {
				(*target)[i] = *body.New
				if after != nil {
					after()
				}
				h.persist(c)
				return
			}
		}
		*target = append(*target, *body.New)
		if after != nil {
			after()
		}
		h.persist(c)
		return
	}
	c.JSON(400, gin.H{"error": "missing fields"})
}

func (h *Handler) deleteFromStringList(c *gin.Context, target *[]string, after func()) {
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		_, err := fmt.Sscanf(idxStr, "%d", &idx)
		if err == nil && idx >= 0 && idx < len(*target) {
			*target = append((*target)[:idx], (*target)[idx+1:]...)
			if after != nil {
				after()
			}
			h.persist(c)
			return
		}
	}
	if val := strings.TrimSpace(c.Query("value")); val != "" {
		out := make([]string, 0, len(*target))
		for _, v := range *target {
			if strings.TrimSpace(v) != val {
				out = append(out, v)
			}
		}
		*target = out
		if after != nil {
			after()
		}
		h.persist(c)
		return
	}
	c.JSON(400, gin.H{"error": "missing index or value"})
}

// api-keys
func (h *Handler) GetAPIKeys(c *gin.Context) { c.JSON(200, gin.H{"api-keys": h.cfg.APIKeys}) }
func (h *Handler) PutAPIKeys(c *gin.Context) {
	h.putStringList(c, func(v []string) {
		h.cfg.APIKeys = append([]string(nil), v...)
	}, nil)
}
func (h *Handler) PatchAPIKeys(c *gin.Context) {
	h.patchStringList(c, &h.cfg.APIKeys, func() {})
}
func (h *Handler) DeleteAPIKeys(c *gin.Context) {
	h.deleteFromStringList(c, &h.cfg.APIKeys, func() {})
}

// gemini-api-key: []GeminiKey
func (h *Handler) GetGeminiKeys(c *gin.Context) {
	c.JSON(200, gin.H{"gemini-api-key": h.cfg.GeminiKey})
}
func (h *Handler) PutGeminiKeys(c *gin.Context) {
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var arr []config.GeminiKey
	if err = json.Unmarshal(data, &arr); err != nil {
		var obj struct {
			Items []config.GeminiKey `json:"items"`
		}
		if err2 := json.Unmarshal(data, &obj); err2 != nil || len(obj.Items) == 0 {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		arr = obj.Items
	}
	h.cfg.GeminiKey = append([]config.GeminiKey(nil), arr...)
	h.cfg.SanitizeGeminiKeys()
	h.persist(c)
}
func (h *Handler) PatchGeminiKey(c *gin.Context) {
	type geminiKeyPatch struct {
		APIKey         *string            `json:"api-key"`
		Prefix         *string            `json:"prefix"`
		BaseURL        *string            `json:"base-url"`
		ProxyURL       *string            `json:"proxy-url"`
		Headers        *map[string]string `json:"headers"`
		ExcludedModels *[]string          `json:"excluded-models"`
	}
	var body struct {
		Index *int            `json:"index"`
		Match *string         `json:"match"`
		Value *geminiKeyPatch `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	targetIndex := -1
	if body.Index != nil && *body.Index >= 0 && *body.Index < len(h.cfg.GeminiKey) {
		targetIndex = *body.Index
	}
	if targetIndex == -1 && body.Match != nil {
		match := strings.TrimSpace(*body.Match)
		if match != "" {
			for i := range h.cfg.GeminiKey {
				if h.cfg.GeminiKey[i].APIKey == match {
					targetIndex = i
					break
				}
			}
		}
	}
	if targetIndex == -1 {
		c.JSON(404, gin.H{"error": "item not found"})
		return
	}

	entry := h.cfg.GeminiKey[targetIndex]
	if body.Value.APIKey != nil {
		trimmed := strings.TrimSpace(*body.Value.APIKey)
		if trimmed == "" {
			h.cfg.GeminiKey = append(h.cfg.GeminiKey[:targetIndex], h.cfg.GeminiKey[targetIndex+1:]...)
			h.cfg.SanitizeGeminiKeys()
			h.persist(c)
			return
		}
		entry.APIKey = trimmed
	}
	if body.Value.Prefix != nil {
		entry.Prefix = strings.TrimSpace(*body.Value.Prefix)
	}
	if body.Value.BaseURL != nil {
		entry.BaseURL = strings.TrimSpace(*body.Value.BaseURL)
	}
	if body.Value.ProxyURL != nil {
		entry.ProxyURL = strings.TrimSpace(*body.Value.ProxyURL)
	}
	if body.Value.Headers != nil {
		entry.Headers = config.NormalizeHeaders(*body.Value.Headers)
	}
	if body.Value.ExcludedModels != nil {
		entry.ExcludedModels = config.NormalizeExcludedModels(*body.Value.ExcludedModels)
	}
	h.cfg.GeminiKey[targetIndex] = entry
	h.cfg.SanitizeGeminiKeys()
	h.persist(c)
}

func (h *Handler) DeleteGeminiKey(c *gin.Context) {
	if val := strings.TrimSpace(c.Query("api-key")); val != "" {
		if baseRaw, okBase := c.GetQuery("base-url"); okBase {
			base := strings.TrimSpace(baseRaw)
			out := make([]config.GeminiKey, 0, len(h.cfg.GeminiKey))
			for _, v := range h.cfg.GeminiKey {
				if strings.TrimSpace(v.APIKey) == val && strings.TrimSpace(v.BaseURL) == base {
					continue
				}
				out = append(out, v)
			}
			if len(out) != len(h.cfg.GeminiKey) {
				h.cfg.GeminiKey = out
				h.cfg.SanitizeGeminiKeys()
				h.persist(c)
			} else {
				c.JSON(404, gin.H{"error": "item not found"})
			}
			return
		}

		matchIndex := -1
		matchCount := 0
		for i := range h.cfg.GeminiKey {
			if strings.TrimSpace(h.cfg.GeminiKey[i].APIKey) == val {
				matchCount++
				if matchIndex == -1 {
					matchIndex = i
				}
			}
		}
		if matchCount == 0 {
			c.JSON(404, gin.H{"error": "item not found"})
			return
		}
		if matchCount > 1 {
			c.JSON(400, gin.H{"error": "multiple items match api-key; base-url is required"})
			return
		}
		h.cfg.GeminiKey = append(h.cfg.GeminiKey[:matchIndex], h.cfg.GeminiKey[matchIndex+1:]...)
		h.cfg.SanitizeGeminiKeys()
		h.persist(c)
		return
	}
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		if _, err := fmt.Sscanf(idxStr, "%d", &idx); err == nil && idx >= 0 && idx < len(h.cfg.GeminiKey) {
			h.cfg.GeminiKey = append(h.cfg.GeminiKey[:idx], h.cfg.GeminiKey[idx+1:]...)
			h.cfg.SanitizeGeminiKeys()
			h.persist(c)
			return
		}
	}
	c.JSON(400, gin.H{"error": "missing api-key or index"})
}

// claude-api-key: []ClaudeKey
func (h *Handler) GetClaudeKeys(c *gin.Context) {
	c.JSON(200, gin.H{"claude-api-key": h.cfg.ClaudeKey})
}
func (h *Handler) PutClaudeKeys(c *gin.Context) {
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var arr []config.ClaudeKey
	if err = json.Unmarshal(data, &arr); err != nil {
		var obj struct {
			Items []config.ClaudeKey `json:"items"`
		}
		if err2 := json.Unmarshal(data, &obj); err2 != nil || len(obj.Items) == 0 {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		arr = obj.Items
	}
	for i := range arr {
		normalizeClaudeKey(&arr[i])
	}
	h.cfg.ClaudeKey = arr
	h.cfg.SanitizeClaudeKeys()
	h.persist(c)
}
func (h *Handler) PatchClaudeKey(c *gin.Context) {
	type claudeKeyPatch struct {
		Name           *string               `json:"name"`
		APIKey         *string               `json:"api-key"`
		Prefix         *string               `json:"prefix"`
		BaseURL        *string               `json:"base-url"`
		ProxyURL       *string               `json:"proxy-url"`
		Models         *[]config.ClaudeModel `json:"models"`
		Headers        *map[string]string    `json:"headers"`
		ExcludedModels *[]string             `json:"excluded-models"`
	}
	var body struct {
		Index *int            `json:"index"`
		Match *string         `json:"match"`
		Value *claudeKeyPatch `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	targetIndex := -1
	if body.Index != nil && *body.Index >= 0 && *body.Index < len(h.cfg.ClaudeKey) {
		targetIndex = *body.Index
	}
	if targetIndex == -1 && body.Match != nil {
		match := strings.TrimSpace(*body.Match)
		for i := range h.cfg.ClaudeKey {
			if h.cfg.ClaudeKey[i].APIKey == match {
				targetIndex = i
				break
			}
		}
	}
	if targetIndex == -1 {
		c.JSON(404, gin.H{"error": "item not found"})
		return
	}

	entry := h.cfg.ClaudeKey[targetIndex]
	if body.Value.Name != nil {
		entry.Name = strings.TrimSpace(*body.Value.Name)
	}
	if body.Value.APIKey != nil {
		entry.APIKey = strings.TrimSpace(*body.Value.APIKey)
	}
	if body.Value.Prefix != nil {
		entry.Prefix = strings.TrimSpace(*body.Value.Prefix)
	}
	if body.Value.BaseURL != nil {
		entry.BaseURL = strings.TrimSpace(*body.Value.BaseURL)
	}
	if body.Value.ProxyURL != nil {
		entry.ProxyURL = strings.TrimSpace(*body.Value.ProxyURL)
	}
	if body.Value.Models != nil {
		entry.Models = append([]config.ClaudeModel(nil), (*body.Value.Models)...)
	}
	if body.Value.Headers != nil {
		entry.Headers = config.NormalizeHeaders(*body.Value.Headers)
	}
	if body.Value.ExcludedModels != nil {
		entry.ExcludedModels = config.NormalizeExcludedModels(*body.Value.ExcludedModels)
	}
	normalizeClaudeKey(&entry)
	h.cfg.ClaudeKey[targetIndex] = entry
	h.cfg.SanitizeClaudeKeys()
	h.persist(c)
}

func (h *Handler) DeleteClaudeKey(c *gin.Context) {
	if val := strings.TrimSpace(c.Query("api-key")); val != "" {
		if baseRaw, okBase := c.GetQuery("base-url"); okBase {
			base := strings.TrimSpace(baseRaw)
			out := make([]config.ClaudeKey, 0, len(h.cfg.ClaudeKey))
			for _, v := range h.cfg.ClaudeKey {
				if strings.TrimSpace(v.APIKey) == val && strings.TrimSpace(v.BaseURL) == base {
					continue
				}
				out = append(out, v)
			}
			h.cfg.ClaudeKey = out
			h.cfg.SanitizeClaudeKeys()
			h.persist(c)
			return
		}

		matchIndex := -1
		matchCount := 0
		for i := range h.cfg.ClaudeKey {
			if strings.TrimSpace(h.cfg.ClaudeKey[i].APIKey) == val {
				matchCount++
				if matchIndex == -1 {
					matchIndex = i
				}
			}
		}
		if matchCount > 1 {
			c.JSON(400, gin.H{"error": "multiple items match api-key; base-url is required"})
			return
		}
		if matchIndex != -1 {
			h.cfg.ClaudeKey = append(h.cfg.ClaudeKey[:matchIndex], h.cfg.ClaudeKey[matchIndex+1:]...)
		}
		h.cfg.SanitizeClaudeKeys()
		h.persist(c)
		return
	}
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		_, err := fmt.Sscanf(idxStr, "%d", &idx)
		if err == nil && idx >= 0 && idx < len(h.cfg.ClaudeKey) {
			h.cfg.ClaudeKey = append(h.cfg.ClaudeKey[:idx], h.cfg.ClaudeKey[idx+1:]...)
			h.cfg.SanitizeClaudeKeys()
			h.persist(c)
			return
		}
	}
	c.JSON(400, gin.H{"error": "missing api-key or index"})
}

// openai-compatibility: []OpenAICompatibility
func (h *Handler) GetOpenAICompat(c *gin.Context) {
	c.JSON(200, gin.H{"openai-compatibility": normalizedOpenAICompatibilityEntries(h.cfg.OpenAICompatibility)})
}
func (h *Handler) PutOpenAICompat(c *gin.Context) {
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var arr []config.OpenAICompatibility
	if err = json.Unmarshal(data, &arr); err != nil {
		var obj struct {
			Items []config.OpenAICompatibility `json:"items"`
		}
		if err2 := json.Unmarshal(data, &obj); err2 != nil || len(obj.Items) == 0 {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		arr = obj.Items
	}
	filtered := make([]config.OpenAICompatibility, 0, len(arr))
	for i := range arr {
		normalizeOpenAICompatibilityEntry(&arr[i])
		if strings.TrimSpace(arr[i].BaseURL) != "" {
			filtered = append(filtered, arr[i])
		}
	}
	h.cfg.OpenAICompatibility = filtered
	h.cfg.SanitizeOpenAICompatibility()
	h.persist(c)
}
func (h *Handler) PatchOpenAICompat(c *gin.Context) {
	type openAICompatPatch struct {
		Name          *string                             `json:"name"`
		Prefix        *string                             `json:"prefix"`
		BaseURL       *string                             `json:"base-url"`
		APIKeyEntries *[]config.OpenAICompatibilityAPIKey `json:"api-key-entries"`
		Models        *[]config.OpenAICompatibilityModel  `json:"models"`
		Headers       *map[string]string                  `json:"headers"`
	}
	var body struct {
		Name  *string            `json:"name"`
		Index *int               `json:"index"`
		Value *openAICompatPatch `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	targetIndex := -1
	if body.Index != nil && *body.Index >= 0 && *body.Index < len(h.cfg.OpenAICompatibility) {
		targetIndex = *body.Index
	}
	if targetIndex == -1 && body.Name != nil {
		match := strings.TrimSpace(*body.Name)
		for i := range h.cfg.OpenAICompatibility {
			if h.cfg.OpenAICompatibility[i].Name == match {
				targetIndex = i
				break
			}
		}
	}
	if targetIndex == -1 {
		c.JSON(404, gin.H{"error": "item not found"})
		return
	}

	entry := h.cfg.OpenAICompatibility[targetIndex]
	if body.Value.Name != nil {
		entry.Name = strings.TrimSpace(*body.Value.Name)
	}
	if body.Value.Prefix != nil {
		entry.Prefix = strings.TrimSpace(*body.Value.Prefix)
	}
	if body.Value.BaseURL != nil {
		trimmed := strings.TrimSpace(*body.Value.BaseURL)
		if trimmed == "" {
			h.cfg.OpenAICompatibility = append(h.cfg.OpenAICompatibility[:targetIndex], h.cfg.OpenAICompatibility[targetIndex+1:]...)
			h.cfg.SanitizeOpenAICompatibility()
			h.persist(c)
			return
		}
		entry.BaseURL = trimmed
	}
	if body.Value.APIKeyEntries != nil {
		entry.APIKeyEntries = append([]config.OpenAICompatibilityAPIKey(nil), (*body.Value.APIKeyEntries)...)
	}
	if body.Value.Models != nil {
		entry.Models = append([]config.OpenAICompatibilityModel(nil), (*body.Value.Models)...)
	}
	if body.Value.Headers != nil {
		entry.Headers = config.NormalizeHeaders(*body.Value.Headers)
	}
	normalizeOpenAICompatibilityEntry(&entry)
	h.cfg.OpenAICompatibility[targetIndex] = entry
	h.cfg.SanitizeOpenAICompatibility()
	h.persist(c)
}

func (h *Handler) DeleteOpenAICompat(c *gin.Context) {
	if name := c.Query("name"); name != "" {
		out := make([]config.OpenAICompatibility, 0, len(h.cfg.OpenAICompatibility))
		for _, v := range h.cfg.OpenAICompatibility {
			if v.Name != name {
				out = append(out, v)
			}
		}
		h.cfg.OpenAICompatibility = out
		h.cfg.SanitizeOpenAICompatibility()
		h.persist(c)
		return
	}
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		_, err := fmt.Sscanf(idxStr, "%d", &idx)
		if err == nil && idx >= 0 && idx < len(h.cfg.OpenAICompatibility) {
			h.cfg.OpenAICompatibility = append(h.cfg.OpenAICompatibility[:idx], h.cfg.OpenAICompatibility[idx+1:]...)
			h.cfg.SanitizeOpenAICompatibility()
			h.persist(c)
			return
		}
	}
	c.JSON(400, gin.H{"error": "missing name or index"})
}

// vertex-api-key: []VertexCompatKey
func (h *Handler) GetVertexCompatKeys(c *gin.Context) {
	c.JSON(200, gin.H{"vertex-api-key": h.cfg.VertexCompatAPIKey})
}
func (h *Handler) PutVertexCompatKeys(c *gin.Context) {
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var arr []config.VertexCompatKey
	if err = json.Unmarshal(data, &arr); err != nil {
		var obj struct {
			Items []config.VertexCompatKey `json:"items"`
		}
		if err2 := json.Unmarshal(data, &obj); err2 != nil || len(obj.Items) == 0 {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		arr = obj.Items
	}
	for i := range arr {
		normalizeVertexCompatKey(&arr[i])
		if arr[i].APIKey == "" {
			c.JSON(400, gin.H{"error": fmt.Sprintf("vertex-api-key[%d].api-key is required", i)})
			return
		}
	}
	h.cfg.VertexCompatAPIKey = append([]config.VertexCompatKey(nil), arr...)
	h.cfg.SanitizeVertexCompatKeys()
	h.persist(c)
}
func (h *Handler) PatchVertexCompatKey(c *gin.Context) {
	type vertexCompatPatch struct {
		APIKey         *string                     `json:"api-key"`
		Prefix         *string                     `json:"prefix"`
		BaseURL        *string                     `json:"base-url"`
		ProxyURL       *string                     `json:"proxy-url"`
		Headers        *map[string]string          `json:"headers"`
		Models         *[]config.VertexCompatModel `json:"models"`
		ExcludedModels *[]string                   `json:"excluded-models"`
	}
	var body struct {
		Index *int               `json:"index"`
		Match *string            `json:"match"`
		Value *vertexCompatPatch `json:"value"`
	}
	if errBindJSON := c.ShouldBindJSON(&body); errBindJSON != nil || body.Value == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	targetIndex := -1
	if body.Index != nil && *body.Index >= 0 && *body.Index < len(h.cfg.VertexCompatAPIKey) {
		targetIndex = *body.Index
	}
	if targetIndex == -1 && body.Match != nil {
		match := strings.TrimSpace(*body.Match)
		if match != "" {
			for i := range h.cfg.VertexCompatAPIKey {
				if h.cfg.VertexCompatAPIKey[i].APIKey == match {
					targetIndex = i
					break
				}
			}
		}
	}
	if targetIndex == -1 {
		c.JSON(404, gin.H{"error": "item not found"})
		return
	}

	entry := h.cfg.VertexCompatAPIKey[targetIndex]
	if body.Value.APIKey != nil {
		trimmed := strings.TrimSpace(*body.Value.APIKey)
		if trimmed == "" {
			h.cfg.VertexCompatAPIKey = append(h.cfg.VertexCompatAPIKey[:targetIndex], h.cfg.VertexCompatAPIKey[targetIndex+1:]...)
			h.cfg.SanitizeVertexCompatKeys()
			h.persist(c)
			return
		}
		entry.APIKey = trimmed
	}
	if body.Value.Prefix != nil {
		entry.Prefix = strings.TrimSpace(*body.Value.Prefix)
	}
	if body.Value.BaseURL != nil {
		trimmed := strings.TrimSpace(*body.Value.BaseURL)
		if trimmed == "" {
			h.cfg.VertexCompatAPIKey = append(h.cfg.VertexCompatAPIKey[:targetIndex], h.cfg.VertexCompatAPIKey[targetIndex+1:]...)
			h.cfg.SanitizeVertexCompatKeys()
			h.persist(c)
			return
		}
		entry.BaseURL = trimmed
	}
	if body.Value.ProxyURL != nil {
		entry.ProxyURL = strings.TrimSpace(*body.Value.ProxyURL)
	}
	if body.Value.Headers != nil {
		entry.Headers = config.NormalizeHeaders(*body.Value.Headers)
	}
	if body.Value.Models != nil {
		entry.Models = append([]config.VertexCompatModel(nil), (*body.Value.Models)...)
	}
	if body.Value.ExcludedModels != nil {
		entry.ExcludedModels = config.NormalizeExcludedModels(*body.Value.ExcludedModels)
	}
	normalizeVertexCompatKey(&entry)
	h.cfg.VertexCompatAPIKey[targetIndex] = entry
	h.cfg.SanitizeVertexCompatKeys()
	h.persist(c)
}

func (h *Handler) DeleteVertexCompatKey(c *gin.Context) {
	if val := strings.TrimSpace(c.Query("api-key")); val != "" {
		if baseRaw, okBase := c.GetQuery("base-url"); okBase {
			base := strings.TrimSpace(baseRaw)
			out := make([]config.VertexCompatKey, 0, len(h.cfg.VertexCompatAPIKey))
			for _, v := range h.cfg.VertexCompatAPIKey {
				if strings.TrimSpace(v.APIKey) == val && strings.TrimSpace(v.BaseURL) == base {
					continue
				}
				out = append(out, v)
			}
			h.cfg.VertexCompatAPIKey = out
			h.cfg.SanitizeVertexCompatKeys()
			h.persist(c)
			return
		}

		matchIndex := -1
		matchCount := 0
		for i := range h.cfg.VertexCompatAPIKey {
			if strings.TrimSpace(h.cfg.VertexCompatAPIKey[i].APIKey) == val {
				matchCount++
				if matchIndex == -1 {
					matchIndex = i
				}
			}
		}
		if matchCount > 1 {
			c.JSON(400, gin.H{"error": "multiple items match api-key; base-url is required"})
			return
		}
		if matchIndex != -1 {
			h.cfg.VertexCompatAPIKey = append(h.cfg.VertexCompatAPIKey[:matchIndex], h.cfg.VertexCompatAPIKey[matchIndex+1:]...)
		}
		h.cfg.SanitizeVertexCompatKeys()
		h.persist(c)
		return
	}
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		_, errScan := fmt.Sscanf(idxStr, "%d", &idx)
		if errScan == nil && idx >= 0 && idx < len(h.cfg.VertexCompatAPIKey) {
			h.cfg.VertexCompatAPIKey = append(h.cfg.VertexCompatAPIKey[:idx], h.cfg.VertexCompatAPIKey[idx+1:]...)
			h.cfg.SanitizeVertexCompatKeys()
			h.persist(c)
			return
		}
	}
	c.JSON(400, gin.H{"error": "missing api-key or index"})
}

// oauth-excluded-models: map[string][]string
func (h *Handler) GetOAuthExcludedModels(c *gin.Context) {
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()
	c.JSON(200, gin.H{"oauth-excluded-models": config.NormalizeOAuthExcludedModels(h.cfg.OAuthExcludedModels)})
}

func (h *Handler) PutOAuthExcludedModels(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var entries map[string][]string
	if err = json.Unmarshal(data, &entries); err != nil {
		var wrapper struct {
			Items map[string][]string `json:"items"`
		}
		if err2 := json.Unmarshal(data, &wrapper); err2 != nil {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		entries = wrapper.Items
	}
	h.cfg.OAuthExcludedModels = config.NormalizeOAuthExcludedModels(entries)
	h.persist(c)
}

func (h *Handler) PatchOAuthExcludedModels(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var body struct {
		Provider *string  `json:"provider"`
		Models   []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Provider == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	provider := strings.ToLower(strings.TrimSpace(*body.Provider))
	if provider == "" {
		c.JSON(400, gin.H{"error": "invalid provider"})
		return
	}
	normalized := config.NormalizeExcludedModels(body.Models)
	if len(normalized) == 0 {
		if h.cfg.OAuthExcludedModels == nil {
			c.JSON(404, gin.H{"error": "provider not found"})
			return
		}
		if _, ok := h.cfg.OAuthExcludedModels[provider]; !ok {
			c.JSON(404, gin.H{"error": "provider not found"})
			return
		}
		delete(h.cfg.OAuthExcludedModels, provider)
		if len(h.cfg.OAuthExcludedModels) == 0 {
			h.cfg.OAuthExcludedModels = nil
		}
		h.persist(c)
		return
	}
	if h.cfg.OAuthExcludedModels == nil {
		h.cfg.OAuthExcludedModels = make(map[string][]string)
	}
	h.cfg.OAuthExcludedModels[provider] = normalized
	h.persist(c)
}

func (h *Handler) DeleteOAuthExcludedModels(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	provider := strings.ToLower(strings.TrimSpace(c.Query("provider")))
	if provider == "" {
		c.JSON(400, gin.H{"error": "missing provider"})
		return
	}
	if h.cfg.OAuthExcludedModels == nil {
		c.JSON(404, gin.H{"error": "provider not found"})
		return
	}
	if _, ok := h.cfg.OAuthExcludedModels[provider]; !ok {
		c.JSON(404, gin.H{"error": "provider not found"})
		return
	}
	delete(h.cfg.OAuthExcludedModels, provider)
	if len(h.cfg.OAuthExcludedModels) == 0 {
		h.cfg.OAuthExcludedModels = nil
	}
	h.persist(c)
}

// oauth-model-alias: map[string][]OAuthModelAlias
func (h *Handler) GetOAuthModelAlias(c *gin.Context) {
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()
	c.JSON(200, gin.H{"oauth-model-alias": sanitizedOAuthModelAlias(h.cfg.OAuthModelAlias)})
}

func (h *Handler) PutOAuthModelAlias(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var entries map[string][]config.OAuthModelAlias
	if err = json.Unmarshal(data, &entries); err != nil {
		var wrapper struct {
			Items map[string][]config.OAuthModelAlias `json:"items"`
		}
		if err2 := json.Unmarshal(data, &wrapper); err2 != nil {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		entries = wrapper.Items
	}
	h.cfg.OAuthModelAlias = sanitizedOAuthModelAlias(entries)
	h.persist(c)
}

func (h *Handler) PatchOAuthModelAlias(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var body struct {
		Provider *string                  `json:"provider"`
		Channel  *string                  `json:"channel"`
		Aliases  []config.OAuthModelAlias `json:"aliases"`
	}
	if errBindJSON := c.ShouldBindJSON(&body); errBindJSON != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	channelRaw := ""
	if body.Channel != nil {
		channelRaw = *body.Channel
	} else if body.Provider != nil {
		channelRaw = *body.Provider
	}
	channel := strings.ToLower(strings.TrimSpace(channelRaw))
	if channel == "" {
		c.JSON(400, gin.H{"error": "invalid channel"})
		return
	}

	normalizedMap := sanitizedOAuthModelAlias(map[string][]config.OAuthModelAlias{channel: body.Aliases})
	normalized := normalizedMap[channel]
	if len(normalized) == 0 {
		if h.cfg.OAuthModelAlias == nil {
			c.JSON(404, gin.H{"error": "channel not found"})
			return
		}
		if _, ok := h.cfg.OAuthModelAlias[channel]; !ok {
			c.JSON(404, gin.H{"error": "channel not found"})
			return
		}
		delete(h.cfg.OAuthModelAlias, channel)
		if len(h.cfg.OAuthModelAlias) == 0 {
			h.cfg.OAuthModelAlias = nil
		}
		h.persist(c)
		return
	}
	if h.cfg.OAuthModelAlias == nil {
		h.cfg.OAuthModelAlias = make(map[string][]config.OAuthModelAlias)
	}
	h.cfg.OAuthModelAlias[channel] = normalized
	h.persist(c)
}

func (h *Handler) DeleteOAuthModelAlias(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	channel := strings.ToLower(strings.TrimSpace(c.Query("channel")))
	if channel == "" {
		channel = strings.ToLower(strings.TrimSpace(c.Query("provider")))
	}
	if channel == "" {
		c.JSON(400, gin.H{"error": "missing channel"})
		return
	}
	if h.cfg.OAuthModelAlias == nil {
		c.JSON(404, gin.H{"error": "channel not found"})
		return
	}
	if _, ok := h.cfg.OAuthModelAlias[channel]; !ok {
		c.JSON(404, gin.H{"error": "channel not found"})
		return
	}
	delete(h.cfg.OAuthModelAlias, channel)
	if len(h.cfg.OAuthModelAlias) == 0 {
		h.cfg.OAuthModelAlias = nil
	}
	h.persist(c)
}

// codex-api-key: []CodexKey
func (h *Handler) GetCodexKeys(c *gin.Context) {
	c.JSON(200, gin.H{"codex-api-key": h.cfg.CodexKey})
}
func (h *Handler) PutCodexKeys(c *gin.Context) {
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var arr []config.CodexKey
	if err = json.Unmarshal(data, &arr); err != nil {
		var obj struct {
			Items []config.CodexKey `json:"items"`
		}
		if err2 := json.Unmarshal(data, &obj); err2 != nil || len(obj.Items) == 0 {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		arr = obj.Items
	}
	// Filter out codex entries with empty base-url (treat as removed)
	filtered := make([]config.CodexKey, 0, len(arr))
	for i := range arr {
		entry := arr[i]
		normalizeCodexKey(&entry)
		if entry.BaseURL == "" {
			continue
		}
		filtered = append(filtered, entry)
	}
	h.cfg.CodexKey = filtered
	h.cfg.SanitizeCodexKeys()
	h.persist(c)
}
func (h *Handler) PatchCodexKey(c *gin.Context) {
	type codexKeyPatch struct {
		APIKey         *string              `json:"api-key"`
		Prefix         *string              `json:"prefix"`
		BaseURL        *string              `json:"base-url"`
		ProxyURL       *string              `json:"proxy-url"`
		Models         *[]config.CodexModel `json:"models"`
		Headers        *map[string]string   `json:"headers"`
		ExcludedModels *[]string            `json:"excluded-models"`
	}
	var body struct {
		Index *int           `json:"index"`
		Match *string        `json:"match"`
		Value *codexKeyPatch `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	targetIndex := -1
	if body.Index != nil && *body.Index >= 0 && *body.Index < len(h.cfg.CodexKey) {
		targetIndex = *body.Index
	}
	if targetIndex == -1 && body.Match != nil {
		match := strings.TrimSpace(*body.Match)
		for i := range h.cfg.CodexKey {
			if h.cfg.CodexKey[i].APIKey == match {
				targetIndex = i
				break
			}
		}
	}
	if targetIndex == -1 {
		c.JSON(404, gin.H{"error": "item not found"})
		return
	}

	entry := h.cfg.CodexKey[targetIndex]
	if body.Value.APIKey != nil {
		entry.APIKey = strings.TrimSpace(*body.Value.APIKey)
	}
	if body.Value.Prefix != nil {
		entry.Prefix = strings.TrimSpace(*body.Value.Prefix)
	}
	if body.Value.BaseURL != nil {
		trimmed := strings.TrimSpace(*body.Value.BaseURL)
		if trimmed == "" {
			h.cfg.CodexKey = append(h.cfg.CodexKey[:targetIndex], h.cfg.CodexKey[targetIndex+1:]...)
			h.cfg.SanitizeCodexKeys()
			h.persist(c)
			return
		}
		entry.BaseURL = trimmed
	}
	if body.Value.ProxyURL != nil {
		entry.ProxyURL = strings.TrimSpace(*body.Value.ProxyURL)
	}
	if body.Value.Models != nil {
		entry.Models = append([]config.CodexModel(nil), (*body.Value.Models)...)
	}
	if body.Value.Headers != nil {
		entry.Headers = config.NormalizeHeaders(*body.Value.Headers)
	}
	if body.Value.ExcludedModels != nil {
		entry.ExcludedModels = config.NormalizeExcludedModels(*body.Value.ExcludedModels)
	}
	normalizeCodexKey(&entry)
	h.cfg.CodexKey[targetIndex] = entry
	h.cfg.SanitizeCodexKeys()
	h.persist(c)
}

func (h *Handler) DeleteCodexKey(c *gin.Context) {
	if val := strings.TrimSpace(c.Query("api-key")); val != "" {
		if baseRaw, okBase := c.GetQuery("base-url"); okBase {
			base := strings.TrimSpace(baseRaw)
			out := make([]config.CodexKey, 0, len(h.cfg.CodexKey))
			for _, v := range h.cfg.CodexKey {
				if strings.TrimSpace(v.APIKey) == val && strings.TrimSpace(v.BaseURL) == base {
					continue
				}
				out = append(out, v)
			}
			h.cfg.CodexKey = out
			h.cfg.SanitizeCodexKeys()
			h.persist(c)
			return
		}

		matchIndex := -1
		matchCount := 0
		for i := range h.cfg.CodexKey {
			if strings.TrimSpace(h.cfg.CodexKey[i].APIKey) == val {
				matchCount++
				if matchIndex == -1 {
					matchIndex = i
				}
			}
		}
		if matchCount > 1 {
			c.JSON(400, gin.H{"error": "multiple items match api-key; base-url is required"})
			return
		}
		if matchIndex != -1 {
			h.cfg.CodexKey = append(h.cfg.CodexKey[:matchIndex], h.cfg.CodexKey[matchIndex+1:]...)
		}
		h.cfg.SanitizeCodexKeys()
		h.persist(c)
		return
	}
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		_, err := fmt.Sscanf(idxStr, "%d", &idx)
		if err == nil && idx >= 0 && idx < len(h.cfg.CodexKey) {
			h.cfg.CodexKey = append(h.cfg.CodexKey[:idx], h.cfg.CodexKey[idx+1:]...)
			h.cfg.SanitizeCodexKeys()
			h.persist(c)
			return
		}
	}
	c.JSON(400, gin.H{"error": "missing api-key or index"})
}

func normalizeOpenAICompatibilityEntry(entry *config.OpenAICompatibility) {
	if entry == nil {
		return
	}
	// Trim base-url; empty base-url indicates provider should be removed by sanitization
	entry.BaseURL = strings.TrimSpace(entry.BaseURL)
	entry.Headers = config.NormalizeHeaders(entry.Headers)
	existing := make(map[string]struct{}, len(entry.APIKeyEntries))
	for i := range entry.APIKeyEntries {
		trimmed := strings.TrimSpace(entry.APIKeyEntries[i].APIKey)
		entry.APIKeyEntries[i].APIKey = trimmed
		if trimmed != "" {
			existing[trimmed] = struct{}{}
		}
	}
}

func normalizedOpenAICompatibilityEntries(entries []config.OpenAICompatibility) []config.OpenAICompatibility {
	if len(entries) == 0 {
		return nil
	}
	out := make([]config.OpenAICompatibility, len(entries))
	for i := range entries {
		copyEntry := entries[i]
		if len(copyEntry.APIKeyEntries) > 0 {
			copyEntry.APIKeyEntries = append([]config.OpenAICompatibilityAPIKey(nil), copyEntry.APIKeyEntries...)
		}
		normalizeOpenAICompatibilityEntry(&copyEntry)
		out[i] = copyEntry
	}
	return out
}

func normalizeClaudeKey(entry *config.ClaudeKey) {
	if entry == nil {
		return
	}
	entry.APIKey = strings.TrimSpace(entry.APIKey)
	entry.BaseURL = strings.TrimSpace(entry.BaseURL)
	entry.ProxyURL = strings.TrimSpace(entry.ProxyURL)
	entry.Headers = config.NormalizeHeaders(entry.Headers)
	entry.ExcludedModels = config.NormalizeExcludedModels(entry.ExcludedModels)
	if len(entry.Models) == 0 {
		return
	}
	normalized := make([]config.ClaudeModel, 0, len(entry.Models))
	for i := range entry.Models {
		model := entry.Models[i]
		model.Name = strings.TrimSpace(model.Name)
		model.Alias = strings.TrimSpace(model.Alias)
		if model.Name == "" && model.Alias == "" {
			continue
		}
		normalized = append(normalized, model)
	}
	entry.Models = normalized
}

func normalizeCodexKey(entry *config.CodexKey) {
	if entry == nil {
		return
	}
	entry.APIKey = strings.TrimSpace(entry.APIKey)
	entry.Prefix = strings.TrimSpace(entry.Prefix)
	entry.BaseURL = strings.TrimSpace(entry.BaseURL)
	entry.ProxyURL = strings.TrimSpace(entry.ProxyURL)
	entry.Headers = config.NormalizeHeaders(entry.Headers)
	entry.ExcludedModels = config.NormalizeExcludedModels(entry.ExcludedModels)
	if len(entry.Models) == 0 {
		return
	}
	normalized := make([]config.CodexModel, 0, len(entry.Models))
	for i := range entry.Models {
		model := entry.Models[i]
		model.Name = strings.TrimSpace(model.Name)
		model.Alias = strings.TrimSpace(model.Alias)
		if model.Name == "" && model.Alias == "" {
			continue
		}
		normalized = append(normalized, model)
	}
	entry.Models = normalized
}

// trae-api-key: []TraeKey
func (h *Handler) GetTraeKeys(c *gin.Context) {
	keys := h.cfg.TraeKey

	// Build a lookup map from Trae's getconfig API for contextLength enrichment.
	// This is needed because saved configs may not have contextLength (added later),
	// and Trae models are not in the static registry.
	type clInfo struct {
		configName      string
		promptMaxTokens int
	}
	var clMap map[string]int // configName -> promptMaxTokens

	for i := range keys {
		hasMissing := false
		for j := range keys[i].Models {
			if keys[i].Models[j].ContextLength <= 0 {
				hasMissing = true
				break
			}
		}
		if !hasMissing {
			continue
		}

		// Call Trae getconfig API to fetch prompt_max_tokens
		baseURL := keys[i].BaseURL
		if baseURL == "" {
			baseURL = "https://console.enterprise.trae.cn"
		}
		configs, _, err := executor.FetchTraeConfigList(c.Request.Context(), baseURL, keys[i].APIKey, keys[i].RefreshToken)
		if err != nil || len(configs) == 0 {
			continue
		}
		if clMap == nil {
			clMap = make(map[string]int)
		}
		for _, cfg := range configs {
			if cfg.Usage != "chat_completion" && cfg.Usage != "" {
				continue
			}
			for _, detail := range cfg.ModelDetailList {
				if detail.PromptMaxTokens > 0 {
					modelName := detail.ModelName
					if modelName == "" {
						modelName = cfg.ConfigName
					}
					clMap[modelName] = detail.PromptMaxTokens
					clMap[cfg.ConfigName] = detail.PromptMaxTokens
				}
			}
			// Also handle configs without ModelDetailList
			if len(cfg.ModelDetailList) == 0 && cfg.Usage == "chat_completion" {
				// Will fall through to static registry below
			}
		}
	}

	// Enrich contextLength for saved models that lack it
	for i := range keys {
		for j := range keys[i].Models {
			if keys[i].Models[j].ContextLength <= 0 {
				m := keys[i].Models[j]
				enriched := false

				// Try Trae API data first
				if clMap != nil {
					if v, ok := clMap[m.ModelName]; ok && v > 0 {
						keys[i].Models[j].ContextLength = int64(v)
						enriched = true
					} else if v, ok := clMap[m.ConfigName]; ok && v > 0 {
						keys[i].Models[j].ContextLength = int64(v)
						enriched = true
					}
				}

				// Fallback to static registry
				if !enriched {
					if upstream := registry.LookupStaticModelInfo(m.ModelName); upstream != nil && upstream.ContextLength > 0 {
						keys[i].Models[j].ContextLength = int64(upstream.ContextLength)
					} else if upstream := registry.LookupStaticModelInfo(m.ConfigName); upstream != nil && upstream.ContextLength > 0 {
						keys[i].Models[j].ContextLength = int64(upstream.ContextLength)
					}
				}
			}
		}
	}

	c.JSON(200, gin.H{"trae-api-key": keys})
}
func (h *Handler) PutTraeKeys(c *gin.Context) {
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var arr []config.TraeKey
	if err = json.Unmarshal(data, &arr); err != nil {
		var obj struct {
			Items []config.TraeKey `json:"items"`
		}
		if err2 := json.Unmarshal(data, &obj); err2 != nil || len(obj.Items) == 0 {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		arr = obj.Items
	}
	for i := range arr {
		normalizeTraeKey(&arr[i])
	}
	h.cfg.TraeKey = arr
	h.cfg.SanitizeTraeKeys()
	h.persist(c)
}
func (h *Handler) PatchTraeKey(c *gin.Context) {
	type traeKeyPatch struct {
		Name           *string             `json:"name"`
		APIKey         *string             `json:"api-key"`
		RefreshToken   *string             `json:"refresh-token"`
		Prefix         *string             `json:"prefix"`
		BaseURL        *string             `json:"base-url"`
		ProxyURL       *string             `json:"proxy-url"`
		Models         *[]config.TraeModel `json:"models"`
		Headers        *map[string]string  `json:"headers"`
		ExcludedModels *[]string           `json:"excluded-models"`
	}
	var body struct {
		Index *int          `json:"index"`
		Match *string       `json:"match"`
		Value *traeKeyPatch `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	targetIndex := -1
	if body.Index != nil && *body.Index >= 0 && *body.Index < len(h.cfg.TraeKey) {
		targetIndex = *body.Index
	}
	if targetIndex == -1 && body.Match != nil {
		match := strings.TrimSpace(*body.Match)
		for i := range h.cfg.TraeKey {
			if h.cfg.TraeKey[i].APIKey == match {
				targetIndex = i
				break
			}
		}
	}
	if targetIndex == -1 {
		c.JSON(404, gin.H{"error": "item not found"})
		return
	}

	entry := h.cfg.TraeKey[targetIndex]
	if body.Value.Name != nil {
		entry.Name = strings.TrimSpace(*body.Value.Name)
	}
	if body.Value.APIKey != nil {
		entry.APIKey = strings.TrimSpace(*body.Value.APIKey)
	}
	if body.Value.RefreshToken != nil {
		entry.RefreshToken = strings.TrimSpace(*body.Value.RefreshToken)
	}
	if body.Value.Prefix != nil {
		entry.Prefix = strings.TrimSpace(*body.Value.Prefix)
	}
	if body.Value.BaseURL != nil {
		entry.BaseURL = strings.TrimSpace(*body.Value.BaseURL)
	}
	if body.Value.ProxyURL != nil {
		entry.ProxyURL = strings.TrimSpace(*body.Value.ProxyURL)
	}
	if body.Value.Models != nil {
		entry.Models = append([]config.TraeModel(nil), (*body.Value.Models)...)
	}
	if body.Value.Headers != nil {
		entry.Headers = config.NormalizeHeaders(*body.Value.Headers)
	}
	if body.Value.ExcludedModels != nil {
		entry.ExcludedModels = config.NormalizeExcludedModels(*body.Value.ExcludedModels)
	}
	normalizeTraeKey(&entry)
	h.cfg.TraeKey[targetIndex] = entry
	h.cfg.SanitizeTraeKeys()
	h.persist(c)
}
func (h *Handler) DeleteTraeKey(c *gin.Context) {
	if val := strings.TrimSpace(c.Query("api-key")); val != "" {
		if baseRaw, okBase := c.GetQuery("base-url"); okBase {
			base := strings.TrimSpace(baseRaw)
			out := make([]config.TraeKey, 0, len(h.cfg.TraeKey))
			for _, v := range h.cfg.TraeKey {
				if strings.TrimSpace(v.APIKey) == val && strings.TrimSpace(v.BaseURL) == base {
					continue
				}
				out = append(out, v)
			}
			h.cfg.TraeKey = out
			h.cfg.SanitizeTraeKeys()
			h.persist(c)
			return
		}

		matchIndex := -1
		matchCount := 0
		for i := range h.cfg.TraeKey {
			if strings.TrimSpace(h.cfg.TraeKey[i].APIKey) == val {
				matchCount++
				if matchIndex == -1 {
					matchIndex = i
				}
			}
		}
		if matchCount > 1 {
			c.JSON(400, gin.H{"error": "multiple items match api-key; base-url is required"})
			return
		}
		if matchIndex != -1 {
			h.cfg.TraeKey = append(h.cfg.TraeKey[:matchIndex], h.cfg.TraeKey[matchIndex+1:]...)
		}
		h.cfg.SanitizeTraeKeys()
		h.persist(c)
		return
	}
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		_, err := fmt.Sscanf(idxStr, "%d", &idx)
		if err == nil && idx >= 0 && idx < len(h.cfg.TraeKey) {
			h.cfg.TraeKey = append(h.cfg.TraeKey[:idx], h.cfg.TraeKey[idx+1:]...)
			h.cfg.SanitizeTraeKeys()
			h.persist(c)
			return
		}
	}
	c.JSON(400, gin.H{"error": "missing api-key or index"})
}

func normalizeTraeKey(entry *config.TraeKey) {
	if entry == nil {
		return
	}
	entry.APIKey = strings.TrimSpace(entry.APIKey)
	entry.RefreshToken = strings.TrimSpace(entry.RefreshToken)
	entry.Prefix = strings.TrimSpace(entry.Prefix)
	entry.BaseURL = strings.TrimSpace(entry.BaseURL)
	entry.ProxyURL = strings.TrimSpace(entry.ProxyURL)
	entry.Headers = config.NormalizeHeaders(entry.Headers)
	entry.ExcludedModels = config.NormalizeExcludedModels(entry.ExcludedModels)
	if len(entry.Models) == 0 {
		return
	}
	normalized := make([]config.TraeModel, 0, len(entry.Models))
	for i := range entry.Models {
		model := entry.Models[i]
		model.Name = strings.TrimSpace(model.Name)
		model.DisplayName = strings.TrimSpace(model.DisplayName)
		model.ConfigName = strings.TrimSpace(model.ConfigName)
		model.ModelName = strings.TrimSpace(model.ModelName)
		if model.Name == "" && model.ModelName == "" {
			continue
		}
		normalized = append(normalized, model)
	}
	entry.Models = normalized
}

// TestTraeKey sends a lightweight request to Trae Gateway to validate credentials.
func (h *Handler) TestTraeKey(c *gin.Context) {
	var body struct {
		BaseURL      string `json:"baseUrl"`
		APIKey       string `json:"apiKey"`
		RefreshToken string `json:"refreshToken"`
		ConfigName   string `json:"configName"`
		ModelName    string `json:"modelName"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}

	baseURL := strings.TrimSpace(body.BaseURL)
	if baseURL == "" {
		baseURL = "https://console.enterprise.trae.cn"
	}
	apiKey := strings.TrimSpace(body.APIKey)
	refreshToken := strings.TrimSpace(body.RefreshToken)
	if apiKey == "" && refreshToken == "" {
		c.JSON(400, gin.H{"error": "api key or refresh token is required"})
		return
	}

	configName := strings.TrimSpace(body.ConfigName)
	if configName == "" {
		c.JSON(400, gin.H{"error": "config name is required"})
		return
	}
	modelName := strings.TrimSpace(body.ModelName)
	if modelName == "" {
		c.JSON(400, gin.H{"error": "model name is required"})
		return
	}

	// If only refresh token is provided, exchange it for a JWT
	if apiKey == "" && refreshToken != "" {
		result, err := executor.ExchangeTraeToken(c.Request.Context(), baseURL, refreshToken)
		if err != nil {
			c.JSON(200, gin.H{"success": false, "message": fmt.Sprintf("token exchange failed: %v", err)})
			return
		}
		apiKey = result.Data.Token
	}

	sessionID := executor.NewTraeID()
	conversationID := executor.NewTraeID()
	agentLoopID := sessionID

	payload := map[string]interface{}{
		"config_name":     configName,
		"conversation_id": conversationID,
		"model_name":      modelName,
		"session_id":      sessionID,
		"user_input":      "Hi",
		"messages": []map[string]interface{}{
			{
				"role":         "user",
				"content":      []map[string]string{{"type": "text", "text": "Hi"}},
				"tool_calls":   []interface{}{},
				"tool_call_id": "",
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to build request"})
		return
	}

	extra, _ := json.Marshal(map[string]interface{}{
		"agent_loop_id":         agentLoopID,
		"api_host":              baseURL,
		"api_key":               apiKey,
		"base_url":              baseURL + "/trae-cli/api/v1/llm/proxy",
		"config_name":           configName,
		"config_source":         1,
		"display_name":          configName,
		"model_name":            modelName,
		"proxy_version":         "",
		"real_api_key":          "",
		"real_base_url":         "",
		"session_id":            conversationID,
		"user_prompt_submit_id": sessionID,
	})

	url := strings.TrimRight(baseURL, "/") + "/api/ide/v2/llm_raw_chat"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, strings.NewReader(string(encoded)))
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to create request"})
		return
	}
	req.Header.Set("Authorization", "Cloud-IDE-JWT "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Id", "7b3f9dc2-8a4e-5c6d-2f1b-9e4a3c5b7df0")
	req.Header.Set("X-Ide-Function", "chat")
	req.Header.Set("X-Ide-Version", "99.99.99")
	req.Header.Set("X-Ide-Version-Code", "20260206")
	req.Header.Set("X-Flow-Traceparent", executor.GenerateTraceparent())
	req.Header.Set("Extra", string(extra))

	resp, err := executor.TraeHTTPClient.Do(req)
	if err != nil {
		c.JSON(200, gin.H{"success": false, "message": err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		c.JSON(200, gin.H{"success": false, "message": fmt.Sprintf("gateway returned %d: %s", resp.StatusCode, string(bodyBytes))})
		return
	}

	// Read first few chunks of the SSE stream to detect model errors
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	bodyStr := string(bodyBytes)

	// Check for common error patterns in the response
	lowerBody := strings.ToLower(bodyStr)
	if strings.Contains(lowerBody, `"error"`) || strings.Contains(lowerBody, `"err"`) ||
		strings.Contains(lowerBody, `invalid`) ||
		strings.Contains(lowerBody, `not found`) || strings.Contains(lowerBody, `not exist`) ||
		strings.Contains(lowerBody, `unauthorized`) || strings.Contains(lowerBody, `forbidden`) ||
		strings.Contains(lowerBody, `bad request`) || strings.Contains(lowerBody, `unknown model`) ||
		strings.Contains(lowerBody, `model not`) || strings.Contains(lowerBody, `unsupported`) ||
		strings.Contains(lowerBody, `"code":`) {
		// Extract the first data line for error details
		lines := strings.Split(bodyStr, "\n")
		var detail string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				detail = strings.TrimPrefix(line, "data:")
				detail = strings.TrimSpace(detail)
				if detail != "" && detail != "[DONE]" {
					break
				}
			}
		}
		if detail == "" {
			detail = bodyStr
		}
		if len(detail) > 256 {
			detail = detail[:256] + "..."
		}
		c.JSON(200, gin.H{"success": false, "message": fmt.Sprintf("model rejected by gateway: %s", detail)})
		return
	}

	c.JSON(200, gin.H{"success": true, "message": "connection successful"})
}

// ImportTraeModels calls the Trae get_config_list API and returns model suggestions.
func (h *Handler) ImportTraeModels(c *gin.Context) {
	var body struct {
		BaseURL      string `json:"baseUrl"`
		APIKey       string `json:"apiKey"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}

	baseURL := strings.TrimSpace(body.BaseURL)
	if baseURL == "" {
		baseURL = "https://console.enterprise.trae.cn"
	}
	apiKey := strings.TrimSpace(body.APIKey)
	refreshToken := strings.TrimSpace(body.RefreshToken)

	configs, jwt, err := executor.FetchTraeConfigList(c.Request.Context(), baseURL, apiKey, refreshToken)
	if err != nil {
		c.JSON(200, gin.H{"success": false, "message": err.Error()})
		return
	}

	// parseDisplayName extracts display_name from display_config JSON.
	parseDisplayName := func(raw json.RawMessage) string {
		if len(raw) == 0 {
			return ""
		}
		var obj struct {
			DisplayName string `json:"display_name"`
		}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return ""
		}
		return obj.DisplayName
	}

	type modelSuggestion struct {
		Name         string `json:"name"`
		DisplayName  string `json:"displayName"`
		ConfigName   string `json:"configName"`
		ModelName    string `json:"modelName"`
		ContextLength int64 `json:"context_length,omitempty"`
	}
	var models []modelSuggestion
	for _, cfg := range configs {
		if cfg.Usage != "chat_completion" && cfg.Usage != "" {
			continue
		}
		if cfg.ConfigName == "custom_model_placeholder" || cfg.ConfigName == "summary" {
			continue
		}
		displayName := parseDisplayName(cfg.DisplayConfig)
		if displayName == "" {
			displayName = cfg.ConfigName
		}
		for _, detail := range cfg.ModelDetailList {
			modelName := detail.ModelName
			if modelName == "" {
				modelName = cfg.ConfigName
			}
			suggestion := modelSuggestion{
				Name:        cfg.ConfigName,
				DisplayName: displayName,
				ConfigName:  cfg.ConfigName,
				ModelName:   modelName,
			}
			if detail.PromptMaxTokens > 0 {
				suggestion.ContextLength = int64(detail.PromptMaxTokens)
			} else if upstream := registry.LookupStaticModelInfo(modelName); upstream != nil && upstream.ContextLength > 0 {
				suggestion.ContextLength = int64(upstream.ContextLength)
			} else if upstream := registry.LookupStaticModelInfo(cfg.ConfigName); upstream != nil && upstream.ContextLength > 0 {
				suggestion.ContextLength = int64(upstream.ContextLength)
			}
			models = append(models, suggestion)
		}
		// Configs without model_detail_list (like kimi-k2, deepseek-V3.1)
		if len(cfg.ModelDetailList) == 0 && cfg.Usage == "chat_completion" {
			suggestion := modelSuggestion{
				Name:        cfg.ConfigName,
				DisplayName: displayName,
				ConfigName:  cfg.ConfigName,
				ModelName:   cfg.ConfigName,
			}
			if upstream := registry.LookupStaticModelInfo(cfg.ConfigName); upstream != nil && upstream.ContextLength > 0 {
				suggestion.ContextLength = int64(upstream.ContextLength)
			}
			models = append(models, suggestion)
		}
	}

	c.JSON(200, gin.H{
		"success": true,
		"models":  models,
		"apiKey":  jwt,
	})
}

func normalizeVertexCompatKey(entry *config.VertexCompatKey) {
	if entry == nil {
		return
	}
	entry.APIKey = strings.TrimSpace(entry.APIKey)
	entry.Prefix = strings.TrimSpace(entry.Prefix)
	entry.BaseURL = strings.TrimSpace(entry.BaseURL)
	entry.ProxyURL = strings.TrimSpace(entry.ProxyURL)
	entry.Headers = config.NormalizeHeaders(entry.Headers)
	entry.ExcludedModels = config.NormalizeExcludedModels(entry.ExcludedModels)
	if len(entry.Models) == 0 {
		return
	}
	normalized := make([]config.VertexCompatModel, 0, len(entry.Models))
	for i := range entry.Models {
		model := entry.Models[i]
		model.Name = strings.TrimSpace(model.Name)
		model.Alias = strings.TrimSpace(model.Alias)
		if model.Name == "" || model.Alias == "" {
			continue
		}
		normalized = append(normalized, model)
	}
	entry.Models = normalized
}

func sanitizedOAuthModelAlias(entries map[string][]config.OAuthModelAlias) map[string][]config.OAuthModelAlias {
	if len(entries) == 0 {
		return nil
	}
	copied := make(map[string][]config.OAuthModelAlias, len(entries))
	for channel, aliases := range entries {
		if len(aliases) == 0 {
			continue
		}
		copied[channel] = append([]config.OAuthModelAlias(nil), aliases...)
	}
	if len(copied) == 0 {
		return nil
	}
	cfg := config.Config{OAuthModelAlias: copied}
	cfg.SanitizeOAuthModelAlias()
	if len(cfg.OAuthModelAlias) == 0 {
		return nil
	}
	return cfg.OAuthModelAlias
}

// GetAmpCode returns the complete ampcode configuration.
func (h *Handler) GetAmpCode(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(200, gin.H{"ampcode": config.AmpCode{}})
		return
	}
	c.JSON(200, gin.H{"ampcode": h.cfg.AmpCode})
}

// GetAmpUpstreamURL returns the ampcode upstream URL.
func (h *Handler) GetAmpUpstreamURL(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(200, gin.H{"upstream-url": ""})
		return
	}
	c.JSON(200, gin.H{"upstream-url": h.cfg.AmpCode.UpstreamURL})
}

// PutAmpUpstreamURL updates the ampcode upstream URL.
func (h *Handler) PutAmpUpstreamURL(c *gin.Context) {
	h.updateStringField(c, func(v string) { h.cfg.AmpCode.UpstreamURL = strings.TrimSpace(v) })
}

// DeleteAmpUpstreamURL clears the ampcode upstream URL.
func (h *Handler) DeleteAmpUpstreamURL(c *gin.Context) {
	h.cfg.AmpCode.UpstreamURL = ""
	h.persist(c)
}

// GetAmpUpstreamAPIKey returns the ampcode upstream API key.
func (h *Handler) GetAmpUpstreamAPIKey(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(200, gin.H{"upstream-api-key": ""})
		return
	}
	c.JSON(200, gin.H{"upstream-api-key": h.cfg.AmpCode.UpstreamAPIKey})
}

// PutAmpUpstreamAPIKey updates the ampcode upstream API key.
func (h *Handler) PutAmpUpstreamAPIKey(c *gin.Context) {
	h.updateStringField(c, func(v string) { h.cfg.AmpCode.UpstreamAPIKey = strings.TrimSpace(v) })
}

// DeleteAmpUpstreamAPIKey clears the ampcode upstream API key.
func (h *Handler) DeleteAmpUpstreamAPIKey(c *gin.Context) {
	h.cfg.AmpCode.UpstreamAPIKey = ""
	h.persist(c)
}

// GetAmpRestrictManagementToLocalhost returns the localhost restriction setting.
func (h *Handler) GetAmpRestrictManagementToLocalhost(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(200, gin.H{"restrict-management-to-localhost": true})
		return
	}
	c.JSON(200, gin.H{"restrict-management-to-localhost": h.cfg.AmpCode.RestrictManagementToLocalhost})
}

// PutAmpRestrictManagementToLocalhost updates the localhost restriction setting.
func (h *Handler) PutAmpRestrictManagementToLocalhost(c *gin.Context) {
	h.updateBoolField(c, func(v bool) { h.cfg.AmpCode.RestrictManagementToLocalhost = v })
}

// GetAmpModelMappings returns the ampcode model mappings.
func (h *Handler) GetAmpModelMappings(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(200, gin.H{"model-mappings": []config.AmpModelMapping{}})
		return
	}
	c.JSON(200, gin.H{"model-mappings": h.cfg.AmpCode.ModelMappings})
}

// PutAmpModelMappings replaces all ampcode model mappings.
func (h *Handler) PutAmpModelMappings(c *gin.Context) {
	var body struct {
		Value []config.AmpModelMapping `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	h.cfg.AmpCode.ModelMappings = body.Value
	h.persist(c)
}

// PatchAmpModelMappings adds or updates model mappings.
func (h *Handler) PatchAmpModelMappings(c *gin.Context) {
	var body struct {
		Value []config.AmpModelMapping `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}

	existing := make(map[string]int)
	for i, m := range h.cfg.AmpCode.ModelMappings {
		existing[strings.TrimSpace(m.From)] = i
	}

	for _, newMapping := range body.Value {
		from := strings.TrimSpace(newMapping.From)
		if idx, ok := existing[from]; ok {
			h.cfg.AmpCode.ModelMappings[idx] = newMapping
		} else {
			h.cfg.AmpCode.ModelMappings = append(h.cfg.AmpCode.ModelMappings, newMapping)
			existing[from] = len(h.cfg.AmpCode.ModelMappings) - 1
		}
	}
	h.persist(c)
}

// DeleteAmpModelMappings removes specified model mappings by "from" field.
func (h *Handler) DeleteAmpModelMappings(c *gin.Context) {
	var body struct {
		Value []string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Value) == 0 {
		h.cfg.AmpCode.ModelMappings = nil
		h.persist(c)
		return
	}

	toRemove := make(map[string]bool)
	for _, from := range body.Value {
		toRemove[strings.TrimSpace(from)] = true
	}

	newMappings := make([]config.AmpModelMapping, 0, len(h.cfg.AmpCode.ModelMappings))
	for _, m := range h.cfg.AmpCode.ModelMappings {
		if !toRemove[strings.TrimSpace(m.From)] {
			newMappings = append(newMappings, m)
		}
	}
	h.cfg.AmpCode.ModelMappings = newMappings
	h.persist(c)
}

// GetAmpForceModelMappings returns whether model mappings are forced.
func (h *Handler) GetAmpForceModelMappings(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(200, gin.H{"force-model-mappings": false})
		return
	}
	c.JSON(200, gin.H{"force-model-mappings": h.cfg.AmpCode.ForceModelMappings})
}

// PutAmpForceModelMappings updates the force model mappings setting.
func (h *Handler) PutAmpForceModelMappings(c *gin.Context) {
	h.updateBoolField(c, func(v bool) { h.cfg.AmpCode.ForceModelMappings = v })
}

// GetAmpUpstreamAPIKeys returns the ampcode upstream API keys mapping.
func (h *Handler) GetAmpUpstreamAPIKeys(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(200, gin.H{"upstream-api-keys": []config.AmpUpstreamAPIKeyEntry{}})
		return
	}
	c.JSON(200, gin.H{"upstream-api-keys": h.cfg.AmpCode.UpstreamAPIKeys})
}

// PutAmpUpstreamAPIKeys replaces all ampcode upstream API keys mappings.
func (h *Handler) PutAmpUpstreamAPIKeys(c *gin.Context) {
	var body struct {
		Value []config.AmpUpstreamAPIKeyEntry `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	// Normalize entries: trim whitespace, filter empty
	normalized := normalizeAmpUpstreamAPIKeyEntries(body.Value)
	h.cfg.AmpCode.UpstreamAPIKeys = normalized
	h.persist(c)
}

// PatchAmpUpstreamAPIKeys adds or updates upstream API keys entries.
// Matching is done by upstream-api-key value.
func (h *Handler) PatchAmpUpstreamAPIKeys(c *gin.Context) {
	var body struct {
		Value []config.AmpUpstreamAPIKeyEntry `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}

	existing := make(map[string]int)
	for i, entry := range h.cfg.AmpCode.UpstreamAPIKeys {
		existing[strings.TrimSpace(entry.UpstreamAPIKey)] = i
	}

	for _, newEntry := range body.Value {
		upstreamKey := strings.TrimSpace(newEntry.UpstreamAPIKey)
		if upstreamKey == "" {
			continue
		}
		normalizedEntry := config.AmpUpstreamAPIKeyEntry{
			UpstreamAPIKey: upstreamKey,
			APIKeys:        normalizeAPIKeysList(newEntry.APIKeys),
		}
		if idx, ok := existing[upstreamKey]; ok {
			h.cfg.AmpCode.UpstreamAPIKeys[idx] = normalizedEntry
		} else {
			h.cfg.AmpCode.UpstreamAPIKeys = append(h.cfg.AmpCode.UpstreamAPIKeys, normalizedEntry)
			existing[upstreamKey] = len(h.cfg.AmpCode.UpstreamAPIKeys) - 1
		}
	}
	h.persist(c)
}

// DeleteAmpUpstreamAPIKeys removes specified upstream API keys entries.
// Body must be JSON: {"value": ["<upstream-api-key>", ...]}.
// If "value" is an empty array, clears all entries.
// If JSON is invalid or "value" is missing/null, returns 400 and does not persist any change.
func (h *Handler) DeleteAmpUpstreamAPIKeys(c *gin.Context) {
	var body struct {
		Value []string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}

	if body.Value == nil {
		c.JSON(400, gin.H{"error": "missing value"})
		return
	}

	// Empty array means clear all
	if len(body.Value) == 0 {
		h.cfg.AmpCode.UpstreamAPIKeys = nil
		h.persist(c)
		return
	}

	toRemove := make(map[string]bool)
	for _, key := range body.Value {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		toRemove[trimmed] = true
	}
	if len(toRemove) == 0 {
		c.JSON(400, gin.H{"error": "empty value"})
		return
	}

	newEntries := make([]config.AmpUpstreamAPIKeyEntry, 0, len(h.cfg.AmpCode.UpstreamAPIKeys))
	for _, entry := range h.cfg.AmpCode.UpstreamAPIKeys {
		if !toRemove[strings.TrimSpace(entry.UpstreamAPIKey)] {
			newEntries = append(newEntries, entry)
		}
	}
	h.cfg.AmpCode.UpstreamAPIKeys = newEntries
	h.persist(c)
}

// normalizeAmpUpstreamAPIKeyEntries normalizes a list of upstream API key entries.
func normalizeAmpUpstreamAPIKeyEntries(entries []config.AmpUpstreamAPIKeyEntry) []config.AmpUpstreamAPIKeyEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]config.AmpUpstreamAPIKeyEntry, 0, len(entries))
	for _, entry := range entries {
		upstreamKey := strings.TrimSpace(entry.UpstreamAPIKey)
		if upstreamKey == "" {
			continue
		}
		apiKeys := normalizeAPIKeysList(entry.APIKeys)
		out = append(out, config.AmpUpstreamAPIKeyEntry{
			UpstreamAPIKey: upstreamKey,
			APIKeys:        apiKeys,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeAPIKeysList trims and filters empty strings from a list of API keys.
func normalizeAPIKeysList(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		trimmed := strings.TrimSpace(k)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
