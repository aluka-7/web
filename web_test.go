package web_test

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/aluka-7/configuration"
	"github.com/aluka-7/configuration/backends"
	"github.com/aluka-7/web"
	"github.com/labstack/echo/v4"
	. "github.com/smartystreets/goconvey/convey"
)

func startServer(t *testing.T) {
	web.App(func(eng *echo.Echo) {
		eng.GET("/none/api", func(ctx echo.Context) error {
			return ctx.String(http.StatusOK, "test app")
		})
	}, "1000", configuration.MockEngine(t, backends.StoreConfig{Exp: map[string]string{
		"/system/base/server/1000": "{\"addr\":\":9999\"}",
	}}))
}

func TestApp(t *testing.T) {
	go startServer(t)
	client := &http.Client{}
	Convey("test App\n", t, func() {
		req, err := http.NewRequest("GET", "http://localhost:9999/none/api", nil)
		So(err, ShouldBeNil)
		resp, err := client.Do(req)
		So(err, ShouldBeNil)
		rid := resp.Header.Get(echo.HeaderXRequestID)
		So(rid, ShouldNotBeNil)
		actual, err := ioutil.ReadAll(resp.Body)
		So(err, ShouldBeNil)
		So(string(actual), ShouldEqual, "test app")
		resp.Body.Close()
	})
}
