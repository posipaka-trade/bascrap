package scraper

import (
	"encoding/json"
	"errors"
	"github.com/zelenin/go-tdlib/client"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	//binanceChannelId = -1001146915409
	//binanceChannelId    = -1001390705243 // testChannel
	//binanceChannelId = -1001501948186 //Oleg
	madNewsChannelId = -1001219306781

	binanceNewsSource = "Binance EN"
	newsTitle         = "title"
	newsSource        = "source"
)

type ScrapHandler struct {
	latestTelegramNews string
	latestWebsiteNews  string
	nextRequestTime    time.Time

	httpClient  *http.Client
	tdlibClient *client.Client
}

func New(tclient *client.Client) ScrapHandler {
	handler := ScrapHandler{
		tdlibClient: tclient,
		httpClient:  new(http.Client),
	}

	_, err := handler.LatestTelegramNews()
	if err != nil {
		if _, isOkay := err.(*NoNewsUpdate); !isOkay {
			panic("Initial news request to Mad telegram channel failed: " + err.Error())
		}
	}

	_, err = handler.LatestWebsiteNews()
	if err != nil {
		if _, isOkay := err.(*NoNewsUpdate); !isOkay {
			panic("Initial news request to Mad website failed: " + err.Error())
		}
	}

	return handler
}

func (handler *ScrapHandler) LatestTelegramNews() (string, error) {
	messages, err := handler.tdlibClient.GetChatHistory(&client.GetChatHistoryRequest{
		ChatId:        madNewsChannelId,
		FromMessageId: 0,
		Offset:        0,
		Limit:         20,
		OnlyLocal:     false,
	})
	if err != nil {
		return "", err
	}

	content, isOkay := messages.Messages[0].Content.(*client.MessageText)
	if !isOkay {
		return "", errors.New("[scraper] -> Casting of text message failed")
	}

	if handler.latestTelegramNews == content.Text.Text[:strings.Index(content.Text.Text, "\n")] {
		return "", &NoNewsUpdate{}
	}

	handler.latestTelegramNews = content.Text.Text[:strings.Index(content.Text.Text, "\n")]
	return handler.latestTelegramNews, nil
}

func (handler *ScrapHandler) LatestWebsiteNews() (string, error) {
	newsMap, err := handler.requestNewsFromWebsite()
	if err != nil {
		return "", err
	}

	isBinanceNews, err := newsRelatesToBinance(newsMap)
	if err != nil {
		return "", err
	}

	if !isBinanceNews {
		return "", &NoNewsUpdate{}
	}

	title, isOk := newsMap[0][newsTitle].(string)
	if isOk != true {
		return "", errors.New("news title value parsing failed")
	}

	if title != handler.latestWebsiteNews {
		handler.latestWebsiteNews = title
		return title, nil
	}

	return "", &NoNewsUpdate{}
}

func (handler *ScrapHandler) requestNewsFromWebsite() ([]map[string]interface{}, error) {
	if time.Now().Before(handler.nextRequestTime) {
		return nil, errors.New("there is no available request to Mad website")
	}
	response, err := handler.httpClient.Get("https://www.madnews.io/api/news?limit=1")
	if err != nil {
		return nil, err
	}

	if response.Header.Get("X-Ratelimit-Remaining") == "0" {
		timestamp, err := strconv.Atoi(response.Header.Get("X-Ratelimit-Reset"))
		if err != nil {
			return nil, err
		}
		handler.nextRequestTime = time.Unix(int64(timestamp), 0)
	}

	if response.StatusCode != 200 {
		return nil, errors.New("Mad website http error response: " + response.Status)
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var newsMap []map[string]interface{}
	err = json.Unmarshal(body, &newsMap)
	if err != nil {
		return nil, err
	}

	return newsMap, nil
}

func newsRelatesToBinance(newsMap []map[string]interface{}) (bool, error) {
	if len(newsMap) != 1 {
		return false, errors.New("incorrect response size from Mad website")
	}

	source, isOk := newsMap[0][newsSource].(string)
	if isOk != true {
		return false, errors.New("news source value parsing failed")
	}

	if source == binanceNewsSource {
		return true, nil
	}

	return false, nil
}
