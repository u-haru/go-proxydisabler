package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	localHost          = "localhost:8080"
	proxyHost          = ""
	proxyUser          = ""
	proxyAuthorization = ""
	noProxy            = false
	srv                *http.Server
)

func HandleHttps(writer http.ResponseWriter, req *http.Request) {
	if noProxy == false {
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
			if proxyAuthorization != "" {
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
	} else {
		destConn, err := net.Dial("tcp", req.URL.Hostname()+":443")
		if err != nil {
			log.Print(err)
		}
		writer.WriteHeader(200)

		if clientConn, _, err := writer.(http.Hijacker).Hijack(); err != nil {
			log.Print(err)
		} else {
			go func() {
				io.Copy(clientConn, destConn)
				destConn.Close()
			}()
			go func() {
				io.Copy(destConn, clientConn)
				clientConn.Close()
			}()
		}
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
		HandleHttps(writer, req)
	} else {
		HandleHttp(writer, req)
	}
}

func InitLocalServer() {
	if noProxy == false {
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
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}
	srv = &http.Server{
		Addr:    localHost,
		Handler: http.HandlerFunc(HandleRequest),
	}
	proxyAuthorization = "Basic " + base64.StdEncoding.EncodeToString([]byte(proxyUser))
}

func StartServer() {
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("Server closed with error:", err)
		}
	}()
	log.Printf("Start serving on %s", localHost)
}

func StopServer() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Println("Failed to gracefully shutdown:", err)
	}
	log.Println("Server shutdown")
}

func main() {
	flag.StringVar(&proxyUser, "u", "", "username:password")
	flag.StringVar(&localHost, "p", "localhost:8080", "Proxy:port")
	flag.StringVar(&proxyHost, "x", "10.1.16.8:8080", "Proxy:port")
	flag.BoolVar(&noProxy, "n", false, "NoProxy")
	flag.Parse()

	InitLocalServer()

	StartServer()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	log.Printf("SIGNAL %d received, shutting down...\n", <-quit)

	StopServer()
}
