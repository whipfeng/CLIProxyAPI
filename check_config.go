package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	d, err := os.ReadFile("C:/Users/Docker/Desktop/Workspace/proxy-ai-model/config.yaml")
	if err != nil {
		fmt.Println("Read error:", err)
		return
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(d, &m); err != nil {
		fmt.Println("YAML error:", err)
		return
	}
	cfg := m["trae-api-key"]
	fmt.Printf("trae-api-key: %#v\n", cfg)
}
