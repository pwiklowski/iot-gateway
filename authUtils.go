package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/tidwall/gjson"
)

const (
	AUTH_HUB   = "AUTH_HUB"
	AUTH_WEB   = "AUTH_WEB"
	AUTH_ALEXA = "AUTH_ALEXA"
)

type AuthUserData struct {
	Active   bool   `json:"active"`
	Username string `json:"username"`
}

type OAuthData struct {
	Client string
	Secret string
}

func redirectPolicyFunc(req *http.Request, via []*http.Request) error {
	auth := getAuthData(AUTH_WEB)
	req.SetBasicAuth(auth.Client, auth.Secret)
	return nil
}

func GetUserInfo(token string, auth *OAuthData) (user *AuthUserData, e error) {
	form := url.Values{
		"token":           {token},
		"token_type_hint": {"access_token"},
	}
	client := &http.Client{
		CheckRedirect: redirectPolicyFunc,
	}

	body := bytes.NewReader([]byte(form.Encode()))
	req, err := http.NewRequest("POST", "https://auth.wiklosoft.com/v1/oauth/introspect", body)
	req.Header.Add("Content-type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(auth.Client, auth.Secret)
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return &AuthUserData{}, err
	}
	defer resp.Body.Close()

	userData := &AuthUserData{}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return &AuthUserData{}, err
	}
	r := gjson.ParseBytes(bodyBytes)

	log.Println(r)

	userData.Username = r.Get("username").String()
	userData.Active = r.Get("active").Bool()

	return userData, nil
}

func getAuthData(authType string) *OAuthData {
	authData := &OAuthData{}
	if authType == AUTH_HUB {
		authData.Client = os.Getenv("AUTH_HUB_CLIENT")
		authData.Secret = os.Getenv("AUTH_HUB_CLIENT_SECRET")
	} else if authType == AUTH_ALEXA {
		authData.Client = os.Getenv("AUTH_ALEXA_CLIENT")
		authData.Secret = os.Getenv("AUTH_ALEXA_CLIENT_SECRET")
	} else if authType == AUTH_WEB {
		authData.Client = os.Getenv("AUTH_WEB_CLIENT")
		authData.Secret = os.Getenv("AUTH_WEB_CLIENT_SECRET")
	}

	return authData
}
