package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/aluka-7/configuration"
	"github.com/aluka-7/metacode"
	"github.com/aluka-7/trace"
	"github.com/aluka-7/zipkin"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type WebApp func(eng *echo.Echo)

type Config struct {
	Addr        string      `json:"addr"`
	Gzip        int         `json:"gzip"`        // gzip压缩等级
	MetricsAddr string      `json:"metricsAddr"` // 是否打开日记
	EnableLog   bool        `json:"enableLog"`   // 是否打开日记
	Tag         []trace.Tag `json:"tag"`
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
	var config = Config{MetricsAddr: ":7070"}
	if err := conf.Clazz("base", "server", "", systemId, &config); err != nil {
		panic("加载web引擎配置出错")
	}
	w.server.HideBanner = true
	w.server.Validator = formValidator
	w.server.Renderer = &webAppTemplate{}
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
	go metrics(config.MetricsAddr)
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

var formValidator = &echoValidator{validator: validator.New()}

type echoValidator struct {
	validator *validator.Validate
}

func (e echoValidator) Validate(i interface{}) error {
	err := e.validator.Struct(i)
	if err == nil {
		return nil
	}
	return metacode.Errorf(1006, "validator error[%s]", err)
}

// RegisterValidation将验证功能添加到由键表示的验证者的验证者映射中
// 注意:如果密钥已经存在,则先前的验证功能将被替换。
// 注意:此方法不是线程安全的,因此应在进行任何验证之前先将它们全部注册
func RegisterValidation(key string, fn validator.Func) error {
	return formValidator.validator.RegisterValidation(key, fn)
}
func NewWebAppTemplate(opt RenderOptions, tplSets ...string) *webAppTemplate {
	ts, op, cs := renderHandler(prepareRenderOptions([]RenderOptions{opt}), tplSets)
	return &webAppTemplate{ts, op, cs}
}

type webAppTemplate struct {
	ts      *TemplateSet
	opt     RenderOptions
	charset string
}

func (f webAppTemplate) Render(writer io.Writer, s string, data interface{}, ctx echo.Context) error {
	r := &TplRender{ResponseWriter: ctx.Response(), TemplateSet: f.ts, Opt: &f.opt, Charset: f.charset}
	// Add global methods if data is a map
	if viewContext, isMap := data.(map[string]interface{}); isMap {
		viewContext["reverse"] = ctx.Echo().Reverse
		ns := os.Getenv("CI_PROJECT_NAMESPACE")
		an := os.Getenv("CI_APP_NAME")
		PaPath := ""
		if len(ns) > 0 && len(an) > 0 {
			PaPath = "/" + ns + "/" + an
		}
		viewContext["PaPath"] = PaPath
		viewContext["TmplLoadTimes"] = func() string {
			if r.startTime.IsZero() {
				return ""
			}
			return fmt.Sprint(time.Since(r.startTime).Nanoseconds()/1e6) + "ms"
		}
	}
	if v, e := r.HTMLSetBytes(DefaultTplSetName, s, data); e == nil {
		return ctx.HTMLBlob(http.StatusOK, v)
	} else {
		return e
	}
}
