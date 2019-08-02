package main

import (
	"time"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"fmt"
	"strings"
	"github.com/gomodule/redigo/redis"
	"encoding/hex"
	"io"
	"crypto/md5"
	"github.com/kataras/iris/middleware/logger"
	"github.com/kataras/iris/middleware/recover"
	"github.com/kataras/iris"
)

// RecaptchaResponse is the struct of json recv from recaptcha.net
type RecaptchaResponse struct {
	Success     bool      `json:"success"`
	ChallengeTs time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	Score       float64   `json:"score"`
	Action      string    `json:"action"`
}


// recaptchacSecret should be change
const (
	recaptchaSecret = ""
)

// Create Redis client instance
var app = iris.New()
var redisClient, err = redis.Dial("tcp", "127.0.0.1:6379")

func init() {
	// check redis conn error
	if err != nil {
		panic(err)
	}

	// Create Iris app
	app.Logger().SetLevel("debug")
	app.Use(recover.New())
	app.Use(logger.New())

	// ViewRegister
	app.RegisterView(iris.HTML("./public", ".html").Reload(true))

	// Static assets Handler
	app.HandleDir("/css", "./public/css")
	app.HandleDir("/img", "./public/img")
}

func main() {
	// Method: GET
	// Main Webpage
	app.Handle("GET", "/", mainPageHandler)

	// Method: POST
	// Recv User input
	app.Post("/paste", inputPageHandler)

	// Method: GET
	// Show RAW data
	app.Get("/{id:string}", textDataHandler)

	// http://localhost:8964
	// http://localhost:8964/paste
	// http://localhost:8964/css
	// http://localhost:8964/{id:string}
	app.Run(iris.Addr(":8082"), iris.WithoutServerError(iris.ErrServerClosed))
}

func inputPageHandler(ctx iris.Context){

	// Verify with recaptcha
	if !verify(ctx) {
		ctx.View("error.html")
		return
	}
	text := ctx.FormValue("text")

	// Generate an ID with md5[0:6]
	textMd5 := md5.New()
	io.WriteString(textMd5, text)
	textID := (hex.EncodeToString(textMd5.Sum(nil)))[0:6]

	app.Logger().Infof("IP:%s Send a paste %s", ctx.RemoteAddr(), textID)
	redisClient.Do("SET", textID, text, "ex", "1000")

	ctx.ViewData("id", textID)
	ctx.View("redirect.html")
}

func mainPageHandler(ctx iris.Context) {
	ctx.View("input.html")
}

func textDataHandler(ctx iris.Context){
	textID := ctx.Params().GetStringDefault("id", "")
	
	v, err := redis.String(redisClient.Do("GET", strings.ToLower(textID)))
	if err != nil {
		ctx.ViewData("id", "/")
		ctx.View("redirect.html")
	} else {
		ctx.ViewData("content", v)
		ctx.View("raw.html")
	}
}

// Verify by myself but not iris
// www.google.com is not available in some region like china mainland
func verify(ctx iris.Context) bool {
	// Makeup URL
	verifyURL, _ := url.Parse("https://recaptcha.net/recaptcha/api/siteverify")
	arg := verifyURL.Query()
	arg.Set("secret", recaptchaSecret)
	arg.Set("response", ctx.FormValue("g-recaptcha-response"))
	verifyURL.RawQuery = arg.Encode()

	// Send to recaptcha verigy server
	recv, err := http.Get(verifyURL.String())
	if err != nil {
		app.Logger().Infof("Can't connect to recaptcha server.")
		return false
	}

	// Get json
	result, err := ioutil.ReadAll(recv.Body)
	recv.Body.Close()
	if err != nil {
		fmt.Println(err)
		app.Logger().Infof("Connection of recaptcha server seems incorrect")
		return false
	}
	fmt.Println(string(result))

	// Unmarshal Json to Struct
	var reRes RecaptchaResponse

	err = json.Unmarshal(result, &reRes)
	
	if err != nil {
		fmt.Println(err)
		app.Logger().Infof("Connection of recaptcha server seems incorrect")
	}

	// If verify secceed and user score >= 0.5 then return true
	if reRes.Success && reRes.Score >=0.5 {
		return true
	}
	return false
}