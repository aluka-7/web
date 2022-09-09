package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/aluka-7/configuration"
	"github.com/aluka-7/trace"
	"github.com/aluka-7/zipkin"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type WebApp func(eng *echo.Echo)

type Config struct {
	Addr      string      `json:"addr"`
	Gzip      int         `json:"gzip"`      // gzip压缩等级
	EnableLog bool        `json:"enableLog"` // 是否打开日记
	Tag       []trace.Tag `json:"tag"`
}

var SwagHandler echo.HandlerFunc

func init() {
	fmt.Println("Loading Web Engine ver:1.0")
}

func App(wa WebApp, systemId string, conf configuration.Configuration) {
	OptApp(wa, systemId, conf).Close(func() {})
}

type web struct {
	server *echo.Echo
}

func OptApp(wa WebApp, systemId string, conf configuration.Configuration) *web {
	w := newWeb()
	var config Config
	if err := conf.Clazz("base", "server", "", systemId, &config); err != nil {
		panic("加载web引擎配置出错")
	}
	w.server.HideBanner = true
	if len(config.Tag) > 0 {
		zipkin.Init(systemId, conf, config.Tag)
	}
	w.server.Use(middleware.Recover(), Trace(), LoggerWithConfig(systemId, config.EnableLog, DefaultLoggerConfig))
	// 为请求生成唯一id
	// Dependency Injection & Route Register
	wa(w.server)
	w.server.GET("/healthy", func(c echo.Context) error {
		return c.JSON(http.StatusOK, "Okey!")
	})
	if SwagHandler != nil {
		w.server.GET("/doc/*", SwagHandler)
	} else {
		w.server.Use(middleware.Gzip())
	}
	go metrics()
	w.start(config)
	return w
}

func newWeb() *web {
	server := echo.New()
	return &web{server}
}
func (w *web) start(config Config) {
	go func() {
		webPort := os.Getenv("WebPort")
		if len(webPort) == 0 {
			webPort = config.Addr
		}
		if err := w.server.Start(webPort); err != nil {
			fmt.Println("Echo Engine Start has error")
		}
	}()
}
func (w *web) Close(close func()) {
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit
	close()
	w.shutdown()
}
func (w *web) shutdown() {
	fmt.Println("Web Engine Shutdown Server ...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.server.Shutdown(ctx); err != nil {
		fmt.Println("Web Engine Shutdown has error")
	} else {
		fmt.Println("Web Engine exiting")
	}
}
