package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"golang.org/x/net/websocket"
)

var echoWs = websocket.Handler(func(ws *websocket.Conn) {
	os.Stdout.Write([]byte("Start ECHO"))
	defer os.Stdout.Write([]byte("End ECHO"))
	r := io.TeeReader(ws, os.Stdout)
	io.Copy(ws, r)
})

func EchoServer(w http.ResponseWriter, r *http.Request) {
	echoWs.ServeHTTP(w, r)
}

func EchoServer2(w http.ResponseWriter, r *http.Request) {
	fmt.Println("echo2")
	c, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		panic("cannot hijack http")
	}
	defer c.Close()

	tee := io.TeeReader(c, os.Stdout)
	c.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
	fmt.Println("start echo2...")
	io.Copy(c, tee)
}

func EchoServer3(w http.ResponseWriter, r *http.Request) {
	fmt.Println("echo3")
	c, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		panic("cannot hijack http")
	}
	defer c.Close()

	tee := io.TeeReader(c, os.Stdout)
	c.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
	fmt.Println("start echo2...")
	ibuf := make([]byte, 1024)
	obuf := make([]byte, 2*1024)
	var n int
	var ierr, oerr error
	for ierr == nil && oerr == nil {
		n, ierr = tee.Read(ibuf)
		n = hex.Encode(obuf, ibuf[:n])
		_, oerr = c.Write(obuf[:n])
	}
}

func WebServer(ws *websocket.Conn) {
	os.Stdout.Write([]byte("Start WEB"))
	defer os.Stdout.Write([]byte("End WEB"))
	w := io.MultiWriter(ws, os.Stdout)

	resp, err := http.Get("http://httpbin.org/ip")
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	resp.Write(w)
}

func serveGET(ws net.Conn, req *http.Request) {
	fmt.Println("req.RequestURI", req.RequestURI)
	req.RequestURI = ""
	fmt.Println("req.URL.Scheme", req.URL.Scheme)
	req.URL.Scheme = "http"
	fmt.Println("req.URL.Host", req.URL.Host)
	req.URL.Host = req.Host
	fmt.Println("URL", req.URL.RequestURI())

	resp, err := http.DefaultTransport.(*http.Transport).RoundTrip(req)
	if err != nil {
		fmt.Println(err)
		io.WriteString(ws, "HTTP/1.1 400 Bad Request\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n400 Bad Request: "+err.Error())
		return
	}
	resp.Write(ws)
}

func serveCONNECT(ws net.Conn, req *http.Request) {
	host := req.URL.Host
	fmt.Println("CONNECTING", host, "...")
	c, err := http.DefaultTransport.(*http.Transport).Dial("tcp", host)
	if err != nil {
		fmt.Println("ERR:", err)
		io.WriteString(ws, "HTTP/1.1 500 Internal Server Error\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n500 Internal Server Error: "+err.Error())
		return
	}
	c.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
	fmt.Println("start tunnel...")
	go func() {
		io.Copy(ws, c)
		ws.Close()
	}()
	io.Copy(c, ws)
	c.Close()
}

var wsProxy = websocket.Handler(func(ws *websocket.Conn) {
	req, err := http.ReadRequest(bufio.NewReader(ws))
	if err != nil {
		io.WriteString(ws, "HTTP/1.1 400 Bad Request\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n400 Bad Request")
		return
	}
	//b, _ := httputil.DumpRequestOut(req, true)
	//os.Stdout.Write(b)

	fmt.Println("req.Method", req.Method)
	switch req.Method {
	case "CONNECT":
		serveCONNECT(ws, req)
	default:
		serveGET(ws, req)
	}
	//io.WriteString(ws, "HTTP/1.1 400 Bad Request\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n400 Bad Request")
	//return
})

func handleProxy(w http.ResponseWriter, r *http.Request) {
	c, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		panic("cannot hijack http")
	}
	defer c.Close()

	req, err := http.ReadRequest(rw.Reader)
	if err != nil {
		io.WriteString(c, "HTTP/1.1 400 Bad Request\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n400 Bad Request")
		return
	}
	//b, _ := httputil.DumpRequestOut(req, true)
	//os.Stdout.Write(b)

	fmt.Println("req.Method", req.Method)
	switch req.Method {
	case "CONNECT":
		serveCONNECT(c, req)
	default:
		serveGET(c, req)
	}
}

func main() {
	http.HandleFunc("/echo", EchoServer)
	http.HandleFunc("/echo2", EchoServer2)
	http.HandleFunc("/echo3", EchoServer3)
	http.HandleFunc("/proxy2", handleProxy)
	http.Handle("/proxy", wsProxy)
	//proxy := NewProxyListener(nil)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Hello world")
		fmt.Fprintf(w, "Hello world!")
	})

	bind := getIP() + ":" + getPort()
	fmt.Println("Listening to", bind)

	err := http.ListenAndServe(bind, nil)
	if err != nil {
		panic(err)
	}
}
