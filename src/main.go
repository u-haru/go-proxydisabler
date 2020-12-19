package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

var (
	port               = 8080
	proxyHost          = ""
	proxyUser          = ""
	proxyPass          = ""
	proxyAuthorization = ""
	noProxy            = false
)

func HandleHttps(writer http.ResponseWriter, req *http.Request) {
	hijacker, _ := writer.(http.Hijacker)
	if proxyConn, err := net.Dial("tcp", proxyHost); err != nil {
		log.Print(err)
	} else if clientConn, _, err := hijacker.Hijack(); err != nil {
		log.Print(err)
	} else {
		addr, err := net.ResolveIPAddr("ip4", req.URL.Hostname())
		if err != nil {
			log.Print(err)
		}
		req.Host = fmt.Sprintf("%s", addr.String())
		if proxyUser != "" {
			req.Header.Set("Proxy-Authorization", proxyAuthorization)
		}
		req.Write(proxyConn)
		go func() {
			io.Copy(clientConn, proxyConn)
			proxyConn.Close()
		}()
		go func() {
			io.Copy(proxyConn, clientConn)
			clientConn.Close()
		}()
	}
}

func HandleHttp(writer http.ResponseWriter, req *http.Request) {
	hijacker, _ := writer.(http.Hijacker)
	client := new(http.Client)
	req.RequestURI = ""
	if resp, err := client.Do(req); err != nil {
		log.Print(err)
	} else if conn, _, err := hijacker.Hijack(); err != nil {
		log.Print(err)
	} else {
		defer conn.Close()
		resp.Write(conn)
	}
}

func HandleRequest(writer http.ResponseWriter, req *http.Request) {
	if req.Method == "CONNECT" {
		if noProxy == true {
			http.Error(writer, "Not Supported", http.StatusNotFound)
			return
		}
		HandleHttps(writer, req)
	} else {
		HandleHttp(writer, req)
	}
}

func main() {
	_proxyUser := flag.String("u", "", "username:password")
	_port := flag.Int("p", 8080, "local port")
	_proxyHost := flag.String("x", "10.1.16.8:8080", "Proxy:port")
	_noProxy := flag.Bool("n", false, "NoProxy")
	flag.Parse()
	proxyUser = *_proxyUser
	port = *_port
	proxyHost = *_proxyHost
	noProxy = *_noProxy

	proxyAuthorization = "Basic " + base64.StdEncoding.EncodeToString([]byte(proxyUser))
	proxyUrlString := ""

	if proxyUser != "" {
		proxyUrlString = fmt.Sprintf("http://%s@%s", strings.Replace(url.QueryEscape(proxyUser), "%3A", ":", 1), proxyHost)
	} else {
		proxyUrlString = fmt.Sprintf("http://%s", proxyHost)
	}

	proxyUrl, err := url.Parse(proxyUrlString)
	if err != nil {
		log.Fatal(err)
	}
	if noProxy == false {
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
		// fmt.Println(proxyHost)
	}
	proxyhandler := http.HandlerFunc(HandleRequest)

	listen := fmt.Sprintf("0.0.0.0:%d", port)
	log.Printf("Start serving on %s", listen)
	log.Fatal(http.ListenAndServe(listen, proxyhandler))
}
