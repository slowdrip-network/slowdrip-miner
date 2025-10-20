package mediamtx

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type Client struct {
	base string
	http *http.Client
	log  zerolog.Logger
}

func NewClient(base string, log zerolog.Logger) *Client {
	return &Client{
		base: base,
		http: &http.Client{Timeout: 5 * time.Second},
		log:  log,
	}
}

func (c *Client) Paths() ([]string, error) {
	resp, err := c.http.Get(c.base + "/v3/paths/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		names = append(names, it.Name)
	}
	return names, nil
}
