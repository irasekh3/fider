package httpclient

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/getfider/fider/app/pkg/bus"
)

func init() {
	bus.Register(&Service{})
}

type Service struct{}

func (s Service) Enabled() bool {
	return true
}

func (s Service) Init() {
	bus.AddHandler(s, requestHandler)
}

type BasicAuth struct {
	User     string
	Password string
}

type Request struct {
	URL       string
	Body      io.Reader
	Method    string
	Headers   map[string]string
	BasicAuth *BasicAuth

	ResponseBody       []byte
	ResponseStatusCode int
}

func requestHandler(ctx context.Context, cmd *Request) error {
	req, err := http.NewRequest(cmd.Method, cmd.URL, cmd.Body)
	if err != nil {
		return err
	}

	for k, v := range cmd.Headers {
		req.Header.Set(k, v)
	}
	if cmd.BasicAuth != nil {
		req.SetBasicAuth(cmd.BasicAuth.User, cmd.BasicAuth.Password)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	respBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	cmd.ResponseBody = respBody
	cmd.ResponseStatusCode = res.StatusCode
	return nil
}
