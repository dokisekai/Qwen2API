package proxy

import (
	"bufio"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/config"
	utls "github.com/refraction-networking/utls"
)

var transport *http.Transport

func Init() {
	if config.C.ProxyURL == "" {
		transport = newUTLSTransport(nil)
		return
	}
	u, err := url.Parse(config.C.ProxyURL)
	if err != nil {
		transport = newUTLSTransport(nil)
		return
	}
	transport = newUTLSTransport(u)
}

func newUTLSTransport(proxyURL *url.URL) *http.Transport {
	return &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			if proxyURL != nil {
				return proxyURL, nil
			}
			return nil, nil
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

func NewUTLSHttpClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Minute, Transport: newUTLSRoundTripper(nil)}
}

type utlsRoundTripper struct {
	proxyURL *url.URL
}

func newUTLSRoundTripper(proxyURL *url.URL) *utlsRoundTripper {
	return &utlsRoundTripper{proxyURL: proxyURL}
}

func (rt *utlsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	targetAddr := req.URL.Host
	if req.URL.Port() != "" {
		targetAddr = net.JoinHostPort(req.URL.Hostname(), req.URL.Port())
	} else {
		targetAddr = net.JoinHostPort(req.URL.Host, "443")
	}

	var conn net.Conn
	var err error

	if rt.proxyURL != nil {
		conn, err = net.DialTimeout("tcp", rt.proxyURL.Host, 30*time.Second)
		if err != nil {
			return nil, err
		}

		connectReq := "CONNECT " + targetAddr + " HTTP/1.1\r\nHost: " + targetAddr + "\r\n\r\n"
		conn.Write([]byte(connectReq))

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			conn.Close()
			return nil, err
		}
		resp := string(buf[:n])
		if len(resp) < 12 || resp[9:12] != "200" {
			conn.Close()
			return nil, err
		}
	} else {
		conn, err = net.DialTimeout("tcp", targetAddr, 30*time.Second)
		if err != nil {
			return nil, err
		}
	}

	tlsConn := utls.UClient(conn, &utls.Config{
		ServerName:         req.URL.Hostname(),
		InsecureSkipVerify: false,
	}, utls.HelloChrome_Auto)

	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, err
	}

	switch req.URL.Scheme {
	case "https":
		_ = tlsConn
	default:
		conn.Close()
		return nil, err
	}

	return rt.roundTripOnConn(tlsConn, req)
}

func (rt *utlsRoundTripper) roundTripOnConn(tlsConn *utls.UConn, req *http.Request) (*http.Response, error) {
	if err := req.Write(tlsConn); err != nil {
		tlsConn.Close()
		return nil, err
	}

	br := bufio.NewReader(tlsConn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		tlsConn.Close()
		return nil, err
	}

	return resp, nil
}

func GetTransport() *http.Transport {
	return transport
}

func ChatBaseURL() string {
	return config.C.QwenChatProxyURL
}

func CliBaseURL() string {
	return config.C.QwenCliProxyURL
}

func Client() *http.Client {
	if transport != nil {
		return &http.Client{Transport: transport, Timeout: 5 * time.Minute}
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func UTLSClient() *http.Client {
	return NewUTLSHttpClient()
}
