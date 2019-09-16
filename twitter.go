package main

import (
	"encoding/json"
	"github.com/garyburd/go-oauth/oauth"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joeshaw/envdecode"

)

var conn net.Conn

func dial(netw, addr string) (net.Conn, error) {
	if conn != nil {
		conn.Close()
		conn = nil
	}
	netc, err := net.DialTimeout(netw, addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	conn = netc
	return netc, nil
}

var reader io.ReadCloser

func closeConn()  {
	if conn != nil {
		conn.Close()
	}
	if reader != nil {
		reader.Close()
	}
}

var (
	authClient *oauth.Client
	creds *oauth.Credentials
)

func setupTwitterAuth()  {
	var ts struct{
		ConsumerKey string `env:"SP_TWITTER_KEY,required"`
		ConsumerSecret string `env:"SP_TWITTER_SECRET, required"`
		AccessToken string `env:"SP_TWITTER_ACCESSTOKEN, required"`
		AccessSecret string `env:"SP_TWITTER_ACCESSSECRET, required"`
	}
	if err := envdecode.Decode(&ts); err != nil {
		log.Fatalln(err)
	}
	creds = &oauth.Credentials {
		Token: ts.AccessToken,
		Secret: ts.AccessSecret,
	}
	authClient = &oauth.Client{
		Credentials: oauth.Credentials {
			Token: ts.ConsumerKey,
			Secret: ts.ConsumerSecret,
		},
	}
}

var (
	authSetupOnce sync.Once
	httpClient *http.Client
)

func makeRequest(req *http.Request, params url.Values) (*http.Response, error) {
  authSetupOnce.Do(func() {
	  setupTwitterAuth()
	  httpClient = &http.Client{
		  Transport: &http.Transport{
            Dial: dial,
		  },
	  }
  })
  formEnc := params.Encode()
  req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
  req.Header.Set("Content-Length", strconv.Itoa(len(formEnc)))
  req.Header.Set("Authorization", authClient.AuthorizationHeader(creds, "POST", req.URL, params))
  return httpClient.Do(req)
}

type tweet struct {
	Text string
}

func readFromTwitter(votes chan<- string)  {
	options, err := loadOptions()
	log.Println("optionsの値")
	log.Println(options)
	if err != nil {
		log.Println("URLの読み込みに失敗しました：", err)
		return
	}
	u, err := url.Parse("https://stream.twitter.com/1.1/statuses/filter.json")
	if err != nil {
		log.Println("URLの解析に失敗しました：", err)
		return
	}
	query := make(url.Values)
	query.Set("track", strings.Join(options, ","))
	req, err := http.NewRequest("POST", u.String(), strings.NewReader(query.Encode()))
	log.Println("reqの値")
	log.Println(req)
	if err != nil {
		log.Println("検索のリクエストの作成に失敗しました：", err)
		return
	}
	resp, err := makeRequest(req, query)
	log.Println("respの値")
	log.Println(resp)
	if err != nil {
		log.Println("検索のリクエストに失敗しました：", err)
		return
	}
	reader := resp.Body
	log.Println("readerの値")
	log.Println(reader)
	decoder := json.NewDecoder(reader)
	log.Println("decoderの値")
	log.Println(decoder)
	for {
		var tweet tweet
		if err := decoder.Decode(&tweet); err != nil {
			break
		}
		for _, option := range options {
			if strings.Contains(strings.ToLower(tweet.Text), strings.ToLower(option)) {
				log.Println("投票：", option)
				votes <- option
			}
		}
	}
}

func startTwitterStream(stopchan <-chan struct{}, votes chan<- string) <-chan struct{}{
	stoppedchan := make(chan struct{}, 1)
	go func() {
		defer func() {
			stoppedchan <- struct{}{}
		}()
		for {
			select {
			case <- stopchan:
				log.Println("Twitterへの問い合わせを終了します...")
				return
			default:
				log.Println("Twitterに問い合わせます...")
				readFromTwitter(votes)
				log.Println("(待機中)")
				time.Sleep(10 * time.Second)//待機してから接続します
			}
		}
	}()
	return stoppedchan
}
