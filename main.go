package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	jsonC "github.com/nwidger/jsoncolor"
	"github.com/pborman/getopt"
)

var (
	head            = "HEAD"
	httpMethod      *string
	postBody        *string
	followRedirects *bool
	onlyHeader      *bool
	httpHeaders     []string
	help            *bool
)

func main() {
	// remove curl from args
	for index, arg := range os.Args {
		if arg == "curl" {
			os.Args = append(os.Args[:index], os.Args[index+1:]...)
		}
	}

	httpMethod = getopt.StringLong("request", 'X', "GET", "HTTP method to use")
	help = getopt.BoolLong("help", 'h', "This help text")
	postBody = getopt.StringLong("data", 'd', "", "HTTP POST data")
	followRedirects = getopt.BoolLong("location", 'L', "Follow redirects")
	onlyHeader = getopt.BoolLong("head", 'I', "Show document info only")
	_ = getopt.ListVarLong(&httpHeaders, "header", 'H', "set HTTP header; repeatable: -H 'Accept: ...' -H 'Range: ...'")
	getopt.Parse()

	if *help {
		fmt.Println("help")
		os.Exit(0)
	}

	args := getopt.Args()

	if len(args) != 1 {
		fmt.Println("too many arguments")
		os.Exit(0)
	}

	if (*httpMethod == "POST" || *httpMethod == "PUT") && postBody == nil {
		log.Fatal("must supply post body using -d when POST or PUT is used")
	}

	if *onlyHeader {
		httpMethod = &head
	}
	visit(parseURL(args[0]))
}

func parseURL(uri string) (urlResponse *url.URL) {
	if !strings.Contains(uri, "://") && !strings.HasPrefix(uri, "//") {
		uri = "//" + uri
	}

	urlResponse, err := url.Parse(uri)
	if err != nil {
		log.Fatalf("could not parse url %q: %v", uri, err)
	}
	if urlResponse.Scheme == "" {
		urlResponse.Scheme = "http"
	}

	return
}

func headerKeyValue(h string) (string, string) {
	i := strings.Index(h, ":")
	if i == -1 {
		log.Fatalf("Header '%s' has invalid format, missing ':'", h)
	}
	return strings.TrimRight(h[:i], " "), strings.TrimLeft(h[i:], " :")
}

func dialContext(network string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, _, addr string) (net.Conn, error) {
		return (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext(ctx, network, addr)
	}
}

// visit visits a url and times the interaction.
// If the response is a 30x, visit follows the redirect.
func visit(url *url.URL) {

	req := newRequest(httpMethod, postBody, url)

	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	tr.DialContext = dialContext("tcp4")

	client := &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// always refuse to follow redirects, visit does that
			// manually if required.
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("failed to read response: %v", err)
	}

	bodyMsg := readResponseBody(req, resp)
	err = resp.Body.Close()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bodyMsg))
}

func isRedirect(resp *http.Response) bool {
	return resp.StatusCode > 299 && resp.StatusCode < 400
}

func newRequest(method, body *string, url *url.URL) *http.Request {
	req, err := http.NewRequest(*method, url.String(), createBody(*body))
	if err != nil {
		log.Fatalf("unable to create request: %v", err)
	}

	for _, h := range httpHeaders {
		k, v := headerKeyValue(h)
		if strings.EqualFold(k, "host") {
			req.Host = v
			continue
		}
		req.Header.Add(k, v)
	}
	return req
}

func createBody(body string) io.Reader {
	if strings.HasPrefix(body, "@") {
		filename := body[1:]
		f, err := os.Open(filename)
		if err != nil {
			log.Fatalf("failed to open data file %s: %v", filename, err)
		}
		return f
	}
	return strings.NewReader(body)
}

// readResponseBody ...
func readResponseBody(req *http.Request, resp *http.Response) (response []byte) {
	if isRedirect(resp) || req.Method == http.MethodHead {
		return
	}
	f := jsonC.NewFormatter()
	// set custom colors
	f.StringColor = color.New(color.FgCyan)
	f.TrueColor = color.New(color.FgCyan)
	f.FalseColor = color.New(color.FgCyan)
	f.NumberColor = color.New(color.FgCyan)
	f.FieldColor = color.New(color.FgBlue)
	f.FieldQuoteColor = color.New(color.FgBlue)
	f.NullColor = color.New(color.FgCyan)

	var jsonMaps []map[string]interface{}
	var jsonMap map[string]interface{}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	if string(bodyBytes)[0] == '[' {
		err = json.Unmarshal(bodyBytes, &jsonMaps)
		if err != nil {
			log.Fatal(err)
		}
		response, err = jsonC.MarshalIndentWithFormatter(jsonMaps, "", "  ", f)
		if err != nil {
			log.Fatal(err)
		}

	} else {
		err = json.Unmarshal(bodyBytes, &jsonMap)
		if err != nil {
			log.Fatal(err)
		}

		response, err = jsonC.MarshalIndentWithFormatter(jsonMap, "", "  ", f)
		if err != nil {
			log.Fatal(err)
		}

	}
	return
}
