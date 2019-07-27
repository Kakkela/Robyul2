package helpers

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

var (
	sushiiImageServerBase string
)

func TakeHTMLScreenshot(html string, width, height int) (data []byte, err error) {
	if sushiiImageServerBase == "" {
		sushiiImageServerBase = GetConfig().Path("sushii-image-server.base").Data().(string)
	}

	marshalledRequest, err := json.Marshal(&SushiRequest{
		Html:   html,
		Width:  width,
		Height: height,
	})
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest("POST", sushiiImageServerBase+"/html", bytes.NewBuffer(marshalledRequest))
	if err != nil {
		return nil, err
	}

	request.Header.Set("User-Agent", DEFAULT_UA)
	request.Header.Set("Content-Type", "application/json")

	response, err := DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}

	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}

	return ioutil.ReadAll(response.Body)
}

type SushiRequest struct {
	Html   string `json:"html"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
