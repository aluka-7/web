package web

import (
	"bufio"
	"errors"
	"github.com/aluka-7/trace"
	"github.com/labstack/echo/v4"
	"net"
	"net/http"
	"strconv"
)

// Trace 是跟踪中间件
func Trace() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			// 处理http请求
			r := c.Request()
			// 从http请求标头获取派生跟踪
			t, err := trace.Extract(trace.HTTPFormat, c.Request().Header)
			if err != nil {
				var opts []trace.Option
				if ok, _ := strconv.ParseBool(trace.FosTraceDebug); ok {
					opts = append(opts, trace.EnableDebug())
				}
				t = trace.New(c.Request().URL.Path, opts...)
			}
			defer t.Finish(nil)

			t.SetTitle(c.Path())
			t.SetTag(trace.String(trace.TagComponent, c.Request().Header.Get("User-Agent")))
			t.SetTag(trace.String(trace.TagHTTPMethod, c.Request().Method))
			t.SetTag(trace.String(trace.TagHttpURL, c.Request().URL.String()))
			t.SetTag(trace.String(trace.TagSpanKind, "server"))
			// 将跟踪ID导出给用户。
			c.Response().Header().Set(trace.FosTraceID, t.TraceId())
			c.SetRequest(c.Request().WithContext(trace.NewContext(r.Context(), t)))
			nrw := NewResponseWriter(c.Response().Writer)
			if err := next(c); err != nil {
				c.Error(err)
			}
			if nrw.Size() > 0 {
				t.SetTag(trace.Int("http.response.size", nrw.Size()))
			}
			if nrw.Status() < 200 || nrw.Status() > 299 {
				t.SetTag(trace.Int(trace.TagHTTPStatusCode, nrw.Status()))
				if nrw.Status() > 399 {
					t.SetTag(trace.Bool(trace.TagError, true))
				}
			}
			return nil
		}
	}
}

// ResponseWriter is a wrapper around http.ResponseWriter that provides extra information about
// the response. It is recommended that middleware handlers use this construct to wrap a response writer
// if the functionality calls for it.
type ResponseWriter interface {
	http.ResponseWriter
	http.Flusher
	// Status returns the status code of the response or 0 if the response has
	// not been written
	Status() int
	// Written returns whether or not the ResponseWriter has been written.
	Written() bool
	// Size returns the size of the response body.
	Size() int
	// Before allows for a function to be called before the ResponseWriter has been written to. This is
	// useful for setting headers or any other operations that must happen before a response has been written.
	Before(func(ResponseWriter))
}

type beforeFunc func(ResponseWriter)

// NewResponseWriter creates a ResponseWriter that wraps an http.ResponseWriter
func NewResponseWriter(rw http.ResponseWriter) ResponseWriter {
	nrw := &responseWriter{
		ResponseWriter: rw,
	}

	return nrw
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	size        int
	beforeFuncs []beforeFunc
}

func (rw *responseWriter) WriteHeader(s int) {
	rw.status = s
	rw.callBefore()
	rw.ResponseWriter.WriteHeader(s)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.Written() {
		// The status will be StatusOK if WriteHeader has not been called yet
		rw.WriteHeader(http.StatusOK)
	}
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) Size() int {
	return rw.size
}

func (rw *responseWriter) Written() bool {
	return rw.status != 0
}

func (rw *responseWriter) Before(before func(ResponseWriter)) {
	rw.beforeFuncs = append(rw.beforeFuncs, before)
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("the ResponseWriter doesn't support the Hijacker interface")
	}
	return hijacker.Hijack()
}

func (rw *responseWriter) callBefore() {
	for i := len(rw.beforeFuncs) - 1; i >= 0; i-- {
		rw.beforeFuncs[i](rw)
	}
}

func (rw *responseWriter) Flush() {
	flusher, ok := rw.ResponseWriter.(http.Flusher)
	if ok {
		if !rw.Written() {
			// The status will be StatusOK if WriteHeader has not been called yet
			rw.WriteHeader(http.StatusOK)
		}
		flusher.Flush()
	}
}

func (rw *responseWriter) CloseNotify() <-chan bool {
	return rw.ResponseWriter.(http.CloseNotifier).CloseNotify()
}
