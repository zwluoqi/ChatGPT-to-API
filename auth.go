package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"freechatgpt/internal/tokens"

	"github.com/xqdoo00o/OpenAIAuth/auth"

	"github.com/golang-jwt/jwt/v5"
)

var accounts []Account

var validAccounts []string

const interval = time.Hour * 24 * 7

type Account struct {
	Email    string `json:"username"`
	Password string `json:"password"`
}

type Claim struct {
	jwt.RegisteredClaims
}

func getTokenExpire(tokenstring string) (*jwt.NumericDate, error) {
	t, _ := jwt.ParseWithClaims(tokenstring, &Claim{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(""), nil
	})
	issue, _ := t.Claims.GetExpirationTime()

	if issue != nil {
		return issue, nil
	} else {
		return nil, errors.New("invalid token")
	}
}

func AppendIfMissing(slice []string, i string) []string {
	for _, ele := range slice {
		if ele == i {
			return slice
		}
	}
	return append(slice, i)
}

func getSecret() (string, string) {
	account := validAccounts[0]
	validAccounts = append(validAccounts[1:], account)
	return ACCESS_TOKENS.GetSecret(account)
}

// Read accounts.txt and create a list of accounts
func readAccounts() {
	accounts = []Account{}
	// Read accounts.txt and create a list of accounts
	if _, err := os.Stat("accounts.txt"); err == nil {
		// Each line is a proxy, put in proxies array
		file, _ := os.Open("accounts.txt")
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			// Split by :
			line := strings.Split(scanner.Text(), ":")
			// Create an account
			account := Account{
				Email:    line[0],
				Password: line[1],
			}
			// Append to accounts
			accounts = append(accounts, account)
		}
	}
}

func newTimeFunc(account Account, token_list *map[string]tokens.Secret, cron bool) func() {
	return func() {
		updateSingleToken(account, token_list, cron)
	}
}

func scheduleTokenPUID() {
	// Check if access_tokens.json exists
	if stat, err := os.Stat("access_tokens.json"); os.IsNotExist(err) {
		// Create the file
		file, err := os.Create("access_tokens.json")
		if err != nil {
			panic(err)
		}
		defer file.Close()
		updateToken()
	} else {
		file, err := os.Open("access_tokens.json")
		if err != nil {
			panic(err)
		}
		defer file.Close()
		decoder := json.NewDecoder(file)
		var token_list map[string]tokens.Secret
		err = decoder.Decode(&token_list)
		if err != nil {
			updateToken()
			return
		}
		if len(token_list) == 0 {
			updateToken()
		} else {
			ACCESS_TOKENS = tokens.NewAccessToken(token_list)
			validAccounts = []string{}
			for _, account := range accounts {
				token := token_list[account.Email].Token
				if token == "" {
					updateSingleToken(account, nil, true)
				} else {
					var toExpire time.Duration
					nowTime := time.Now()
					expireTime, err := getTokenExpire(token)
					if err != nil {
						toExpire = interval - nowTime.Sub(stat.ModTime())
					} else {
						toExpire = expireTime.Sub(nowTime)
						if toExpire > 0 {
							toExpire = toExpire % interval
						}
					}
					if toExpire > 0 {
						validAccounts = AppendIfMissing(validAccounts, account.Email)
						f := newTimeFunc(account, nil, true)
						time.AfterFunc(toExpire, f)
					} else {
						updateSingleToken(account, nil, true)
					}
				}
			}
		}
	}
}

func updateSingleToken(account Account, token_list *map[string]tokens.Secret, cron bool) {
	if os.Getenv("CF_PROXY") != "" {
		// exec warp-cli disconnect and connect
		exec.Command("warp-cli", "disconnect").Run()
		exec.Command("warp-cli", "connect").Run()
		time.Sleep(5 * time.Second)
	}
	println("Updating access token for " + account.Email)
	var proxy_url string
	if len(proxies) == 0 {
		proxy_url = ""
	} else {
		proxy_url = proxies[0]
		// Push used proxy to the back of the list
		proxies = append(proxies[1:], proxies[0])
	}
	authenticator := auth.NewAuthenticator(account.Email, account.Password, proxy_url)
	err := authenticator.RenewWithCookies()
	if err != nil {
		authenticator.ResetCookies()
		err := authenticator.Begin()
		if err != nil {
			if token_list == nil {
				for i, v := range validAccounts {
					if v == account.Email {
						validAccounts = append(validAccounts[:i], validAccounts[i+1:]...)
						break
					}
				}
			}
			println("Location: " + err.Location)
			println("Status code: " + strconv.Itoa(err.StatusCode))
			println("Details: " + err.Details)
			return
		}
	}
	access_token := authenticator.GetAccessToken()
	puid, _ := authenticator.GetPUID()
	if token_list != nil {
		(*token_list)[account.Email] = tokens.Secret{Token: access_token, PUID: puid}
	} else {
		ACCESS_TOKENS.Set(account.Email, access_token, puid)
		ACCESS_TOKENS.Save()
	}
	validAccounts = AppendIfMissing(validAccounts, account.Email)
	println("Success!")
	err = authenticator.SaveCookies()
	if err != nil {
		println(err.Details)
	}
	if cron {
		f := newTimeFunc(account, token_list, cron)
		time.AfterFunc(interval, f)
	}
}

func updateToken() {
	token_list := map[string]tokens.Secret{}
	validAccounts = []string{}
	// Loop through each account
	for _, account := range accounts {
		updateSingleToken(account, &token_list, false)
	}
	// Append access token to access_tokens.json
	ACCESS_TOKENS = tokens.NewAccessToken(token_list)
	ACCESS_TOKENS.Save()
	time.AfterFunc(interval, updateToken)
}
