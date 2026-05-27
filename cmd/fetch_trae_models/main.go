package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

var transport = &http.Transport{
	DialContext: (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	MaxIdleConns: 100,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout: 90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
	ForceAttemptHTTP2: true,
	TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1"}},
}

var client = &http.Client{Transport: transport, Timeout: 30 * time.Second}

func main() {
	host := "https://console.enterprise.trae.cn"
	refreshToken := "trae-lt-4c2508e625dd11126d2ce1db046421bc126b7db7bbf8de0ba3eec10d"

	body, _ := json.Marshal(map[string]string{"RefreshToken": refreshToken})
	req, _ := http.NewRequest(http.MethodPost, host+"/cloudide/api/v3/trae/oauth/ExchangeToken", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cloudide-Token", "")

	resp, _ := client.Do(req)
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var r struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"Token"`
		} `json:"Data"`
	}
	json.Unmarshal(respBody, &r)
	jwt := r.Data.Token

	headers := map[string]string{
		"Authorization": "Cloud-IDE-JWT " + jwt,
		"X-Ide-Version": "99.99.99",
		"X-Ide-Version-Code": "20260206",
		"X-App-Id": "7b3f9dc2-8a4e-5c6d-2f1b-9e4a3c5b7df0",
	}

	// Try 1: GET without configName (full list)
	fmt.Println("=== GET (no params) ===")
	doReq("GET", host+"/api/ide/v1/cli/get_config_list", headers, nil)

	// Try 2: POST without body
	fmt.Println("\n=== POST (empty body) ===")
	doReq("POST", host+"/api/ide/v1/cli/get_config_list", headers, []byte("{}"))

	// Try 3: POST with config_name in body
	fmt.Println("\n=== POST config_name=deepseek-V4-Pro ===")
	doReq("POST", host+"/api/ide/v1/cli/get_config_list", headers, []byte(`{"config_name":"deepseek-V4-Pro"}`))

	// Try 4: POST with ConfigName
	fmt.Println("\n=== POST ConfigName=deepseek-V4-Pro ===")
	doReq("POST", host+"/api/ide/v1/cli/get_config_list", headers, []byte(`{"ConfigName":"deepseek-V4-Pro"}`))

	// Try 5: GET with X-Ide-Function
	fmt.Println("\n=== GET with X-Ide-Function=chat ===")
	h := make(map[string]string)
	for k, v := range headers { h[k] = v }
	h["X-Ide-Function"] = "chat"
	doReq("GET", host+"/api/ide/v1/cli/get_config_list", h, nil)
}

func doReq(method, url string, headers map[string]string, body []byte) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewRequest error: %v\n", err)
		return
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Printf("Status: %d\n", resp.StatusCode)
	var pretty bytes.Buffer
	json.Indent(&pretty, respBody, "", "  ")
	fmt.Println(pretty.String())
}