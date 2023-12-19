package profiler

import (
	"context"
	"net"
	"net/http"
	"strings"
)

func NewHTTPClient(protocol, address, serverId, serverToken string) *http.Client {
	t := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (conn net.Conn, err error) {
			if protocol == "unix" {
				return net.Dial("unix", address)
			}

			return net.Dial(network, addr)
		},
	}

	return &http.Client{
		Transport: &bfTransport{
			Transport:   t,
			serverId:    serverId,
			serverToken: serverToken,
		},
	}
}

type bfTransport struct {
	Transport   http.RoundTripper
	serverId    string
	serverToken string
}

func (t *bfTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.serverId != "" && t.serverToken != "" {
		req.SetBasicAuth(t.serverId, t.serverToken)
	}

	response, err := t.Transport.RoundTrip(req)
	if err != nil {
		if strings.Contains(err.Error(), "malformed HTTP version") {
			log.Error().Str("endpoint", req.URL.String()).Err(err).Msg("failed to send request")
			return response, errOldAgent
		}
		return response, err
	}

	if response.StatusCode == 404 {
		log.Error().Str("endpoint", req.URL.String()).Msg("failed to send request - got 404 response")
		return response, errOldAgent
	}

	return response, err
}
