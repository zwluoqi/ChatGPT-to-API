package main

import (
	"encoding/json"
	"fmt"
	chatgpt_request_converter "freechatgpt/conversion/requests/chatgpt"
	chatgpt "freechatgpt/internal/chatgpt"
	"freechatgpt/internal/tokens"
	"freechatgpt/moderation"
	official_types "freechatgpt/typings/official"
	"github.com/gin-gonic/gin"
	"os"
	"strings"
)

var request35Count int
var request40Count int

func passwordHandler(c *gin.Context) {
	// Get the password from the request (json) and update the password
	type password_struct struct {
		Password string `json:"password"`
	}
	var password password_struct
	err := c.BindJSON(&password)
	if err != nil {
		c.String(400, "password not provided")
		return
	}
	ADMIN_PASSWORD = password.Password
	// Set environment variable
	os.Setenv("ADMIN_PASSWORD", ADMIN_PASSWORD)
	c.String(200, "password updated")
}

func tokensHandler(c *gin.Context) {
	// Get the request_tokens from the request (json) and update the request_tokens
	var request_tokens map[string]tokens.Secret
	err := c.BindJSON(&request_tokens)
	if err != nil {
		c.String(400, "tokens not provided")
		return
	}
	ACCESS_TOKENS = tokens.NewAccessToken(request_tokens)
	ACCESS_TOKENS.Save()
	validAccounts = ACCESS_TOKENS.GetKeys()
	c.String(200, "tokens updated")
}
func optionsHandler(c *gin.Context) {
	// Set headers for CORS
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "POST")
	c.Header("Access-Control-Allow-Headers", "*")
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

func simulateModel(c *gin.Context) {
	c.JSON(200, gin.H{
		"object": "list",
		"data": []gin.H{
			{
				"id":       "gpt-3.5-turbo",
				"object":   "model",
				"created":  1688888888,
				"owned_by": "chatgpt-to-api",
			},
			{
				"id":       "gpt-4",
				"object":   "model",
				"created":  1688888888,
				"owned_by": "chatgpt-to-api",
			},
		},
	})
}
func nightmare(c *gin.Context) {
	var original_request official_types.APIRequest
	err := c.BindJSON(&original_request)
	if err != nil {
		c.JSON(400, gin.H{"error": gin.H{
			"message": "Request must be proper JSON",
			"type":    "invalid_request_error",
			"param":   nil,
			"code":    err.Error(),
		}})
		return
	}

	if !strings.HasPrefix(original_request.Model, "gpt-3.5") {
		completionMessages := original_request.Messages
		textMarshal, err := json.Marshal(completionMessages)
		if err != nil {
			fmt.Println("Text marshal err:", err)
		}
		message := string(textMarshal)
		moderationresponse, err := moderation.PostModerationData(message)
		if err != nil {
			fmt.Println("Error:", err)
		} else {
			if moderationresponse.Results[0].CategoryScores.Sexual > 0.8 {
				textMarshal, err := json.Marshal(moderationresponse.Results)
				if err != nil {
					fmt.Println("Text marshal err:", err)
				}
				c.JSON(444, gin.H{"error": gin.H{
					"message": "消息没有通过moderation审核:\n" + string(textMarshal),
					"type":    "invalid_request_error",
					"param":   nil,
					"code":    444,
				}})
				return
			}
		}
		request40Count++
	} else {
		request35Count++
	}

	authHeader := c.GetHeader("Authorization")
	token, puid := getSecret(original_request.Model)
	if authHeader != "" {
		customAccessToken := strings.Replace(authHeader, "Bearer ", "", 1)
		// Check if customAccessToken starts with sk-
		if strings.HasPrefix(customAccessToken, "eyJhbGciOiJSUzI1NiI") {
			token = customAccessToken
		}
	}
	fmt.Println("request35Count:", request35Count, "request40Count:", request40Count)

	var proxy_url string
	// if len(proxies) == 0 {
	// 	proxy_url = ""
	// } else {
	// 	proxy_url = proxies[0]
	// 	// Push used proxy to the back of the list
	// 	proxies = append(proxies[1:], proxies[0])
	// }

	//不要删除了，删了好像也没用
	fmt.Println("pre delete")
	responseConvs, err := chatgpt.GetConversations(0, 28, token, puid)
	if responseConvs != nil {
		// Determine the length of the array
		length := len(responseConvs)
		// Loop through the array, stopping before the last 5 elements
		for i := length - 1; i > 5; i-- {
			ele := responseConvs[i]
			del_response, del_err := chatgpt.DeleteConversation(ele.ID, token, puid, proxy_url)
			fmt.Println("delete response.StatusCode", del_response.StatusCode)
			if err != nil {
				fmt.Println(del_err)
				return
			}
			defer del_response.Body.Close()
		}
	}
	fmt.Println("post delete")
	// Convert the chat request to a ChatGPT request
	translated_request := chatgpt_request_converter.ConvertAPIRequest(original_request, puid, proxy_url)
	// fmt.Println("ConversationID", translated_request.ConversationID)
	// fmt.Println("ParentMessageID", translated_request.ParentMessageID)

	fmt.Println("post POSTconversation")
	response, err := chatgpt.POSTconversation(translated_request, token, puid, proxy_url)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "error sending request",
		})
		return
	}

	defer response.Body.Close()
	if chatgpt.Handle_request_error(c, response) {
		return
	}
	fmt.Println("post POSTconversation")

	var full_response string
	for i := 3; i > 0; i-- {
		var continue_info *chatgpt.ContinueInfo
		var response_part string
		response_part, continue_info = chatgpt.Handler(c, response, token, puid, translated_request, original_request.Stream)
		full_response += response_part
		if continue_info == nil {
			break
		}
		println("Continuing conversation")
		translated_request.Messages = nil
		translated_request.Action = "continue"
		translated_request.ConversationID = continue_info.ConversationID
		translated_request.ParentMessageID = continue_info.ParentID
		if strings.HasPrefix(original_request.Model, "gpt-4") {
			chatgpt_request_converter.RenewTokenForRequest(&translated_request, puid, proxy_url)
		}
		response, err = chatgpt.POSTconversation(translated_request, token, puid, proxy_url)
		if err != nil {
			c.JSON(500, gin.H{
				"error": "error sending request",
			})
			return
		}
		defer response.Body.Close()
		if chatgpt.Handle_request_error(c, response) {
			return
		}
	}
	if c.Writer.Status() != 200 {
		return
	}
	if !original_request.Stream {
		c.JSON(200, official_types.NewChatCompletion(full_response))
	} else {
		c.String(200, "data: [DONE]\n\n")
	}

	// fmt.Println("ConversationID", ConversationID)
	// del_response, del_err := chatgpt.DeleteConversation(ConversationID, token, proxy_url)
	// fmt.Println("response.StatusCode", del_response.StatusCode)
	// if err != nil {
	// 	fmt.Println(del_err)
	// 	return
	// }
	// defer del_response.Body.Close()
}
