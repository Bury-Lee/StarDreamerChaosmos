package utils

import "encoding/json"

func JsonToken(body string) string {
	var r struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &r)
	return r.Data.Token
}
