package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

const CTX_KEY = "__APIGatewayV2HTTPRequest"

func parseProto(p string) (int, int) {
	n := strings.Split("/", p)
	if len(n) == 2 {
		if f, err := strconv.ParseFloat(n[1], 64); err != nil {
			return int(f), int(f*10) % 10
		}
	}
	return 0, 0
}

func newWriter() *responseWriter {
	return &responseWriter{
		h: http.Header{},
	}
}

type responseWriter struct {
	h http.Header
	b bytes.Buffer
	s int
}

func (w *responseWriter) Header() http.Header {
	return w.h
}

func (w *responseWriter) Write(b []byte) (int, error) {
	return w.b.Write(b)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.s = statusCode
}

func (w *responseWriter) APIGatewayV2HTTPResponse() *events.APIGatewayV2HTTPResponse {
	body := make([]byte, base64.StdEncoding.EncodedLen(w.b.Len()))
	base64.StdEncoding.Encode(body, w.b.Bytes())

	return &events.APIGatewayV2HTTPResponse{
		StatusCode:        w.s,
		MultiValueHeaders: w.h,
		Body:              string(body),
		Cookies:           w.h.Values("Set-Cookie"),
		IsBase64Encoded:   true,
	}
}

func Convert(ctx context.Context, r *events.APIGatewayV2HTTPRequest) (*http.Request, error) {
	var body io.ReadCloser = nil

	if r.IsBase64Encoded {
		body = io.NopCloser(base64.NewDecoder(base64.StdEncoding, bytes.NewBufferString(r.Body)))
	} else {
		body = io.NopCloser(bytes.NewBufferString(r.Body))
	}

	headers := http.Header{}

	for k, v := range r.Headers {
		headers.Set(k, v)
	}

	proto := headers.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "https"
	}

	uri := fmt.Sprintf("%s://%s%s", proto, r.RequestContext.DomainName, r.RawPath)
	p := r.RawQueryString
	if r.RawQueryString != "" {
		uri = uri + "?" + r.RawQueryString
		p = p + "?" + r.RawQueryString
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	mej, mino := parseProto(r.RequestContext.HTTP.Protocol)

	var leng int64
	l, err := strconv.Atoi(headers.Get("Content-Length"))
	if err != nil {
		leng = int64(l)
	}

	req := &http.Request{
		Method:        r.RequestContext.HTTP.Method,
		URL:           u,
		Proto:         r.RequestContext.HTTP.Protocol,
		ProtoMajor:    mej,
		ProtoMinor:    mino,
		Header:        headers,
		Body:          body,
		ContentLength: leng,
		RemoteAddr:    r.RequestContext.HTTP.SourceIP,
		RequestURI:    p,
		Host:          headers.Get("Host"),
	}

	ctx = context.WithValue(ctx, CTX_KEY, r)

	return req.WithContext(ctx), nil
}

func LambdaParam(ctx context.Context) *events.APIGatewayV2HTTPRequest {
	return ctx.Value(CTX_KEY).(*events.APIGatewayV2HTTPRequest)
}

func LambdaFnc(handler http.Handler) func(ctx context.Context, r *events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	return func(ctx context.Context, r *events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		req, err := Convert(ctx, r)
		if err != nil {
			return nil, err
		}

		w := newWriter()

		handler.ServeHTTP(w, req)

		return w.APIGatewayV2HTTPResponse(), nil
	}
}

func IsLambda() bool {
	return os.Getenv("AWS_LAMBDA_RUNTIME_API") != ""
}

func main() {
	a := &events.APIGatewayV2HTTPRequest{}
	log.Println(&a)
}
