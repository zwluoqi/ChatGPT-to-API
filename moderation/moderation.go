// moderation/moderation.go

package moderation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

// ModerationData represents the data structure for the API request.
type ModerationData struct {
	Input string `json:"input"`
}

// ModerationResponse represents the expected structure of the API response.
type ModerationResponse struct {
	Results []struct {
		Flagged    bool `json:"flagged"`
		Categories struct {
			Sexual bool `json:"sexual"`
		} `json:"categories"`
		CategoryScores struct {
			Sexual   float32 `json:"sexual"`
			Violence float32 `json:"violence"`
		} `json:"category_scores"`
	} `json:"results"`
}

// PostModerationData sends a POST request to the OpenAI Moderation API and processes the response.
func PostModerationData(message string) (*ModerationResponse, error) {
	url := "https://api.openai.com/v1/moderations"
	apikey := os.Getenv("OPENAI_APIKEY")
	headers := map[string]string{
		"Authorization": "Bearer " + apikey,
		"Content-Type":  "application/json",
	}

	data := ModerationData{
		Input: message,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("错误: %s, %s", resp.Status, string(body)))
	}

	// fmt.Println("PostModerationData Result\n", string(body))
	var response ModerationResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	// Process the response
	for _, result := range response.Results {
		fmt.Printf("Flagged: %t\n", result.Flagged)
		fmt.Printf("Sexual Content Detected: %t\n", result.Categories.Sexual)
		fmt.Printf("Sexual CategorySexual Detected: %f\n", result.CategoryScores.Sexual)
		fmt.Printf("Sexual CategoryViolence Detected: %f\n", result.CategoryScores.Violence)
		// fmt.Println("PostModerationData Result\n", string(body))
	}

	return &response, nil
}
