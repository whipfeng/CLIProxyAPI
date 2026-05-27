package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func main() {
	body := map[string]string{
		"baseUrl":      "https://console.enterprise.trae.cn",
		"apiKey":       "",
		"refreshToken": "trae-lt-632de91a4265ab01cb1f761920178e705f734a3b43025104d60a51d9",
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "http://127.0.0.1:8317/v0/management/trae-api-key/import", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer $2a$10$ptXaZaipmlvDzRmCIed95.wLJQy7xXIXf4H/qS7KhO3tpwIxI7.H.")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	fmt.Println("Status:", resp.StatusCode)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	if success, _ := result["success"].(bool); success {
		if models, ok := result["models"].([]interface{}); ok {
			fmt.Printf("Total: %d models\n", len(models))
			for i, m := range models {
				if i >= 10 { break }
				if mm, ok := m.(map[string]interface{}); ok {
					name, _ := mm["name"].(string)
					cl := mm["context_length"]
					fmt.Printf("  [%d] %s cl=%v\n", i, name, cl)
				}
			}
		}
	} else {
		msg, _ := result["message"].(string)
		fmt.Println("Failed:", msg)
		s := string(respBody)
		if len(s) > 300 { s = s[:300] }
		fmt.Println(s)
	}
}
